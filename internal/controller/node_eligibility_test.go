package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
)

func eligibilityTestNode(
	unschedulable bool,
	conditions ...corev1.NodeCondition,
) *corev1.Node {
	return &corev1.Node{
		Spec: corev1.NodeSpec{
			Unschedulable: unschedulable,
		},
		Status: corev1.NodeStatus{
			Conditions: conditions,
		},
	}
}

func eligibilityTestCondition(
	conditionType corev1.NodeConditionType,
	status corev1.ConditionStatus,
) corev1.NodeCondition {
	return corev1.NodeCondition{
		Type:   conditionType,
		Status: status,
	}
}

func TestEvaluateNodeEligibility(t *testing.T) {
	testCases := []struct {
		name         string
		node         *corev1.Node
		wantEligible bool
		wantReason   string
	}{
		{
			name:         "missing node",
			node:         nil,
			wantEligible: false,
			wantReason:   nodeReasonMissing,
		},
		{
			name: "unschedulable node",
			node: eligibilityTestNode(
				true,
			),
			wantEligible: false,
			wantReason:   nodeReasonUnschedulable,
		},
		{
			name: "missing ready condition",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeDiskPressure,
					corev1.ConditionFalse,
				),
			),
			wantEligible: false,
			wantReason:   nodeReasonReadyConditionMissing,
		},
		{
			name: "node is not ready",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeReady,
					corev1.ConditionFalse,
				),
				eligibilityTestCondition(
					corev1.NodeDiskPressure,
					corev1.ConditionFalse,
				),
			),
			wantEligible: false,
			wantReason:   nodeReasonNotReady,
		},
		{
			name: "node readiness is unknown",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeReady,
					corev1.ConditionUnknown,
				),
				eligibilityTestCondition(
					corev1.NodeDiskPressure,
					corev1.ConditionFalse,
				),
			),
			wantEligible: false,
			wantReason:   nodeReasonReadyUnknown,
		},
		{
			name: "missing disk pressure condition",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeReady,
					corev1.ConditionTrue,
				),
			),
			wantEligible: false,
			wantReason:   nodeReasonDiskConditionMissing,
		},
		{
			name: "node has disk pressure",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeReady,
					corev1.ConditionTrue,
				),
				eligibilityTestCondition(
					corev1.NodeDiskPressure,
					corev1.ConditionTrue,
				),
			),
			wantEligible: false,
			wantReason:   nodeReasonDiskPressure,
		},
		{
			name: "disk pressure is unknown",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeReady,
					corev1.ConditionTrue,
				),
				eligibilityTestCondition(
					corev1.NodeDiskPressure,
					corev1.ConditionUnknown,
				),
			),
			wantEligible: false,
			wantReason:   nodeReasonDiskPressureUnknown,
		},
		{
			name: "eligible node",
			node: eligibilityTestNode(
				false,
				eligibilityTestCondition(
					corev1.NodeReady,
					corev1.ConditionTrue,
				),
				eligibilityTestCondition(
					corev1.NodeDiskPressure,
					corev1.ConditionFalse,
				),
			),
			wantEligible: true,
			wantReason:   nodeReasonEligible,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gotEligible, gotReason := evaluateNodeEligibility(
				testCase.node,
			)

			if gotEligible != testCase.wantEligible {
				t.Errorf(
					"expected eligible=%t, got %t",
					testCase.wantEligible,
					gotEligible,
				)
			}

			if gotReason != testCase.wantReason {
				t.Errorf(
					"expected reason %q, got %q",
					testCase.wantReason,
					gotReason,
				)
			}
		})
	}
}

func TestFilterHealthyNodes(t *testing.T) {
	const (
		skipLabelKey   = "warming-skip-test"
		skipLabelValue = "yes"
	)

	healthyNode := eligibilityTestNode(
		false,
		eligibilityTestCondition(
			corev1.NodeReady,
			corev1.ConditionTrue,
		),
		eligibilityTestCondition(
			corev1.NodeDiskPressure,
			corev1.ConditionFalse,
		),
	)
	healthyNode.Name = "healthy-node"

	skippedNode := eligibilityTestNode(
		false,
		eligibilityTestCondition(
			corev1.NodeReady,
			corev1.ConditionFalse,
		),
		eligibilityTestCondition(
			corev1.NodeDiskPressure,
			corev1.ConditionFalse,
		),
	)
	skippedNode.Name = "skipped-node"
	skippedNode.Labels = map[string]string{
		skipLabelKey: skipLabelValue,
	}

	notReadyNode := eligibilityTestNode(
		false,
		eligibilityTestCondition(
			corev1.NodeReady,
			corev1.ConditionFalse,
		),
		eligibilityTestCondition(
			corev1.NodeDiskPressure,
			corev1.ConditionFalse,
		),
	)
	notReadyNode.Name = "not-ready-node"

	nodes := []corev1.Node{
		*healthyNode,
		*skippedNode,
		*notReadyNode,
	}

	skipSelector := klabels.SelectorFromSet(
		klabels.Set{
			skipLabelKey: skipLabelValue,
		},
	)

	healthyNodes, skippedReasons := filterHealthyNodes(
		nodes,
		true,
		skipSelector,
	)

	if len(healthyNodes) != 1 {
		t.Fatalf(
			"expected one eligible node, got %d",
			len(healthyNodes),
		)
	}

	if healthyNodes[0].Name != healthyNode.Name {
		t.Errorf(
			"expected eligible node %q, got %q",
			healthyNode.Name,
			healthyNodes[0].Name,
		)
	}

	if skippedReasons[nodeReasonSkipSelector] != 1 {
		t.Errorf(
			"expected one node skipped by selector, got %d",
			skippedReasons[nodeReasonSkipSelector],
		)
	}

	if skippedReasons[nodeReasonNotReady] != 1 {
		t.Errorf(
			"expected one not-ready node, got %d",
			skippedReasons[nodeReasonNotReady],
		)
	}

	if len(skippedReasons) != 2 {
		t.Errorf(
			"expected two skip reasons, got %#v",
			skippedReasons,
		)
	}
}

func TestFilterHealthyNodesIgnoresDisabledSkipSelector(
	t *testing.T,
) {
	const (
		skipLabelKey   = "disabled-skip-test"
		skipLabelValue = "yes"
	)

	node := eligibilityTestNode(
		false,
		eligibilityTestCondition(
			corev1.NodeReady,
			corev1.ConditionTrue,
		),
		eligibilityTestCondition(
			corev1.NodeDiskPressure,
			corev1.ConditionFalse,
		),
	)
	node.Labels = map[string]string{
		skipLabelKey: skipLabelValue,
	}

	skipSelector := klabels.SelectorFromSet(
		klabels.Set{
			skipLabelKey: skipLabelValue,
		},
	)

	healthyNodes, skippedReasons := filterHealthyNodes(
		[]corev1.Node{*node},
		false,
		skipSelector,
	)

	if len(healthyNodes) != 1 {
		t.Fatalf(
			"expected the node to remain eligible, got %d nodes",
			len(healthyNodes),
		)
	}

	if len(skippedReasons) != 0 {
		t.Errorf(
			"expected no skip reasons, got %#v",
			skippedReasons,
		)
	}
}
