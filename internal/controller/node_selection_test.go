package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

func TestSelectHealthyNodes(t *testing.T) {
	const (
		poolLabelKey    = "warmup-pool-test"
		warmPoolValue   = "warm"
		coldPoolValue   = "cold"
		skipLabelKey    = "warmup-skip-test"
		skipLabelValue  = "yes"
		healthyNodeName = "included-healthy-node"
	)

	newNode := func(
		name string,
		readyStatus corev1.ConditionStatus,
		labels map[string]string,
	) *corev1.Node {
		node := eligibilityTestNode(
			false,
			eligibilityTestCondition(
				corev1.NodeReady,
				readyStatus,
			),
			eligibilityTestCondition(
				corev1.NodeDiskPressure,
				corev1.ConditionFalse,
			),
		)

		node.Name = name
		node.Labels = labels

		return node
	}

	healthyNode := newNode(
		healthyNodeName,
		corev1.ConditionTrue,
		map[string]string{
			poolLabelKey: warmPoolValue,
		},
	)

	excludedByIncludeSelector := newNode(
		"excluded-by-include-selector",
		corev1.ConditionTrue,
		map[string]string{
			poolLabelKey: coldPoolValue,
		},
	)

	excludedBySkipSelector := newNode(
		"excluded-by-skip-selector",
		corev1.ConditionTrue,
		map[string]string{
			poolLabelKey: warmPoolValue,
			skipLabelKey: skipLabelValue,
		},
	)

	notReadyNode := newNode(
		"included-but-not-ready",
		corev1.ConditionFalse,
		map[string]string{
			poolLabelKey: warmPoolValue,
		},
	)

	scheme := runtime.NewScheme()

	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf(
			"failed to add core Kubernetes types to scheme: %v",
			err,
		)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			healthyNode,
			excludedByIncludeSelector,
			excludedBySkipSelector,
			notReadyNode,
		).
		Build()

	reconciler := &ImageWarmupPolicyReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-selection-test",
		},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			NodeSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					poolLabelKey: warmPoolValue,
				},
			},
			NodeSkipSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					skipLabelKey: skipLabelValue,
				},
			},
		},
	}

	result, err := reconciler.selectHealthyNodes(
		context.Background(),
		policy,
	)
	if err != nil {
		t.Fatalf(
			"selectHealthyNodes returned an error: %v",
			err,
		)
	}

	if result.matchedNodeCount != 3 {
		t.Errorf(
			"expected three nodes to match NodeSelector, got %d",
			result.matchedNodeCount,
		)
	}

	if result.selectedNodeCount != 2 {
		t.Errorf(
			"expected two nodes after NodeSkipSelector, got %d",
			result.selectedNodeCount,
		)
	}

	if len(result.healthyNodes) != 1 {
		t.Fatalf(
			"expected one healthy node, got %d",
			len(result.healthyNodes),
		)
	}

	if result.healthyNodes[0].Name != healthyNodeName {
		t.Errorf(
			"expected healthy node %q, got %q",
			healthyNodeName,
			result.healthyNodes[0].Name,
		)
	}

	if result.skippedReasons[nodeReasonSkipSelector] != 1 {
		t.Errorf(
			"expected one node skipped by NodeSkipSelector, got %d",
			result.skippedReasons[nodeReasonSkipSelector],
		)
	}

	if result.skippedReasons[nodeReasonNotReady] != 1 {
		t.Errorf(
			"expected one not-ready node, got %d",
			result.skippedReasons[nodeReasonNotReady],
		)
	}

	if len(result.skippedReasons) != 2 {
		t.Errorf(
			"expected two skip reasons, got %#v",
			result.skippedReasons,
		)
	}
}
