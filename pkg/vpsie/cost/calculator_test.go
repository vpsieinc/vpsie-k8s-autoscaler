package cost

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockVPSieClient is a mock implementation of the VPSieClient interface
type MockVPSieClient struct {
	offerings []client.Offering
}

func (m *MockVPSieClient) ListOfferings(ctx context.Context, opts *client.ListOptions) ([]client.Offering, error) {
	return m.offerings, nil
}

// Implement other required methods with no-ops
func (m *MockVPSieClient) GetOffering(ctx context.Context, id string) (*client.Offering, error) {
	for i := range m.offerings {
		if m.offerings[i].ID == id {
			return &m.offerings[i], nil
		}
	}
	return nil, client.ErrNotFound
}

func (m *MockVPSieClient) ListVPS(ctx context.Context, opts *client.ListOptions) ([]client.VPS, error) {
	return nil, nil
}

func (m *MockVPSieClient) GetVPS(ctx context.Context, id int) (*client.VPS, error) {
	return nil, nil
}

func (m *MockVPSieClient) CreateVPS(ctx context.Context, req *client.CreateVPSRequest) (*client.VPS, error) {
	return nil, nil
}

func (m *MockVPSieClient) DeleteVPS(ctx context.Context, id int) error {
	return nil
}

func (m *MockVPSieClient) UpdateVPS(ctx context.Context, id int, req *client.UpdateVPSRequest) (*client.VPS, error) {
	return nil, nil
}

func (m *MockVPSieClient) PerformVPSAction(ctx context.Context, id int, action *client.VPSAction) error {
	return nil
}

func (m *MockVPSieClient) ListDatacenters(ctx context.Context, opts *client.ListOptions) ([]client.Datacenter, error) {
	return nil, nil
}

func (m *MockVPSieClient) ListOSImages(ctx context.Context, opts *client.ListOptions) ([]client.OSImage, error) {
	return nil, nil
}

func (m *MockVPSieClient) Close() error {
	return nil
}

func TestNewCalculator(t *testing.T) {
	mockClient := &MockVPSieClient{}
	calc := NewCalculator(mockClient)

	if calc == nil {
		t.Fatal("NewCalculator returned nil")
	}

	if calc.client == nil {
		t.Error("Calculator client is nil")
	}

	if calc.cache == nil {
		t.Error("Calculator cache is nil")
	}
}

func TestGetOfferingCost(t *testing.T) {
	mockClient := &MockVPSieClient{
		offerings: []client.Offering{
			{
				ID:          "small-1",
				Name:        "Small Instance",
				CPU:         2,
				RAM:         2048,
				Disk:        50,
				Bandwidth:   1000,
				Price:       10.0,
				HourlyPrice: 0.015,
				Available:   true,
				Category:    "standard",
			},
			{
				ID:          "medium-1",
				Name:        "Medium Instance",
				CPU:         4,
				RAM:         4096,
				Disk:        100,
				Bandwidth:   2000,
				Price:       20.0,
				HourlyPrice: 0.030,
				Available:   true,
				Category:    "standard",
			},
		},
	}

	calc := NewCalculator(mockClient)
	ctx := context.Background()

	t.Run("Get existing offering", func(t *testing.T) {
		cost, err := calc.GetOfferingCost(ctx, "small-1")
		if err != nil {
			t.Fatalf("GetOfferingCost failed: %v", err)
		}

		if cost.OfferingID != "small-1" {
			t.Errorf("Expected offering ID 'small-1', got '%s'", cost.OfferingID)
		}

		if cost.MonthlyCost != 10.0 {
			t.Errorf("Expected monthly cost 10.0, got %f", cost.MonthlyCost)
		}

		if cost.HourlyCost != 0.015 {
			t.Errorf("Expected hourly cost 0.015, got %f", cost.HourlyCost)
		}

		if cost.DailyCost != 0.015*24 {
			t.Errorf("Expected daily cost %f, got %f", 0.015*24, cost.DailyCost)
		}

		if cost.Specs.CPU != 2 {
			t.Errorf("Expected 2 CPU cores, got %d", cost.Specs.CPU)
		}
	})

	t.Run("Get non-existent offering", func(t *testing.T) {
		_, err := calc.GetOfferingCost(ctx, "non-existent")
		if err == nil {
			t.Error("Expected error for non-existent offering, got nil")
		}
	})

	t.Run("Cache test", func(t *testing.T) {
		// First call - should hit API
		cost1, err := calc.GetOfferingCost(ctx, "medium-1")
		if err != nil {
			t.Fatalf("GetOfferingCost failed: %v", err)
		}

		// Second call - should hit cache
		cost2, err := calc.GetOfferingCost(ctx, "medium-1")
		if err != nil {
			t.Fatalf("GetOfferingCost failed: %v", err)
		}

		if cost1.LastUpdated != cost2.LastUpdated {
			t.Error("Cache not working - timestamps differ")
		}

		// Clear cache and call again
		calc.ClearCache()
		cost3, err := calc.GetOfferingCost(ctx, "medium-1")
		if err != nil {
			t.Fatalf("GetOfferingCost failed: %v", err)
		}

		if cost1.LastUpdated == cost3.LastUpdated {
			t.Error("Cache not cleared - timestamps should differ")
		}
	})
}

