package cost

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// Calculator calculates costs for VPSie offerings and NodeGroups
type Calculator struct {
	client client.VPSieClient
	cache  *costCache
	mu     sync.RWMutex
}

// costCache caches offering costs to reduce API calls
type costCache struct {
	offerings map[string]*OfferingCost
	mu        sync.RWMutex
	ttl       time.Duration
}

// NewCalculator creates a new cost calculator
func NewCalculator(client client.VPSieClient) *Calculator {
	return &Calculator{
		client: client,
		cache: &costCache{
			offerings: make(map[string]*OfferingCost),
			ttl:       time.Hour, // Cache for 1 hour
		},
	}
}

// GetOfferingCost returns the cost for a specific offering
func (c *Calculator) GetOfferingCost(ctx context.Context, offeringID string) (*OfferingCost, error) {
	// Check cache first
	if cached := c.cache.get(offeringID); cached != nil {
		return cached, nil
	}

	// Fetch from API
	offerings, err := c.client.ListOfferings(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list offerings: %w", err)
	}

	// Find the specific offering
	var found *client.Offering
	for i := range offerings {
		if offerings[i].ID == offeringID {
			found = &offerings[i]
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("offering %s not found", offeringID)
	}

	// Convert to OfferingCost
	cost := &OfferingCost{
		OfferingID:  found.ID,
		Name:        found.Name,
		HourlyCost:  found.HourlyPrice,
		DailyCost:   found.HourlyPrice * 24,
		MonthlyCost: found.Price,
		Currency:    "USD",
		Specs: ResourceSpecs{
			CPU:       found.CPU,
			MemoryMB:  found.RAM,
			DiskGB:    found.Disk,
			Bandwidth: found.Bandwidth,
		},
		Category:    found.Category,
		LastUpdated: time.Now(),
	}

	// Cache the result
	c.cache.set(offeringID, cost)

	return cost, nil
}

// CalculateNodeGroupCost calculates the current cost for a NodeGroup
func (c *Calculator) CalculateNodeGroupCost(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) (*NodeGroupCost, error) {
	if nodeGroup == nil {
		return nil, fmt.Errorf("nodeGroup cannot be nil")
	}

	instanceTypes := make(map[string]InstanceTypeCost)
	var totalHourly float64
	var totalNodes int32

	// If nodeGroup.Status.Nodes is empty, estimate based on desired
	if len(nodeGroup.Status.Nodes) == 0 {
		// Use the first offering from OfferingIDs as default
		if len(nodeGroup.Spec.OfferingIDs) == 0 {
			return nil, fmt.Errorf("no offerings specified in NodeGroup")
		}

		offeringID := nodeGroup.Spec.OfferingIDs[0]
		cost, err := c.GetOfferingCost(ctx, offeringID)
		if err != nil {
			return nil, fmt.Errorf("failed to get offering cost: %w", err)
		}

		count := nodeGroup.Status.DesiredNodes
		if count == 0 {
			count = nodeGroup.Spec.MinNodes
		}

		instanceTypes[offeringID] = InstanceTypeCost{
			OfferingID:   offeringID,
			Count:        count,
			HourlyEach:   cost.HourlyCost,
			TotalHourly:  cost.HourlyCost * float64(count),
			TotalMonthly: cost.MonthlyCost * float64(count),
		}

		totalHourly = cost.HourlyCost * float64(count)
		totalNodes = count
	} else {
		// Calculate based on actual nodes
		offeringCounts := make(map[string]int32)
		for _, node := range nodeGroup.Status.Nodes {
			offeringCounts[node.InstanceType]++
			totalNodes++
		}

		// Calculate cost for each instance type
		for offeringID, count := range offeringCounts {
			cost, err := c.GetOfferingCost(ctx, offeringID)
			if err != nil {
				return nil, fmt.Errorf("failed to get cost for offering %s: %w", offeringID, err)
			}

			instanceCost := InstanceTypeCost{
				OfferingID:   offeringID,
				Count:        count,
				HourlyEach:   cost.HourlyCost,
				TotalHourly:  cost.HourlyCost * float64(count),
				TotalMonthly: cost.MonthlyCost * float64(count),
			}

			instanceTypes[offeringID] = instanceCost
			totalHourly += instanceCost.TotalHourly
		}
	}

	costPerNode := float64(0)
	if totalNodes > 0 {
		costPerNode = totalHourly / float64(totalNodes)
	}

	return &NodeGroupCost{
		NodeGroupName: nodeGroup.Name,
		Namespace:     nodeGroup.Namespace,
		TotalNodes:    totalNodes,
		CostPerNode:   costPerNode,
		TotalHourly:   totalHourly,
		TotalDaily:    totalHourly * 24,
		TotalMonthly:  totalHourly * 730, // Average hours per month
		InstanceTypes: instanceTypes,
		LastUpdated:   time.Now(),
	}, nil
}

// CompareOfferings compares costs between multiple offerings
func (c *Calculator) CompareOfferings(ctx context.Context, offeringIDs []string) (*CostComparison, error) {
	if len(offeringIDs) == 0 {
		return nil, fmt.Errorf("no offerings to compare")
	}

	var offerings []OfferingCost
	var cheapest, mostExpensive *OfferingCost
	var totalCost float64

	for _, id := range offeringIDs {
		cost, err := c.GetOfferingCost(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get cost for offering %s: %w", id, err)
		}

		offerings = append(offerings, *cost)
		totalCost += cost.MonthlyCost

		if cheapest == nil || cost.MonthlyCost < cheapest.MonthlyCost {
			cheapest = cost
		}
		if mostExpensive == nil || cost.MonthlyCost > mostExpensive.MonthlyCost {
			mostExpensive = cost
		}
	}

	// Sort by monthly cost
	sort.Slice(offerings, func(i, j int) bool {
		return offerings[i].MonthlyCost < offerings[j].MonthlyCost
	})

	return &CostComparison{
		Offerings:       offerings,
		CheapestID:      cheapest.OfferingID,
		MostExpensiveID: mostExpensive.OfferingID,
		AverageCost:     totalCost / float64(len(offerings)),
		ComparedAt:      time.Now(),
	}, nil
}

// CalculateSavings estimates savings from switching instance types
func (c *Calculator) CalculateSavings(current, proposed *NodeGroupCost) (*SavingsAnalysis, error) {
	if current == nil || proposed == nil {
		return nil, fmt.Errorf("current and proposed costs cannot be nil")
	}

	monthlySavings := current.TotalMonthly - proposed.TotalMonthly
	annualSavings := monthlySavings * 12
	savingsPercent := float64(0)
	if current.TotalMonthly > 0 {
		savingsPercent = (monthlySavings / current.TotalMonthly) * 100
	}

	// Calculate break-even days (assumes migration has a cost)
	// For now, assume negligible migration cost
	breakEvenDays := 0

	// Determine recommended action
	action := "no_action"
	confidence := 0.0

	if monthlySavings > 0 {
		if savingsPercent > 20 {
			action = "strongly_recommended"
			confidence = 0.9
		} else if savingsPercent > 10 {
			action = "recommended"
			confidence = 0.75
		} else if savingsPercent > 5 {
			action = "consider"
			confidence = 0.6
		}
	} else if monthlySavings < -10 {
		// Proposed is more expensive
		action = "not_recommended"
		confidence = 0.9
	}

	return &SavingsAnalysis{
		CurrentCost:       *current,
		ProposedCost:      *proposed,
		MonthlySavings:    monthlySavings,
		AnnualSavings:     annualSavings,
		SavingsPercent:    savingsPercent,
		BreakEvenDays:     breakEvenDays,
		RecommendedAction: action,
		Confidence:        confidence,
		GeneratedAt:       time.Now(),
	}, nil
}

// FindCheapestOffering finds the cheapest offering that meets requirements
func (c *Calculator) FindCheapestOffering(ctx context.Context, requirements ResourceRequirements, allowedOfferings []string) (*Recommendation, error) {
	offerings, err := c.client.ListOfferings(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list offerings: %w", err)
	}

	// Filter offerings by requirements and allowed list
	allowedMap := make(map[string]bool)
	for _, id := range allowedOfferings {
		allowedMap[id] = true
	}

	var candidates []client.Offering
	for _, offering := range offerings {
		// Check if allowed
		if len(allowedOfferings) > 0 && !allowedMap[offering.ID] {
			continue
		}

		// Check if meets requirements
		if offering.CPU >= requirements.MinCPU &&
			offering.RAM >= requirements.MinMemoryMB &&
			offering.Disk >= requirements.MinDiskGB &&
			offering.Bandwidth >= requirements.MinBandwidth &&
			offering.Available {
			candidates = append(candidates, offering)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no offerings meet the requirements")
	}

	// Sort by price (cheapest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Price < candidates[j].Price
	})

	cheapest := candidates[0]

	// Build alternative options (next 2-3 cheapest)
	var alternatives []string
	for i := 1; i < len(candidates) && i < 4; i++ {
		alternatives = append(alternatives, candidates[i].ID)
	}

	rationale := fmt.Sprintf("Cheapest offering that meets requirements: %d CPU, %d MB RAM, %d GB disk",
		cheapest.CPU, cheapest.RAM, cheapest.Disk)

	// Calculate confidence based on how well it matches requirements
	confidence := 0.8
	if cheapest.CPU == requirements.MinCPU && cheapest.RAM == requirements.MinMemoryMB {
		confidence = 0.95 // Exact match
	} else if cheapest.CPU > requirements.MinCPU*2 || cheapest.RAM > requirements.MinMemoryMB*2 {
		confidence = 0.6 // Significant over-provisioning
	}

	return &Recommendation{
		OfferingID:         cheapest.ID,
		OfferingName:       cheapest.Name,
		Rationale:          rationale,
		ExpectedSavings:    0, // Would need current cost to calculate
		PerformanceImpact:  "none",
		Confidence:         confidence,
		AlternativeOptions: alternatives,
	}, nil
}

