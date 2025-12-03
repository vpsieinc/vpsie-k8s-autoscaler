package cost

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// Analyzer tracks and analyzes cost trends over time
type Analyzer struct {
	calculator *Calculator
	storage    CostStorage
	mu         sync.RWMutex
}

// CostStorage interface for persisting cost data
type CostStorage interface {
	// RecordSnapshot stores a cost snapshot
	RecordSnapshot(ctx context.Context, snapshot *CostSnapshot) error

	// GetSnapshots retrieves snapshots for a NodeGroup within a time range
	GetSnapshots(ctx context.Context, nodeGroup, namespace string, start, end time.Time) ([]*CostSnapshot, error)

	// GetLatestSnapshot retrieves the most recent snapshot
	GetLatestSnapshot(ctx context.Context, nodeGroup, namespace string) (*CostSnapshot, error)

	// DeleteOldSnapshots removes snapshots older than the retention period
	DeleteOldSnapshots(ctx context.Context, before time.Time) error
}

// MemoryCostStorage is an in-memory implementation of CostStorage
type MemoryCostStorage struct {
	snapshots map[string][]*CostSnapshot // key: "namespace/nodegroup"
	mu        sync.RWMutex
}

// NewMemoryCostStorage creates a new in-memory cost storage
func NewMemoryCostStorage() *MemoryCostStorage {
	return &MemoryCostStorage{
		snapshots: make(map[string][]*CostSnapshot),
	}
}

func (m *MemoryCostStorage) RecordSnapshot(ctx context.Context, snapshot *CostSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s/%s", snapshot.Namespace, snapshot.NodeGroupName)
	m.snapshots[key] = append(m.snapshots[key], snapshot)

	// Sort by timestamp
	sort.Slice(m.snapshots[key], func(i, j int) bool {
		return m.snapshots[key][i].Timestamp.Before(m.snapshots[key][j].Timestamp)
	})

	return nil
}

func (m *MemoryCostStorage) GetSnapshots(ctx context.Context, nodeGroup, namespace string, start, end time.Time) ([]*CostSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", namespace, nodeGroup)
	allSnapshots := m.snapshots[key]

	var result []*CostSnapshot
	for _, snapshot := range allSnapshots {
		if (snapshot.Timestamp.Equal(start) || snapshot.Timestamp.After(start)) &&
			(snapshot.Timestamp.Equal(end) || snapshot.Timestamp.Before(end)) {
			result = append(result, snapshot)
		}
	}

	return result, nil
}

func (m *MemoryCostStorage) GetLatestSnapshot(ctx context.Context, nodeGroup, namespace string) (*CostSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", namespace, nodeGroup)
	snapshots := m.snapshots[key]

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots found for %s", key)
	}

	return snapshots[len(snapshots)-1], nil
}

func (m *MemoryCostStorage) DeleteOldSnapshots(ctx context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, snapshots := range m.snapshots {
		var kept []*CostSnapshot
		for _, snapshot := range snapshots {
			if snapshot.Timestamp.After(before) || snapshot.Timestamp.Equal(before) {
				kept = append(kept, snapshot)
			}
		}
		m.snapshots[key] = kept
	}

	return nil
}

// NewAnalyzer creates a new cost analyzer
func NewAnalyzer(calculator *Calculator, storage CostStorage) *Analyzer {
	if storage == nil {
		storage = NewMemoryCostStorage()
	}

	return &Analyzer{
		calculator: calculator,
		storage:    storage,
	}
}

// RecordCost records a cost snapshot for a NodeGroup
func (a *Analyzer) RecordCost(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, utilization ResourceUtilization) error {
	if nodeGroup == nil {
		return fmt.Errorf("nodeGroup cannot be nil")
	}

	// Calculate current cost
	cost, err := a.calculator.CalculateNodeGroupCost(ctx, nodeGroup)
	if err != nil {
		return fmt.Errorf("failed to calculate cost: %w", err)
	}

	// Calculate efficiency score (0-100)
	efficiencyScore := a.calculateEfficiencyScore(cost, utilization)

	snapshot := &CostSnapshot{
		Timestamp:       time.Now(),
		NodeGroupName:   nodeGroup.Name,
		Namespace:       nodeGroup.Namespace,
		Cost:            *cost,
		Utilization:     utilization,
		EfficiencyScore: efficiencyScore,
	}

	return a.storage.RecordSnapshot(ctx, snapshot)
}