func TestCalculateNodeGroupCost(t *testing.T) {
	mockClient := &MockVPSieClient{
		offerings: []client.Offering{
			{
				ID:          "small-1",
				Name:        "Small Instance",
				CPU:         2,
				RAM:         2048,
				Disk:        50,
				Bandwidth:   1000,
				Price:       10.0,
				HourlyPrice: 0.015,
				Available:   true,
				Category:    "standard",
			},
		},
	}

	calc := NewCalculator(mockClient)
	ctx := context.Background()

	t.Run("Calculate cost for NodeGroup with no nodes", func(t *testing.T) {
		nodeGroup := &v1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-group",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeGroupSpec{
				MinNodes:    3,
				MaxNodes:    10,
				OfferingIDs: []string{"small-1"},
			},
			Status: v1alpha1.NodeGroupStatus{
				DesiredNodes: 3,
			},
		}

		cost, err := calc.CalculateNodeGroupCost(ctx, nodeGroup)
		if err != nil {
			t.Fatalf("CalculateNodeGroupCost failed: %v", err)
		}

		if cost.TotalNodes != 3 {
			t.Errorf("Expected 3 nodes, got %d", cost.TotalNodes)
		}

		expectedHourly := 0.015 * 3
		if cost.TotalHourly != expectedHourly {
			t.Errorf("Expected hourly cost %f, got %f", expectedHourly, cost.TotalHourly)
		}

		expectedMonthly := expectedHourly * 730
		if cost.TotalMonthly != expectedMonthly {
			t.Errorf("Expected monthly cost %f, got %f", expectedMonthly, cost.TotalMonthly)
		}
	})

	t.Run("Calculate cost for NodeGroup with actual nodes", func(t *testing.T) {
		nodeGroup := &v1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-group",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeGroupSpec{
				MinNodes:    3,
				MaxNodes:    10,
				OfferingIDs: []string{"small-1"},
			},
			Status: v1alpha1.NodeGroupStatus{
				CurrentNodes: 4,
				DesiredNodes: 4,
				Nodes: []v1alpha1.NodeInfo{
					{NodeName: "node-1", InstanceType: "small-1"},
					{NodeName: "node-2", InstanceType: "small-1"},
					{NodeName: "node-3", InstanceType: "small-1"},
					{NodeName: "node-4", InstanceType: "small-1"},
				},
			},
		}

		cost, err := calc.CalculateNodeGroupCost(ctx, nodeGroup)
		if err != nil {
			t.Fatalf("CalculateNodeGroupCost failed: %v", err)
		}

		if cost.TotalNodes != 4 {
			t.Errorf("Expected 4 nodes, got %d", cost.TotalNodes)
		}

		expectedHourly := 0.015 * 4
		if cost.TotalHourly != expectedHourly {
			t.Errorf("Expected hourly cost %f, got %f", expectedHourly, cost.TotalHourly)
		}
	})

	t.Run("Calculate cost for nil NodeGroup", func(t *testing.T) {
		_, err := calc.CalculateNodeGroupCost(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil NodeGroup, got nil")
		}
	})
}

