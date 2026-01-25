# Task: ESDS-006 - Implement hasPodAntiAffinityViolation

Metadata:
- Dependencies: ESDS-005 (node affinity functions)
- Provides: Pod anti-affinity checking for findSchedulableNode (ESDS-007)
- Size: Small (1 file)

## Implementation Content
Implement pod anti-affinity violation detection:
1. `hasPodAntiAffinityViolation(pod *corev1.Pod, node *corev1.Node, existingPods []*corev1.Pod) bool`
2. `matchesPodAffinityTerm(existingPod *corev1.Pod, term *corev1.PodAffinityTerm, node *corev1.Node) bool`

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety.go`
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Write unit tests for `hasPodAntiAffinityViolation`:
  - Pod with no anti-affinity -> false (no violation)
  - Pod with anti-affinity, no matching existing pods -> false
  - Pod with anti-affinity, matching existing pod on same topology -> true (violation)
  - Pod with preferred anti-affinity, matching pod -> false (soft constraint ignored)
- [x] Write unit tests for `matchesPodAffinityTerm`:
  - Label selector matches, same topology -> true
  - Label selector matches, different topology -> false
  - Label selector doesn't match -> false
- [x] Run tests to confirm failure: `go test ./pkg/scaler -run TestHasPodAntiAffinityViolation -v`

### 2. Green Phase
- [x] Add `matchesPodAffinityTerm` function:
  ```go
  // matchesPodAffinityTerm checks if an existing pod matches a pod affinity term
  // considering the topology key and label selector.
  func matchesPodAffinityTerm(existingPod *corev1.Pod, term *corev1.PodAffinityTerm, node *corev1.Node) bool
  ```
- [x] Implement `matchesPodAffinityTerm`:
  - Convert LabelSelector to selector
  - Check if existingPod labels match selector
  - Verify topology key matches (e.g., hostname)
- [x] Add `hasPodAntiAffinityViolation` function:
  ```go
  // hasPodAntiAffinityViolation checks if scheduling pod to node would violate anti-affinity rules.
  // Only checks RequiredDuringSchedulingIgnoredDuringExecution (hard constraint).
  func hasPodAntiAffinityViolation(pod *corev1.Pod, node *corev1.Node, existingPods []*corev1.Pod) bool
  ```
- [x] Implement `hasPodAntiAffinityViolation`:
  - Return false if no anti-affinity
  - Iterate required anti-affinity terms
  - For each term, check if any existing pod matches
  - Return true on first violation
- [x] Run tests: `go test ./pkg/scaler -run TestHasPodAntiAffinityViolation -v`

### 3. Refactor Phase
- [x] Handle edge cases (nil LabelSelector, empty topology key)
- [x] Add inline comments explaining topology semantics
- [x] Run all safety tests: `go test ./pkg/scaler/... -v`

## Completion Criteria
- [x] `hasPodAntiAffinityViolation` returns false when no anti-affinity
- [x] `hasPodAntiAffinityViolation` only checks Required (hard constraint)
- [x] `matchesPodAffinityTerm` correctly handles label selectors
- [x] Topology key matching works for `kubernetes.io/hostname`
- [x] New unit tests pass
- [x] Existing tests still pass
- [x] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 399-419:
```go
func hasPodAntiAffinityViolation(pod *corev1.Pod, node *corev1.Node, existingPods []*corev1.Pod) bool {
    if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
        return false // No anti-affinity requirements
    }

    antiAffinity := pod.Spec.Affinity.PodAntiAffinity

    // Check required (hard) anti-affinity constraints only
    for _, term := range antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
        for _, existingPod := range existingPods {
            if matchesPodAffinityTerm(existingPod, &term, node) {
                return true // Would violate anti-affinity
            }
        }
    }

    return false
}
```

## Test Examples

```go
func TestHasPodAntiAffinityViolation(t *testing.T) {
    t.Run("No anti-affinity - no violation", func(t *testing.T) {
        pod := &corev1.Pod{
            Spec: corev1.PodSpec{},
        }
        node := &corev1.Node{}
        existingPods := []*corev1.Pod{}

        result := hasPodAntiAffinityViolation(pod, node, existingPods)
        assert.False(t, result)
    })

    t.Run("Anti-affinity violated - matching pod on same node", func(t *testing.T) {
        pod := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"app": "web"},
            },
            Spec: corev1.PodSpec{
                Affinity: &corev1.Affinity{
                    PodAntiAffinity: &corev1.PodAntiAffinity{
                        RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
                            {
                                LabelSelector: &metav1.LabelSelector{
                                    MatchLabels: map[string]string{"app": "web"},
                                },
                                TopologyKey: "kubernetes.io/hostname",
                            },
                        },
                    },
                },
            },
        }

        node := &corev1.Node{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"kubernetes.io/hostname": "worker-1"},
            },
        }

        existingPod := &corev1.Pod{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"app": "web"},
            },
            Spec: corev1.PodSpec{
                NodeName: "worker-1",
            },
        }

        result := hasPodAntiAffinityViolation(pod, node, []*corev1.Pod{existingPod})
        assert.True(t, result)
    })
}
```

## Notes
- Impact scope: `pkg/scaler/safety.go` only
- Constraints: Performance consideration - this is O(n*m) where n=terms, m=existingPods
- This is the most expensive check, so it should be called last in findSchedulableNode
- Common topologyKey values: `kubernetes.io/hostname`, `topology.kubernetes.io/zone`