// calculateEfficiencyScore calculates how efficiently resources are being used
// Score ranges from 0-100, where higher is better
func (a *Analyzer) calculateEfficiencyScore(cost *NodeGroupCost, utilization ResourceUtilization) float64 {
	if cost == nil || cost.TotalNodes == 0 {
		return 0
	}

	// Average utilization across CPU and memory (weighted)
	avgUtilization := (utilization.CPUPercent*0.4 + utilization.MemoryPercent*0.6)

	// Ideal utilization is around 70-80%
	// Score decreases if too low (waste) or too high (risk)
	idealUtilization := 75.0
	deviation := avgUtilization - idealUtilization

	var score float64
	if avgUtilization < idealUtilization {
		// Below ideal - penalize for waste
		score = 100.0 - (deviation * -2.0) // -2.0 to amplify penalty
	} else {
		// Above ideal - penalize for risk
		score = 100.0 - (deviation * 1.5) // 1.5 to be less harsh
	}

	// Clamp between 0 and 100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// GetCostTrend returns cost trend analysis for a time period
func (a *Analyzer) GetCostTrend(ctx context.Context, nodeGroup, namespace string, period time.Duration) (*CostTrend, error) {
	end := time.Now()
	start := end.Add(-period)

	snapshots, err := a.storage.GetSnapshots(ctx, nodeGroup, namespace, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots found for period")
	}

	// Build data points
	var dataPoints []CostDataPoint
	var totalCost float64
	minCost := snapshots[0].Cost.TotalMonthly
	maxCost := snapshots[0].Cost.TotalMonthly

	for _, snapshot := range snapshots {
		dataPoints = append(dataPoints, CostDataPoint{
			Timestamp:   snapshot.Timestamp,
			HourlyCost:  snapshot.Cost.TotalHourly,
			MonthlyCost: snapshot.Cost.TotalMonthly,
			NodeCount:   snapshot.Cost.TotalNodes,
			Utilization: snapshot.Utilization,
		})

		totalCost += snapshot.Cost.TotalMonthly
		if snapshot.Cost.TotalMonthly < minCost {
			minCost = snapshot.Cost.TotalMonthly
		}
		if snapshot.Cost.TotalMonthly > maxCost {
			maxCost = snapshot.Cost.TotalMonthly
		}
	}

	avgCost := totalCost / float64(len(snapshots))

	// Determine trend direction
	trend := a.determineTrend(dataPoints)

	// Calculate change percentage
	firstCost := dataPoints[0].MonthlyCost
	lastCost := dataPoints[len(dataPoints)-1].MonthlyCost
	changePercent := float64(0)
	if firstCost > 0 {
		changePercent = ((lastCost - firstCost) / firstCost) * 100
	}

	return &CostTrend{
		NodeGroupName: nodeGroup,
		Namespace:     namespace,
		StartTime:     start,
		EndTime:       end,
		DataPoints:    dataPoints,
		AverageCost:   avgCost,
		MinCost:       minCost,
		MaxCost:       maxCost,
		Trend:         trend,
		ChangePercent: changePercent,
	}, nil
}

// determineTrend analyzes data points to determine trend direction
func (a *Analyzer) determineTrend(dataPoints []CostDataPoint) TrendDirection {
	if len(dataPoints) < 2 {
		return TrendStable
	}

	// Calculate linear regression slope
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(dataPoints))

	for i, point := range dataPoints {
		x := float64(i)
		y := point.MonthlyCost
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Prevent division by zero in slope calculation
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		// All x values are identical or single point - treat as stable
		return TrendStable
	}
	slope := (n*sumXY - sumX*sumY) / denominator

	// Calculate volatility (standard deviation)
	mean := sumY / n
	var sumSquaredDiff float64
	for _, point := range dataPoints {
		diff := point.MonthlyCost - mean
		sumSquaredDiff += diff * diff
	}

	// Prevent division by zero in standard deviation
	var stdDev float64
	if n > 0 {
		stdDev = sumSquaredDiff / n
	}

	// Determine trend based on slope and volatility
	volatilityThreshold := mean * 0.1 // 10% of mean
	if stdDev > volatilityThreshold {
		return TrendVolatile
	}

	slopeThreshold := mean * 0.01 // 1% of mean per data point
	if slope > slopeThreshold {
		return TrendIncreasing
	} else if slope < -slopeThreshold {
		return TrendDecreasing
	}

	return TrendStable
}

