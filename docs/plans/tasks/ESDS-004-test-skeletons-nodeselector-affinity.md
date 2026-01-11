# Task: ESDS-004 - Test Skeletons for NodeSelector and Affinity

Metadata:
- Dependencies: None (can run parallel with ESDS-001)
- Provides: Test skeletons for AC2, AC3, AC4 (6 tests) - Red state verification
- Size: Small (1 file)

## Implementation Content
Complete the test skeleton implementations for nodeSelector and affinity matching in `safety_test.go`. The test skeletons already exist with `t.Skip()` - this task fills in the test data and assertions while keeping the skip to maintain Red state.

Existing skeletons to complete:
- `TestNodeSelectorInCanPodsBeRescheduled`: AC2 blocked/allowed (2 tests)
- `TestAntiAffinityVerification`: AC3 blocked/allowed (2 tests)
- `TestClearBlockingMessages`: AC4 message format (1 test)

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Read existing test skeletons (lines 1184-1325)
- [x] Complete "AC2: Scale-down blocked - no remaining node has required label" test:
  - Create pod with nodeSelector: `{disktype: ssd}`
  - Create node to remove with label `disktype=ssd`
  - Create remaining node WITHOUT `disktype=ssd` label
  - Add assertions for `MatchesNodeSelector(remainingNode, pod) == false`
  - Add assertions for `canPodsBeRescheduled` returning `(false, reason, nil)`
  - Keep `t.Skip()` at the end
- [x] Complete "AC2: Scale-down allowed - remaining node has required label" test:
  - Create pod with nodeSelector: `{disktype: ssd}`
  - Create remaining node WITH `disktype=ssd` label
  - Add assertions for `MatchesNodeSelector(remainingNode, pod) == true`
  - Keep `t.Skip()` at the end
- [x] Complete "AC3: Scale-down blocked - would violate pod anti-affinity" test:
  - Create pod with required podAntiAffinity: labelSelector `{app: web}`, topologyKey: hostname
  - Create remaining node already running pod with label `app=web`
  - Add assertions for anti-affinity violation detection
  - Keep `t.Skip()` at the end
- [x] Complete "AC3: Scale-down allowed - anti-affinity not violated" test:
  - Create pod with same anti-affinity
  - Create remaining node NOT running any pod with label `app=web`
  - Add assertions for no violation
  - Keep `t.Skip()` at the end
- [x] Complete "AC4: Blocking message includes pod name and constraint type" test:
  - Create pod `myapp/web-abc123` with nodeSelector `{zone: us-east-1}`
  - Create remaining nodes without the required label
  - Add assertions for reason containing pod name and constraint type
  - Keep `t.Skip()` at the end
- [x] Run tests and confirm they compile but skip: `go test ./pkg/scaler -run "TestNodeSelector|TestAntiAffinity|TestClearBlockingMessages" -v`

### 2. Green Phase
- [x] N/A - This is Phase 0, tests remain in Red state

### 3. Refactor Phase
- [x] N/A - This is Phase 0

## Completion Criteria
- [x] All 5 test cases have concrete test data
- [x] All 5 test cases have concrete assertions
- [x] All 5 test cases end with `t.Skip("Skeleton: Implementation required - ...")`
- [x] Tests compile successfully: `go build ./pkg/scaler/...`
- [x] Tests skip with expected messages
- [x] Operation verified (L3: Build Success Verification)

## Code Template - AC2 NodeSelector Test

```go
t.Run("AC2: Scale-down blocked - no remaining node has required label", func(t *testing.T) {
    ctx := context.Background()
    logger := zaptest.NewLogger(t)

    // Arrange:
    nodeToRemove := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "ssd-node-to-remove",
            Labels: map[string]string{
                "disktype": "ssd",
                "autoscaler.vpsie.com/nodegroup": "test-group",
            },
        },
        Spec: corev1.NodeSpec{Unschedulable: false},
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {Type: corev1.NodeReady, Status: corev1.ConditionTrue},
            },
            Allocatable: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("4"),
                corev1.ResourceMemory: resource.MustParse("8Gi"),
            },
        },
    }

    remainingNode := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "hdd-node",
            Labels: map[string]string{
                "disktype": "hdd", // DOES NOT match required "ssd"
                "autoscaler.vpsie.com/nodegroup": "test-group",
            },
        },
        Spec: corev1.NodeSpec{Unschedulable: false},
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {Type: corev1.NodeReady, Status: corev1.ConditionTrue},
            },
            Allocatable: corev1.ResourceList{
                corev1.ResourceCPU:    resource.MustParse("4"),
                corev1.ResourceMemory: resource.MustParse("8Gi"),
            },
        },
    }

    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "ssd-app",
            Namespace: "default",
        },
        Spec: corev1.PodSpec{
            NodeSelector: map[string]string{
                "disktype": "ssd",
            },
            Containers: []corev1.Container{
                {
                    Name: "app",
                    Resources: corev1.ResourceRequirements{
                        Requests: corev1.ResourceList{
                            corev1.ResourceCPU:    resource.MustParse("100m"),
                            corev1.ResourceMemory: resource.MustParse("128Mi"),
                        },
                    },
                },
            },
        },
    }

    fakeClient := fake.NewSimpleClientset(nodeToRemove, remainingNode)
    manager := &ScaleDownManager{
        client: fakeClient,
        logger: logger.Sugar(),
        config: DefaultConfig(),
    }

    // Act:
    // canSchedule, reason, err := manager.canPodsBeRescheduled(ctx, []*corev1.Pod{pod})

    // Assert:
    // require.NoError(t, err)
    // assert.False(t, canSchedule, "Scale-down should be blocked - no SSD node available")
    // assert.Contains(t, reason, "ssd-app", "Reason should contain pod name")

    t.Skip("Skeleton: Implementation required - findSchedulableNode function")
})
```

## Code Template - AC3 Anti-Affinity Test

```go
t.Run("AC3: Scale-down blocked - would violate pod anti-affinity", func(t *testing.T) {
    // Arrange:
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "web-replica-1",
            Namespace: "default",
            Labels: map[string]string{
                "app": "web",
            },
        },
        Spec: corev1.PodSpec{
            Affinity: &corev1.Affinity{
                PodAntiAffinity: &corev1.PodAntiAffinity{
                    RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
                        {
                            LabelSelector: &metav1.LabelSelector{
                                MatchLabels: map[string]string{
                                    "app": "web",
                                },
                            },
                            TopologyKey: "kubernetes.io/hostname",
                        },
                    },
                },
            },
        },
    }

    remainingNode := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "worker-1",
            Labels: map[string]string{
                "kubernetes.io/hostname": "worker-1",
            },
        },
    }

    existingPodOnRemainingNode := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "web-replica-2",
            Namespace: "default",
            Labels: map[string]string{
                "app": "web", // Matches anti-affinity selector
            },
        },
        Spec: corev1.PodSpec{
            NodeName: "worker-1",
        },
    }

    // Act:
    // violation := hasPodAntiAffinityViolation(pod, remainingNode, []*corev1.Pod{existingPodOnRemainingNode})

    // Assert:
    // assert.True(t, violation, "Should detect anti-affinity violation")

    _ = existingPodOnRemainingNode
    _ = remainingNode
    t.Skip("Skeleton: Implementation required - hasPodAntiAffinityViolation function")
})
```

## Notes
- Impact scope: Test file only - no production code changes
- Constraints: Maintain existing test structure and imports
- These tests are for Phase 2 functionality, but skeletons are created in Phase 0