func TestCompareOfferings(t *testing.T) {
	mockClient := &MockVPSieClient{
		offerings: []client.Offering{
			{
				ID:          "small-1",
				Name:        "Small Instance",
				Price:       10.0,
				HourlyPrice: 0.015,
				Available:   true,
			},
			{
				ID:          "medium-1",
				Name:        "Medium Instance",
				Price:       20.0,
				HourlyPrice: 0.030,
				Available:   true,
			},
			{
				ID:          "large-1",
				Name:        "Large Instance",
				Price:       40.0,
				HourlyPrice: 0.060,
				Available:   true,
			},
		},
	}

	calc := NewCalculator(mockClient)
	ctx := context.Background()

	t.Run("Compare three offerings", func(t *testing.T) {
		comparison, err := calc.CompareOfferings(ctx, []string{"small-1", "medium-1", "large-1"})
		if err != nil {
			t.Fatalf("CompareOfferings failed: %v", err)
		}

		if comparison.CheapestID != "small-1" {
			t.Errorf("Expected cheapest to be 'small-1', got '%s'", comparison.CheapestID)
		}

		if comparison.MostExpensiveID != "large-1" {
			t.Errorf("Expected most expensive to be 'large-1', got '%s'", comparison.MostExpensiveID)
		}

		expectedAvg := (10.0 + 20.0 + 40.0) / 3
		if comparison.AverageCost != expectedAvg {
			t.Errorf("Expected average cost %f, got %f", expectedAvg, comparison.AverageCost)
		}

		// Check sorted order
		if len(comparison.Offerings) != 3 {
			t.Fatalf("Expected 3 offerings, got %d", len(comparison.Offerings))
		}

		if comparison.Offerings[0].MonthlyCost > comparison.Offerings[1].MonthlyCost {
			t.Error("Offerings not sorted by cost")
		}
	})

	t.Run("Compare with empty list", func(t *testing.T) {
		_, err := calc.CompareOfferings(ctx, []string{})
		if err == nil {
			t.Error("Expected error for empty offerings list, got nil")
		}
	})
}

func TestCalculateSavings(t *testing.T) {
	current := &NodeGroupCost{
		NodeGroupName: "test-group",
		TotalNodes:    5,
		TotalMonthly:  100.0,
	}

	proposed := &NodeGroupCost{
		NodeGroupName: "test-group",
		TotalNodes:    5,
		TotalMonthly:  75.0,
	}

	calc := NewCalculator(&MockVPSieClient{})

	t.Run("Calculate savings with 25% reduction", func(t *testing.T) {
		savings, err := calc.CalculateSavings(current, proposed)
		if err != nil {
			t.Fatalf("CalculateSavings failed: %v", err)
		}

		if savings.MonthlySavings != 25.0 {
			t.Errorf("Expected monthly savings 25.0, got %f", savings.MonthlySavings)
		}

		if savings.AnnualSavings != 300.0 {
			t.Errorf("Expected annual savings 300.0, got %f", savings.AnnualSavings)
		}

		if savings.SavingsPercent != 25.0 {
			t.Errorf("Expected savings percent 25.0, got %f", savings.SavingsPercent)
		}

		if savings.RecommendedAction != "strongly_recommended" {
			t.Errorf("Expected action 'strongly_recommended', got '%s'", savings.RecommendedAction)
		}
	})

	t.Run("Calculate with nil costs", func(t *testing.T) {
		_, err := calc.CalculateSavings(nil, proposed)
		if err == nil {
			t.Error("Expected error for nil current cost, got nil")
		}

		_, err = calc.CalculateSavings(current, nil)
		if err == nil {
			t.Error("Expected error for nil proposed cost, got nil")
		}
	})
}

