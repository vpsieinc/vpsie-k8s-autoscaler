//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// BenchmarkScaleUp measures the performance of scale-up operations
func BenchmarkScaleUp(b *testing.B) {
	scenarios := []struct {
		name     string
		minNodes int32
		maxNodes int32
		podCount int
	}{
		{"Small_1to3", 1, 3, 2},
		{"Medium_3to10", 3, 10, 7},
		{"Large_10to50", 10, 50, 40},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			ctx := context.Background()

			// Start mock server once for all iterations
			mockServer := NewMockVPSieServer()
			mockServer.StateTransitions = []VMStateTransition{
				{FromState: "provisioning", ToState: "running", Duration: 100 * time.Millisecond},
				{FromState: "running", ToState: "ready", Duration: 50 * time.Millisecond},
			}
			defer mockServer.Close()

			// Create secret
			secretName := fmt.Sprintf("bench-secret-%s", scenario.name)
			err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
			require.NoError(b, err)
			defer func() {
				_ = deleteTestSecret(ctx, secretName, testNamespace)
			}()

			// Start controller
			proc, err := startControllerInBackground(13000, 13100, secretName, testNamespace)
			require.NoError(b, err)
			defer cleanup(proc)

			// Wait for controller to be ready
			require.Eventually(b, func() bool {
				return proc.IsHealthy()
			}, 30*time.Second, 1*time.Second)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				nodeGroupName := fmt.Sprintf("bench-ng-%s-%d", scenario.name, i)

				// Create NodeGroup
				nodeGroup := &autoscalerv1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeGroupName,
						Namespace: testNamespace,
					},
					Spec: autoscalerv1alpha1.NodeGroupSpec{
						MinNodes:           scenario.minNodes,
						MaxNodes:           scenario.maxNodes,
						DatacenterID:       "us-west-1",
						OfferingIDs:        []string{"standard-4cpu-8gb"},
						ResourceIdentifier: "test-cluster",
						Project:            "test-project",
						OSImageID:          "test-os-image",
						KubernetesVersion:  "v1.28.0",
					},
				}

				err := k8sClient.Create(ctx, nodeGroup)
				require.NoError(b, err)

				// Create unschedulable pods
				for j := 0; j < scenario.podCount; j++ {
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-pod-%d", nodeGroupName, j),
							Namespace: testNamespace,
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"nodegroup": nodeGroupName,
							},
							Containers: []corev1.Container{
								{
									Name:  "bench",
									Image: "busybox",
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1"),
											corev1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					}
					err := k8sClient.Create(ctx, pod)
					require.NoError(b, err)
				}

				// Measure time to scale
				startTime := time.Now()
				expectedNodes := scenario.minNodes + int32(scenario.podCount)
				if expectedNodes > scenario.maxNodes {
					expectedNodes = scenario.maxNodes
				}

				// Wait for scaling
				err = waitForNodeGroupScaling(ctx, testNamespace, nodeGroupName,
					int(expectedNodes), 2*time.Minute)
				require.NoError(b, err)

				scaleTime := time.Since(startTime)
				b.ReportMetric(float64(scaleTime.Milliseconds()), "ms/scale")
				b.ReportMetric(float64(scenario.podCount)/scaleTime.Seconds(), "nodes/sec")

				// Cleanup for next iteration
				cleanupScalingTest(ctx, testNamespace, nodeGroupName)
			}
		})
	}
}

