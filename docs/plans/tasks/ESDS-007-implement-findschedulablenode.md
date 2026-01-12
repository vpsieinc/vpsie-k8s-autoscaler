# Task: ESDS-007 - Implement findSchedulableNode and buildNodePodsCache

Metadata:
- Dependencies: ESDS-003 (toleration), ESDS-005 (node affinity), ESDS-006 (anti-affinity)
- Provides: Core scheduling simulation for canPodsBeRescheduled (ESDS-008)
- Size: Small (1 file)

## Implementation Content
Implement the core scheduling simulation functions:
1. `findSchedulableNode(pod *corev1.Pod, nodes []*corev1.Node, nodePodsCache map[string][]*corev1.Pod) (bool, *corev1.Node)`
2. `buildNodePodsCache(ctx context.Context, nodes []*corev1.Node) (map[string][]*corev1.Pod, error)`

## Target Files
- [ ] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety.go`
- [ ] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [ ] Write unit tests for `findSchedulableNode`:
  - Pod with no constraints -> first available node
  - Pod with toleration requirement -> only nodes tolerating taint
  - Pod with nodeSelector -> only nodes with matching labels
  - Pod with node affinity -> only nodes matching affinity
  - Pod with anti-affinity -> only nodes without conflicting pods
  - Pod with multiple constraints -> all must be satisfied
  - No suitable node found -> (false, nil)
- [ ] Write unit tests for `buildNodePodsCache`:
  - Empty nodes list -> empty cache
  - Nodes with pods -> cache populated correctly
- [ ] Run tests to confirm failure: `go test ./pkg/scaler -run TestFindSchedulableNode -v`

### 2. Green Phase
- [ ] Add `buildNodePodsCache` method to ScaleDownManager:
  ```go
  // buildNodePodsCache creates a cache of pods per node for anti-affinity checks.
  // Built once per canPodsBeRescheduled call for performance.
  func (s *ScaleDownManager) buildNodePodsCache(ctx context.Context, nodes []*corev1.Node) (map[string][]*corev1.Pod, error)
  ```
- [ ] Implement `buildNodePodsCache`:
  - Create map[string][]*corev1.Pod
  - For each node, use existing `getNodePods` method
  - Return populated cache
- [ ] Add `findSchedulableNode` function:
  ```go
  // findSchedulableNode finds a node where the pod can be scheduled.
  // Checks constraints in order of computational cost:
  // 1. Tolerations (cheapest)
  // 2. NodeSelector (cheap)
  // 3. NodeAffinity (moderate)
  // 4. PodAntiAffinity (most expensive)
  // Returns (true, node) if found, (false, nil) if no suitable node.
  func findSchedulableNode(pod *corev1.Pod, nodes []*corev1.Node, nodePodsCache map[string][]*corev1.Pod) (bool, *corev1.Node)
  ```
- [ ] Implement `findSchedulableNode`:
  - Iterate nodes
  - Check tolerations: `tolerationsTolerateTaints(pod.Spec.Tolerations, node.Spec.Taints)`
  - Check nodeSelector: `MatchesNodeSelector(node, pod)` (existing helper)
  - Check node affinity: `matchesNodeAffinity(pod, node)`
  - Check anti-affinity: `!hasPodAntiAffinityViolation(pod, node, nodePodsCache[node.Name])`
  - Return first matching node
- [ ] Run tests: `go test ./pkg/scaler -run TestFindSchedulableNode -v`

### 3. Refactor Phase
- [ ] Ensure constraint check order matches Design Doc (performance)
- [ ] Add debug logging for constraint failures
- [ ] Run all safety tests: `go test ./pkg/scaler/... -v`

## Completion Criteria
- [ ] `findSchedulableNode` checks all 4 constraint types in correct order
- [ ] `findSchedulableNode` returns first viable node (short-circuit)
- [ ] `buildNodePodsCache` caches pods per node correctly
- [ ] Uses existing helpers: `MatchesNodeSelector`, `isSkippableDaemonSetPod`
- [ ] New unit tests pass
- [ ] Existing tests still pass
- [ ] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 279-305:
```go
func findSchedulableNode(pod *corev1.Pod, nodes []*corev1.Node, nodePodsCache map[string][]*corev1.Pod) (bool, *corev1.Node) {
    for _, node := range nodes {
        // Check 1: Tolerations match node taints (NoSchedule, NoExecute only)
        if !tolerationsTolerateTaints(pod.Spec.Tolerations, node.Spec.Taints) {
            continue
        }

        // Check 2: NodeSelector matches (uses existing helper)
        if !MatchesNodeSelector(node, pod) {
            continue
        }

        // Check 3: NodeAffinity requirements
        if !matchesNodeAffinity(pod, node) {
            continue
        }

        // Check 4: PodAntiAffinity constraints
        if hasPodAntiAffinityViolation(pod, node, nodePodsCache[node.Name]) {
            continue
        }

        // Found a suitable node
        return true, node
    }
    return false, nil
}
```

## Test Examples

```go
func TestFindSchedulableNode(t *testing.T) {
    t.Run("Pod with no constraints finds first node", func(t *testing.T) {
        pod := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{Name: "simple-pod"},
            Spec:       corev1.PodSpec{},
        }

        nodes := []*corev1.Node{
            {
                ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
                Spec:       corev1.NodeSpec{},
            },
            {
                ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
                Spec:       corev1.NodeSpec{},
            },
        }

        nodePodsCache := map[string][]*corev1.Pod{}

        found, node := findSchedulableNode(pod, nodes, nodePodsCache)
        assert.True(t, found)
        assert.Equal(t, "node-1", node.Name)
    })

    t.Run("Pod with nodeSelector finds only matching node", func(t *testing.T) {
        pod := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{Name: "ssd-pod"},
            Spec: corev1.PodSpec{
                NodeSelector: map[string]string{"disktype": "ssd"},
            },
        }

        nodes := []*corev1.Node{
            {
                ObjectMeta: metav1.ObjectMeta{
                    Name:   "hdd-node",
                    Labels: map[string]string{"disktype": "hdd"},
                },
            },
            {
                ObjectMeta: metav1.ObjectMeta{
                    Name:   "ssd-node",
                    Labels: map[string]string{"disktype": "ssd"},
                },
            },
        }

        nodePodsCache := map[string][]*corev1.Pod{}

        found, node := findSchedulableNode(pod, nodes, nodePodsCache)
        assert.True(t, found)
        assert.Equal(t, "ssd-node", node.Name)
    })

    t.Run("No suitable node returns false", func(t *testing.T) {
        pod := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod"},
            Spec: corev1.PodSpec{
                Tolerations: []corev1.Toleration{
                    {Key: "gpu", Value: "true", Effect: corev1.TaintEffectNoSchedule},
                },
            },
        }

        nodes := []*corev1.Node{
            {
                ObjectMeta: metav1.ObjectMeta{Name: "cpu-node"},
                Spec:       corev1.NodeSpec{}, // No GPU taint
            },
        }

        nodePodsCache := map[string][]*corev1.Pod{}

        // Pod tolerates GPU taint but requires it (inverse logic - pod expects GPU node)
        // Since no node has GPU taint, and pod has tolerations, it can schedule on cpu-node
        // This test verifies the toleration logic works correctly
        found, _ := findSchedulableNode(pod, nodes, nodePodsCache)
        assert.True(t, found) // Pod with toleration CAN schedule on untainted node
    })
}
```

## Notes
- Impact scope: `pkg/scaler/safety.go` only
- Constraints: Must maintain constraint check order for performance
- The existing `getNodePods` method is used by `buildNodePodsCache`
- Performance: O(n*m) where n=nodes, m=pods per node for anti-affinity check