func TestFindCheapestOffering(t *testing.T) {
	mockClient := &MockVPSieClient{
		offerings: []client.Offering{
			{
				ID:          "small-1",
				Name:        "Small Instance",
				CPU:         2,
				RAM:         2048,
				Disk:        50,
				Bandwidth:   1000,
				Price:       10.0,
				HourlyPrice: 0.015,
				Available:   true,
			},
			{
				ID:          "medium-1",
				Name:        "Medium Instance",
				CPU:         4,
				RAM:         4096,
				Disk:        100,
				Bandwidth:   2000,
				Price:       20.0,
				HourlyPrice: 0.030,
				Available:   true,
			},
			{
				ID:          "large-1",
				Name:        "Large Instance",
				CPU:         8,
				RAM:         8192,
				Disk:        200,
				Bandwidth:   4000,
				Price:       40.0,
				HourlyPrice: 0.060,
				Available:   true,
			},
		},
	}

	calc := NewCalculator(mockClient)
	ctx := context.Background()

	t.Run("Find cheapest with minimal requirements", func(t *testing.T) {
		requirements := ResourceRequirements{
			MinCPU:      2,
			MinMemoryMB: 2048,
			MinDiskGB:   50,
		}

		rec, err := calc.FindCheapestOffering(ctx, requirements, nil)
		if err != nil {
			t.Fatalf("FindCheapestOffering failed: %v", err)
		}

		if rec.OfferingID != "small-1" {
			t.Errorf("Expected offering 'small-1', got '%s'", rec.OfferingID)
		}

		if rec.Confidence < 0.8 {
			t.Errorf("Expected confidence >= 0.8, got %f", rec.Confidence)
		}
	})

	t.Run("Find cheapest with higher requirements", func(t *testing.T) {
		requirements := ResourceRequirements{
			MinCPU:      4,
			MinMemoryMB: 4096,
			MinDiskGB:   100,
		}

		rec, err := calc.FindCheapestOffering(ctx, requirements, nil)
		if err != nil {
			t.Fatalf("FindCheapestOffering failed: %v", err)
		}

		if rec.OfferingID != "medium-1" {
			t.Errorf("Expected offering 'medium-1', got '%s'", rec.OfferingID)
		}
	})

	t.Run("Find cheapest with allowed list", func(t *testing.T) {
		requirements := ResourceRequirements{
			MinCPU:      2,
			MinMemoryMB: 2048,
			MinDiskGB:   50,
		}

		// Only allow medium and large
		allowed := []string{"medium-1", "large-1"}

		rec, err := calc.FindCheapestOffering(ctx, requirements, allowed)
		if err != nil {
			t.Fatalf("FindCheapestOffering failed: %v", err)
		}

		if rec.OfferingID != "medium-1" {
			t.Errorf("Expected offering 'medium-1', got '%s'", rec.OfferingID)
		}
	})
}

func TestCacheExpiration(t *testing.T) {
	mockClient := &MockVPSieClient{
		offerings: []client.Offering{
			{
				ID:          "test-1",
				Price:       10.0,
				HourlyPrice: 0.015,
				Available:   true,
			},
		},
	}

	calc := NewCalculator(mockClient)
	calc.SetCacheTTL(100 * time.Millisecond) // Very short TTL for testing
	ctx := context.Background()

	// Get cost - should cache it
	cost1, err := calc.GetOfferingCost(ctx, "test-1")
	if err != nil {
		t.Fatalf("GetOfferingCost failed: %v", err)
	}

	// Get again immediately - should hit cache
	cost2, err := calc.GetOfferingCost(ctx, "test-1")
	if err != nil {
		t.Fatalf("GetOfferingCost failed: %v", err)
	}

	if cost1.LastUpdated != cost2.LastUpdated {
		t.Error("Cache miss when should have hit")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Get again - should miss cache and refetch
	cost3, err := calc.GetOfferingCost(ctx, "test-1")
	if err != nil {
		t.Fatalf("GetOfferingCost failed: %v", err)
	}

	if cost1.LastUpdated == cost3.LastUpdated {
		t.Error("Cache hit when should have expired")
	}
}