// BenchmarkConcurrentScaling measures performance with multiple NodeGroups scaling concurrently
func BenchmarkConcurrentScaling(b *testing.B) {
	concurrencyLevels := []int{2, 5, 10}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", concurrency), func(b *testing.B) {
			ctx := context.Background()

			// Start mock server
			mockServer := NewMockVPSieServer()
			mockServer.StateTransitions = []VMStateTransition{
				{FromState: "provisioning", ToState: "ready", Duration: 200 * time.Millisecond},
			}
			defer mockServer.Close()

			// Create secret
			secretName := fmt.Sprintf("bench-concurrent-secret-%d", concurrency)
			err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
			require.NoError(b, err)
			defer func() {
				_ = deleteTestSecret(ctx, secretName, testNamespace)
			}()

			// Start controller
			proc, err := startControllerInBackground(13002, 13102, secretName, testNamespace)
			require.NoError(b, err)
			defer cleanup(proc)

			// Wait for controller
			require.Eventually(b, func() bool {
				return proc.IsHealthy()
			}, 30*time.Second, 1*time.Second)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				startTime := time.Now()

				// Create and scale multiple NodeGroups concurrently
				for j := 0; j < concurrency; j++ {
					wg.Add(1)
					go func(idx int) {
						defer wg.Done()

						nodeGroupName := fmt.Sprintf("bench-concurrent-%d-%d", i, idx)

						// Create NodeGroup
						nodeGroup := &autoscalerv1alpha1.NodeGroup{
							ObjectMeta: metav1.ObjectMeta{
								Name:      nodeGroupName,
								Namespace: testNamespace,
							},
							Spec: autoscalerv1alpha1.NodeGroupSpec{
								MinNodes:           1,
								MaxNodes:           3,
								DatacenterID:       "us-west-1",
								OfferingIDs:        []string{"standard-2cpu-4gb"},
								ResourceIdentifier: "test-cluster",
								Project:            "test-project",
								OSImageID:          "test-os-image",
								KubernetesVersion:  "v1.28.0",
							},
						}

						_ = k8sClient.Create(ctx, nodeGroup)

						// Wait for initial node
						_ = waitForNodeGroupScaling(ctx, testNamespace, nodeGroupName, 1, 30*time.Second)

						// Trigger scale-up
						for k := 0; k < 2; k++ {
							pod := &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      fmt.Sprintf("%s-pod-%d", nodeGroupName, k),
									Namespace: testNamespace,
								},
								Spec: corev1.PodSpec{
									NodeSelector: map[string]string{
										"nodegroup": nodeGroupName,
									},
									Containers: []corev1.Container{
										{
											Name:  "bench",
											Image: "busybox",
										},
									},
								},
							}
							_ = k8sClient.Create(ctx, pod)
						}

						// Wait for scale-up
						_ = waitForNodeGroupScaling(ctx, testNamespace, nodeGroupName, 3, 60*time.Second)

						// Cleanup
						cleanupScalingTest(ctx, testNamespace, nodeGroupName)
					}(j)
				}

				wg.Wait()
				totalTime := time.Since(startTime)

				b.ReportMetric(float64(totalTime.Milliseconds()), "ms/concurrent-scale")
				b.ReportMetric(float64(concurrency*3)/totalTime.Seconds(), "total-nodes/sec")
			}
		})
	}
}

// BenchmarkAPILatency measures the impact of API latency on scaling performance
func BenchmarkAPILatency(b *testing.B) {
	latencies := []time.Duration{
		0,
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
	}

	for _, latency := range latencies {
		b.Run(fmt.Sprintf("Latency_%dms", latency.Milliseconds()), func(b *testing.B) {
			ctx := context.Background()

			// Start mock server with specific latency
			mockServer := NewMockVPSieServer()
			mockServer.Latency = latency
			mockServer.StateTransitions = []VMStateTransition{
				{FromState: "provisioning", ToState: "ready", Duration: 100 * time.Millisecond},
			}
			defer mockServer.Close()

			// Create secret
			secretName := fmt.Sprintf("bench-latency-secret-%d", latency.Milliseconds())
			err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
			require.NoError(b, err)
			defer func() {
				_ = deleteTestSecret(ctx, secretName, testNamespace)
			}()

			// Start controller
			proc, err := startControllerInBackground(13004, 13104, secretName, testNamespace)
			require.NoError(b, err)
			defer cleanup(proc)

			// Wait for controller
			require.Eventually(b, func() bool {
				return proc.IsHealthy()
			}, 30*time.Second, 1*time.Second)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				nodeGroupName := fmt.Sprintf("bench-latency-%d-%d", latency.Milliseconds(), i)

				// Create NodeGroup
				nodeGroup := &autoscalerv1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeGroupName,
						Namespace: testNamespace,
					},
					Spec: autoscalerv1alpha1.NodeGroupSpec{
						MinNodes:           1,
						MaxNodes:           5,
						DatacenterID:       "us-west-1",
						OfferingIDs:        []string{"standard-2cpu-4gb"},
						ResourceIdentifier: "test-cluster",
						Project:            "test-project",
						OSImageID:          "test-os-image",
						KubernetesVersion:  "v1.28.0",
					},
				}

				err := k8sClient.Create(ctx, nodeGroup)
				require.NoError(b, err)

				// Measure provisioning time
				startTime := time.Now()
				err = waitForNodeGroupScaling(ctx, testNamespace, nodeGroupName, 1, 2*time.Minute)
				require.NoError(b, err)
				provisionTime := time.Since(startTime)

				// Create pods to trigger scale-up
				for j := 0; j < 2; j++ {
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-pod-%d", nodeGroupName, j),
							Namespace: testNamespace,
						},
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"nodegroup": nodeGroupName,
							},
							Containers: []corev1.Container{
								{
									Name:  "bench",
									Image: "busybox",
								},
							},
						},
					}
					_ = k8sClient.Create(ctx, pod)
				}

				// Measure scale-up time
				scaleStartTime := time.Now()
				err = waitForNodeGroupScaling(ctx, testNamespace, nodeGroupName, 3, 2*time.Minute)
				require.NoError(b, err)
				scaleTime := time.Since(scaleStartTime)

				b.ReportMetric(float64(provisionTime.Milliseconds()), "ms/initial-provision")
				b.ReportMetric(float64(scaleTime.Milliseconds()), "ms/scale-up")
				b.ReportMetric(float64(latency.Milliseconds()), "ms/api-latency")

				// Cleanup
				cleanupScalingTest(ctx, testNamespace, nodeGroupName)
			}
		})
	}
}