// AnalyzeUtilization analyzes resource utilization vs cost
func (a *Analyzer) AnalyzeUtilization(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) (*UtilizationAnalysis, error) {
	if nodeGroup == nil {
		return nil, fmt.Errorf("nodeGroup cannot be nil")
	}

	// Get recent snapshots (last 24 hours)
	end := time.Now()
	start := end.Add(-24 * time.Hour)

	snapshots, err := a.storage.GetSnapshots(ctx, nodeGroup.Name, nodeGroup.Namespace, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no utilization data available")
	}

	// Calculate average and peak utilization
	var avgCPU, avgMemory, avgDisk float64
	peakCPU := snapshots[0].Utilization.CPUPercent
	peakMemory := snapshots[0].Utilization.MemoryPercent
	peakDisk := snapshots[0].Utilization.DiskPercent

	for _, snapshot := range snapshots {
		avgCPU += snapshot.Utilization.CPUPercent
		avgMemory += snapshot.Utilization.MemoryPercent
		avgDisk += snapshot.Utilization.DiskPercent

		if snapshot.Utilization.CPUPercent > peakCPU {
			peakCPU = snapshot.Utilization.CPUPercent
		}
		if snapshot.Utilization.MemoryPercent > peakMemory {
			peakMemory = snapshot.Utilization.MemoryPercent
		}
		if snapshot.Utilization.DiskPercent > peakDisk {
			peakDisk = snapshot.Utilization.DiskPercent
		}
	}

	n := float64(len(snapshots))
	avgUtilization := ResourceUtilization{
		CPUPercent:    avgCPU / n,
		MemoryPercent: avgMemory / n,
		DiskPercent:   avgDisk / n,
		NodeCount:     snapshots[len(snapshots)-1].Cost.TotalNodes,
	}

	peakUtilization := ResourceUtilization{
		CPUPercent:    peakCPU,
		MemoryPercent: peakMemory,
		DiskPercent:   peakDisk,
		NodeCount:     snapshots[len(snapshots)-1].Cost.TotalNodes,
	}

	// Get current cost
	latestSnapshot := snapshots[len(snapshots)-1]
	cost := latestSnapshot.Cost

	// Calculate cost per resource
	var costPerCPUCore, costPerGBMemory, costPerGBDisk float64

	// Get offering details for first instance type
	for offeringID := range cost.InstanceTypes {
		offeringCost, err := a.calculator.GetOfferingCost(ctx, offeringID)
		if err == nil {
			if offeringCost.Specs.CPU > 0 {
				costPerCPUCore = offeringCost.MonthlyCost / float64(offeringCost.Specs.CPU)
			}
			if offeringCost.Specs.MemoryMB > 0 {
				costPerGBMemory = offeringCost.MonthlyCost / (float64(offeringCost.Specs.MemoryMB) / 1024.0)
			}
			if offeringCost.Specs.DiskGB > 0 {
				costPerGBDisk = offeringCost.MonthlyCost / float64(offeringCost.Specs.DiskGB)
			}
			break // Use first offering for calculation
		}
	}

	// Calculate efficiency score
	efficiencyScore := a.calculateEfficiencyScore(&cost, avgUtilization)

	// Estimate waste (unused resources)
	wastePercent := 100.0 - ((avgUtilization.CPUPercent + avgUtilization.MemoryPercent) / 2.0)
	wasteEstimate := cost.TotalMonthly * (wastePercent / 100.0)

	// Generate recommendations
	recommendations := a.generateUtilizationRecommendations(avgUtilization, peakUtilization)

	return &UtilizationAnalysis{
		NodeGroupName:      nodeGroup.Name,
		Namespace:          nodeGroup.Namespace,
		AverageUtilization: avgUtilization,
		PeakUtilization:    peakUtilization,
		CostPerCPUCore:     costPerCPUCore,
		CostPerGBMemory:    costPerGBMemory,
		CostPerGBDisk:      costPerGBDisk,
		EfficiencyScore:    efficiencyScore,
		WasteEstimate:      wasteEstimate,
		Recommendations:    recommendations,
		AnalyzedAt:         time.Now(),
	}, nil
}

