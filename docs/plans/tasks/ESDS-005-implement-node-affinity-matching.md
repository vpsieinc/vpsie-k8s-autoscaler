# Task: ESDS-005 - Implement matchesNodeAffinity and matchesNodeSelectorTerms

Metadata:
- Dependencies: ESDS-003 (toleration functions complete)
- Provides: Node affinity matching for findSchedulableNode (ESDS-007)
- Size: Small (1 file)

## Implementation Content
Implement node affinity matching functions:
1. `matchesNodeAffinity(pod *corev1.Pod, node *corev1.Node) bool` - Check pod's node affinity against node
2. `matchesNodeSelectorTerms(node *corev1.Node, terms []corev1.NodeSelectorTerm) bool` - Match selector terms
3. `matchNodeSelectorRequirement(node *corev1.Node, req *corev1.NodeSelectorRequirement) bool` - Match single requirement

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety.go`
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Write unit tests for `matchesNodeAffinity`:
  - Pod with no affinity -> true
  - Pod with required node affinity matching node labels -> true
  - Pod with required node affinity NOT matching node labels -> false
  - Pod with only preferred affinity -> true (soft constraint ignored)
- [x] Write unit tests for `matchNodeSelectorRequirement`:
  - In operator with matching value -> true
  - In operator with non-matching value -> false
  - NotIn operator with matching value -> false
  - Exists operator with key present -> true
  - DoesNotExist operator with key absent -> true
- [x] Run tests to confirm failure: `go test ./pkg/scaler -run TestMatchesNodeAffinity -v`

### 2. Green Phase
- [x] Add `matchNodeSelectorRequirement` function:
  ```go
  // matchNodeSelectorRequirement checks if a node satisfies a single node selector requirement.
  // Supports operators: In, NotIn, Exists, DoesNotExist, Gt, Lt
  func matchNodeSelectorRequirement(node *corev1.Node, req *corev1.NodeSelectorRequirement) bool
  ```
- [x] Implement operators:
  - In: node label value is in requirement values
  - NotIn: node label value is NOT in requirement values
  - Exists: node has the label key (any value)
  - DoesNotExist: node does NOT have the label key
  - Gt/Lt: numeric comparison (optional - can skip for v1)
- [x] Add `matchesNodeSelectorTerms` function:
  ```go
  // matchesNodeSelectorTerms checks if a node matches any of the node selector terms.
  // Terms are ORed - matching any term is sufficient.
  // Within a term, expressions are ANDed - all must match.
  func matchesNodeSelectorTerms(node *corev1.Node, terms []corev1.NodeSelectorTerm) bool
  ```
- [x] Add `matchesNodeAffinity` function:
  ```go
  // matchesNodeAffinity checks if a pod's node affinity requirements are satisfied by a node.
  // Only checks RequiredDuringSchedulingIgnoredDuringExecution (hard constraint).
  // Preferred constraints are ignored for scale-down decisions.
  func matchesNodeAffinity(pod *corev1.Pod, node *corev1.Node) bool
  ```
- [x] Run tests: `go test ./pkg/scaler -run TestMatchesNodeAffinity -v`

### 3. Refactor Phase
- [x] Ensure consistent error handling
- [x] Add inline comments explaining OR/AND semantics
- [x] Run all safety tests: `go test ./pkg/scaler/... -v`

## Completion Criteria
- [x] `matchesNodeAffinity` returns true when no affinity is specified
- [x] `matchesNodeAffinity` only checks RequiredDuringScheduling (hard constraint)
- [x] `matchesNodeSelectorTerms` correctly implements OR logic for terms
- [x] `matchNodeSelectorRequirement` supports In, NotIn, Exists, DoesNotExist
- [x] New unit tests pass
- [x] Existing tests still pass
- [x] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 374-395:
```go
func matchesNodeAffinity(pod *corev1.Pod, node *corev1.Node) bool {
    if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
        return true // No affinity requirements
    }

    nodeAffinity := pod.Spec.Affinity.NodeAffinity

    // Check required (hard) constraints
    if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
        if !matchesNodeSelectorTerms(node, nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) {
            return false
        }
    }

    // Preferred (soft) constraints are NOT checked for scale-down decisions
    return true
}
```

## Test Examples

```go
func TestMatchesNodeAffinity(t *testing.T) {
    t.Run("Pod with no affinity matches any node", func(t *testing.T) {
        pod := &corev1.Pod{
            Spec: corev1.PodSpec{},
        }
        node := &corev1.Node{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"zone": "us-east-1a"},
            },
        }

        result := matchesNodeAffinity(pod, node)
        assert.True(t, result)
    })

    t.Run("Pod with required affinity matches node with correct label", func(t *testing.T) {
        pod := &corev1.Pod{
            Spec: corev1.PodSpec{
                Affinity: &corev1.Affinity{
                    NodeAffinity: &corev1.NodeAffinity{
                        RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
                            NodeSelectorTerms: []corev1.NodeSelectorTerm{
                                {
                                    MatchExpressions: []corev1.NodeSelectorRequirement{
                                        {
                                            Key:      "zone",
                                            Operator: corev1.NodeSelectorOpIn,
                                            Values:   []string{"us-east-1a", "us-east-1b"},
                                        },
                                    },
                                },
                            },
                        },
                    },
                },
            },
        }
        node := &corev1.Node{
            ObjectMeta: metav1.ObjectMeta{
                Labels: map[string]string{"zone": "us-east-1a"},
            },
        }

        result := matchesNodeAffinity(pod, node)
        assert.True(t, result)
    })
}
```

## Notes
- Impact scope: `pkg/scaler/safety.go` only
- Constraints: Must handle nil pointers gracefully
- Existing `HasNodeAffinity` helper checks if pod HAS affinity; this checks if affinity MATCHES