// BenchmarkResourceUsage measures controller resource usage during scaling
func BenchmarkResourceUsage(b *testing.B) {
	b.Run("MemoryUsage", func(b *testing.B) {
		ctx := context.Background()

		// Start mock server
		mockServer := NewMockVPSieServer()
		defer mockServer.Close()

		// Create secret
		secretName := "bench-memory-secret"
		err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
		require.NoError(b, err)
		defer func() {
			_ = deleteTestSecret(ctx, secretName, testNamespace)
		}()

		// Start controller
		proc, err := startControllerInBackground(13006, 13106, secretName, testNamespace)
		require.NoError(b, err)
		defer cleanup(proc)

		// Wait for controller
		require.Eventually(b, func() bool {
			return proc.IsHealthy()
		}, 30*time.Second, 1*time.Second)

		// Get baseline memory
		runtime.GC()
		var baselineMem runtime.MemStats
		runtime.ReadMemStats(&baselineMem)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Create many NodeGroups to stress memory
			var nodeGroups []*autoscalerv1alpha1.NodeGroup
			for j := 0; j < 10; j++ {
				nodeGroup := &autoscalerv1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("bench-mem-ng-%d-%d", i, j),
						Namespace: testNamespace,
					},
					Spec: autoscalerv1alpha1.NodeGroupSpec{
						MinNodes:           1,
						MaxNodes:           5,
						DatacenterID:       "us-west-1",
						OfferingIDs:        []string{"standard-2cpu-4gb"},
						ResourceIdentifier: "test-cluster",
						Project:            "test-project",
						OSImageID:          "test-os-image",
						KubernetesVersion:  "v1.28.0",
					},
				}
				err := k8sClient.Create(ctx, nodeGroup)
				require.NoError(b, err)
				nodeGroups = append(nodeGroups, nodeGroup)
			}

			// Wait for all to provision
			time.Sleep(5 * time.Second)

			// Measure memory after scaling
			runtime.GC()
			var afterMem runtime.MemStats
			runtime.ReadMemStats(&afterMem)

			memoryUsed := afterMem.Alloc - baselineMem.Alloc
			b.ReportMetric(float64(memoryUsed)/1024/1024, "MB/10-nodegroups")
			b.ReportMetric(float64(afterMem.NumGoroutine), "goroutines")

			// Cleanup
			for _, ng := range nodeGroups {
				_ = k8sClient.Delete(ctx, ng)
			}

			// Clean up VPSieNodes
			vpsieNodeList := &autoscalerv1alpha1.VPSieNodeList{}
			_ = k8sClient.List(ctx, vpsieNodeList, client.InNamespace(testNamespace))
			for _, node := range vpsieNodeList.Items {
				_ = k8sClient.Delete(ctx, &node)
			}
		}
	})
}

// BenchmarkReconciliationLoop measures the performance of the reconciliation loop
func BenchmarkReconciliationLoop(b *testing.B) {
	nodeGroupSizes := []int{1, 10, 50, 100}

	for _, size := range nodeGroupSizes {
		b.Run(fmt.Sprintf("NodeGroups_%d", size), func(b *testing.B) {
			ctx := context.Background()

			// Start mock server
			mockServer := NewMockVPSieServer()
			defer mockServer.Close()

			// Create secret
			secretName := fmt.Sprintf("bench-reconcile-secret-%d", size)
			err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
			require.NoError(b, err)
			defer func() {
				_ = deleteTestSecret(ctx, secretName, testNamespace)
			}()

			// Start controller
			proc, err := startControllerInBackground(13008, 13108, secretName, testNamespace)
			require.NoError(b, err)
			defer cleanup(proc)

			// Wait for controller
			require.Eventually(b, func() bool {
				return proc.IsHealthy()
			}, 30*time.Second, 1*time.Second)

			// Create NodeGroups
			for i := 0; i < size; i++ {
				nodeGroup := &autoscalerv1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("bench-reconcile-ng-%d", i),
						Namespace: testNamespace,
					},
					Spec: autoscalerv1alpha1.NodeGroupSpec{
						MinNodes:           1,
						MaxNodes:           3,
						DatacenterID:       "us-west-1",
						OfferingIDs:        []string{"standard-2cpu-4gb"},
						ResourceIdentifier: "test-cluster",
						Project:            "test-project",
						OSImageID:          "test-os-image",
						KubernetesVersion:  "v1.28.0",
					},
				}
				err := k8sClient.Create(ctx, nodeGroup)
				require.NoError(b, err)
			}

			// Let initial reconciliation complete
			time.Sleep(10 * time.Second)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Trigger reconciliation by updating a NodeGroup
				ng := &autoscalerv1alpha1.NodeGroup{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      "bench-reconcile-ng-0",
					Namespace: testNamespace,
				}, ng)
				require.NoError(b, err)

				// Make a minor change
				ng.Spec.Notes = fmt.Sprintf("bench-iteration-%d", i)
				startTime := time.Now()
				err = k8sClient.Update(ctx, ng)
				require.NoError(b, err)

				// Wait for reconciliation to process
				time.Sleep(100 * time.Millisecond)
				reconcileTime := time.Since(startTime)

				b.ReportMetric(float64(reconcileTime.Microseconds()), "μs/reconcile")
				b.ReportMetric(float64(size), "nodegroups")
			}

			// Cleanup
			nodeGroupList := &autoscalerv1alpha1.NodeGroupList{}
			_ = k8sClient.List(ctx, nodeGroupList, client.InNamespace(testNamespace))
			for _, ng := range nodeGroupList.Items {
				_ = k8sClient.Delete(ctx, &ng)
			}
		})
	}
}