// generateUtilizationRecommendations generates recommendations based on utilization
func (a *Analyzer) generateUtilizationRecommendations(avg, peak ResourceUtilization) []string {
	var recommendations []string

	// CPU recommendations
	if avg.CPUPercent < 30 {
		recommendations = append(recommendations, "CPU utilization is low (<30%). Consider downsizing to smaller instance types.")
	} else if avg.CPUPercent > 80 && peak.CPUPercent > 90 {
		recommendations = append(recommendations, "CPU utilization is high. Consider scaling up to larger instance types to avoid performance issues.")
	}

	// Memory recommendations
	if avg.MemoryPercent < 40 {
		recommendations = append(recommendations, "Memory utilization is low (<40%). Consider using memory-optimized instances or downsizing.")
	} else if avg.MemoryPercent > 85 && peak.MemoryPercent > 95 {
		recommendations = append(recommendations, "Memory utilization is very high. Immediate scale-up recommended to prevent OOM issues.")
	}

	// Balanced recommendations
	if avg.CPUPercent < 30 && avg.MemoryPercent < 40 {
		recommendations = append(recommendations, "Overall utilization is low. Significant cost savings possible through consolidation.")
	}

	// Spot instance recommendation
	if avg.CPUPercent > 20 && avg.CPUPercent < 70 && peak.CPUPercent < 80 {
		recommendations = append(recommendations, "Workload is stable and suitable for spot instances. Consider enabling spot instances for 60-80% cost savings.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Resource utilization is well-balanced. Current configuration is optimal.")
	}

	return recommendations
}

// ForecastCost forecasts future costs based on historical trends
func (a *Analyzer) ForecastCost(ctx context.Context, nodeGroup, namespace string, horizon time.Duration) (*CostForecast, error) {
	// Get trend for past period (use 2x horizon for prediction)
	lookback := horizon * 2
	trend, err := a.GetCostTrend(ctx, nodeGroup, namespace, lookback)
	if err != nil {
		return nil, fmt.Errorf("failed to get cost trend: %w", err)
	}

	if len(trend.DataPoints) < 2 {
		return nil, fmt.Errorf("insufficient data for forecasting")
	}

	// Use linear regression to predict
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(trend.DataPoints))

	for i, point := range trend.DataPoints {
		x := float64(i)
		y := point.MonthlyCost
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate slope and intercept
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	intercept := (sumY - slope*sumX) / n

	// Predict future value
	futureX := n + (horizon.Hours() / (lookback.Hours() / n))
	predictedCost := slope*futureX + intercept

	// Ensure non-negative
	if predictedCost < 0 {
		predictedCost = trend.AverageCost
	}

	// Calculate confidence level based on trend volatility
	confidence := 0.8
	if trend.Trend == TrendVolatile {
		confidence = 0.5
	} else if trend.Trend == TrendStable {
		confidence = 0.9
	}

	// Calculate bounds (±20% for high confidence, ±40% for low)
	variance := 0.2
	if confidence < 0.7 {
		variance = 0.4
	}

	upperBound := predictedCost * (1 + variance)
	lowerBound := predictedCost * (1 - variance)
	if lowerBound < 0 {
		lowerBound = 0
	}

	assumptions := []string{
		fmt.Sprintf("Based on %d data points over %.0f hours", len(trend.DataPoints), lookback.Hours()),
		fmt.Sprintf("Current trend: %s", trend.Trend),
		"Assumes no major changes in workload or configuration",
		"Assumes stable instance pricing",
	}

	return &CostForecast{
		NodeGroupName:   nodeGroup,
		Namespace:       namespace,
		ForecastHorizon: horizon,
		PredictedCost:   predictedCost,
		ConfidenceLevel: confidence,
		UpperBound:      upperBound,
		LowerBound:      lowerBound,
		Assumptions:     assumptions,
		GeneratedAt:     time.Now(),
	}, nil
}

// CleanupOldData removes old snapshots beyond retention period
func (a *Analyzer) CleanupOldData(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	return a.storage.DeleteOldSnapshots(ctx, cutoff)
}
