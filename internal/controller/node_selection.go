package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

type nodeSelectionResult struct {
	matchedNodeCount  int
	selectedNodeCount int
	healthyNodes      []*corev1.Node
	skippedReasons    map[string]int
}

func (r *ImageWarmupPolicyReconciler) selectHealthyNodes(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
) (nodeSelectionResult, error) {
	nodeSelector, err := metav1.LabelSelectorAsSelector(
		&policy.Spec.NodeSelector,
	)
	if err != nil {
		return nodeSelectionResult{}, fmt.Errorf(
			"convert node selector: %w",
			err,
		)
	}

	nodeSkipSelector, err := metav1.LabelSelectorAsSelector(
		&policy.Spec.NodeSkipSelector,
	)
	if err != nil {
		return nodeSelectionResult{}, fmt.Errorf(
			"convert node skip selector: %w",
			err,
		)
	}

	nodeSkipSelectorEnabled :=
		len(policy.Spec.NodeSkipSelector.MatchLabels) > 0 ||
			len(policy.Spec.NodeSkipSelector.MatchExpressions) > 0

	var nodes corev1.NodeList

	err = r.List(
		ctx,
		&nodes,
		client.MatchingLabelsSelector{
			Selector: nodeSelector,
		},
	)
	if err != nil {
		return nodeSelectionResult{}, fmt.Errorf(
			"list nodes matching selector: %w",
			err,
		)
	}

	healthyNodes, skippedReasons := filterHealthyNodes(
		nodes.Items,
		nodeSkipSelectorEnabled,
		nodeSkipSelector,
	)

	matchedNodeCount := len(nodes.Items)
	selectedNodeCount :=
		matchedNodeCount - skippedReasons[nodeReasonSkipSelector]

	return nodeSelectionResult{
		matchedNodeCount:  matchedNodeCount,
		selectedNodeCount: selectedNodeCount,
		healthyNodes:      healthyNodes,
		skippedReasons:    skippedReasons,
	}, nil
}