// BenchmarkStateTransitions measures the performance of VM state transitions
func BenchmarkStateTransitions(b *testing.B) {
	transitionConfigs := []struct {
		name        string
		transitions []VMStateTransition
	}{
		{
			name: "Fast",
			transitions: []VMStateTransition{
				{FromState: "provisioning", ToState: "running", Duration: 10 * time.Millisecond},
				{FromState: "running", ToState: "ready", Duration: 10 * time.Millisecond},
			},
		},
		{
			name: "Normal",
			transitions: []VMStateTransition{
				{FromState: "provisioning", ToState: "running", Duration: 500 * time.Millisecond},
				{FromState: "running", ToState: "ready", Duration: 300 * time.Millisecond},
			},
		},
		{
			name: "Slow",
			transitions: []VMStateTransition{
				{FromState: "provisioning", ToState: "running", Duration: 2 * time.Second},
				{FromState: "running", ToState: "ready", Duration: 1 * time.Second},
			},
		},
	}

	for _, config := range transitionConfigs {
		b.Run(config.name, func(b *testing.B) {
			ctx := context.Background()

			// Start mock server with specific transitions
			mockServer := NewMockVPSieServer()
			mockServer.StateTransitions = config.transitions
			defer mockServer.Close()

			// Create secret
			secretName := fmt.Sprintf("bench-transition-%s", config.name)
			err := createTestSecret(ctx, secretName, testNamespace, mockServer.URL())
			require.NoError(b, err)
			defer func() {
				_ = deleteTestSecret(ctx, secretName, testNamespace)
			}()

			// Start controller
			proc, err := startControllerInBackground(13010, 13110, secretName, testNamespace)
			require.NoError(b, err)
			defer cleanup(proc)

			// Wait for controller
			require.Eventually(b, func() bool {
				return proc.IsHealthy()
			}, 30*time.Second, 1*time.Second)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				nodeGroupName := fmt.Sprintf("bench-transition-%s-%d", config.name, i)

				// Create NodeGroup
				nodeGroup := &autoscalerv1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeGroupName,
						Namespace: testNamespace,
					},
					Spec: autoscalerv1alpha1.NodeGroupSpec{
						MinNodes:           3,
						MaxNodes:           3,
						DatacenterID:       "us-west-1",
						OfferingIDs:        []string{"standard-2cpu-4gb"},
						ResourceIdentifier: "test-cluster",
						Project:            "test-project",
						OSImageID:          "test-os-image",
						KubernetesVersion:  "v1.28.0",
					},
				}

				err := k8sClient.Create(ctx, nodeGroup)
				require.NoError(b, err)

				// Measure time for all nodes to become ready
				startTime := time.Now()
				err = waitForAllNodesReady(ctx, testNamespace, nodeGroupName, 2*time.Minute)
				require.NoError(b, err)
				readyTime := time.Since(startTime)

				b.ReportMetric(float64(readyTime.Milliseconds()), "ms/all-nodes-ready")

				// Calculate transition overhead
				expectedTime := time.Duration(0)
				for _, t := range config.transitions {
					expectedTime += t.Duration
				}
				overhead := readyTime - (expectedTime * 3) // 3 nodes
				b.ReportMetric(float64(overhead.Milliseconds()), "ms/transition-overhead")

				// Cleanup
				cleanupScalingTest(ctx, testNamespace, nodeGroupName)
			}
		})
	}
}
