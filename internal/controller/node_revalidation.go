package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
)

func (r *ImageWarmupPolicyReconciler) revalidateNodeForWarmup(
	ctx context.Context,
	nodeName string,
) (*corev1.Node, string, error) {
	node := &corev1.Node{}

	err := r.Get(
		ctx,
		types.NamespacedName{
			Name: nodeName,
		},
		node,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nodeReasonMissing, nil
		}

		return nil, "", fmt.Errorf(
			"get Node %q before warming %w",
			nodeName,
			err,
		)
	}

	eligble, reason := evaluateNodeEligibility(node)
	if !eligble {
		return nil, reason, nil
	}

	return node, nodeReasonEligible, nil
}