// CalculateCostPerResource calculates cost per CPU, memory, and disk
func (c *Calculator) CalculateCostPerResource(ctx context.Context, offeringID string) (cpuCost, memoryCost, diskCost float64, err error) {
	cost, err := c.GetOfferingCost(ctx, offeringID)
	if err != nil {
		return 0, 0, 0, err
	}

	// Prevent division by zero
	if cost.Specs.CPU > 0 {
		cpuCost = cost.MonthlyCost / float64(cost.Specs.CPU)
	}
	if cost.Specs.MemoryMB > 0 {
		memoryCost = cost.MonthlyCost / float64(cost.Specs.MemoryMB)
	}
	if cost.Specs.DiskGB > 0 {
		diskCost = cost.MonthlyCost / float64(cost.Specs.DiskGB)
	}

	return cpuCost, memoryCost, diskCost, nil
}

// cache methods

func (cc *costCache) get(offeringID string) *OfferingCost {
	cc.mu.RLock()
	cost, ok := cc.offerings[offeringID]
	cc.mu.RUnlock()

	if !ok {
		return nil
	}

	// Check if expired
	if time.Since(cost.LastUpdated) < cc.ttl {
		return cost
	}

	// Expired - remove with write lock
	cc.mu.Lock()
	delete(cc.offerings, offeringID)
	cc.mu.Unlock()

	return nil
}

func (cc *costCache) set(offeringID string, cost *OfferingCost) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.offerings[offeringID] = cost
}

func (cc *costCache) clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.offerings = make(map[string]*OfferingCost)
}

// SetCacheTTL sets the cache TTL
func (c *Calculator) SetCacheTTL(ttl time.Duration) {
	c.cache.ttl = ttl
}

// ClearCache clears the cost cache
func (c *Calculator) ClearCache() {
	c.cache.clear()
}
