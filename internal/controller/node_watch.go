package controller

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

func (r *ImageWarmupPolicyReconciler) mapNodeToPolicies(
	ctx context.Context,
	_ client.Object,
) []reconcile.Request {
	var policies cachev1alpha1.ImageWarmupPolicyList
	if err := r.List(ctx, &policies); err != nil {
		logf.FromContext(ctx).Error(
			err,
			"Could not list ImageWarmupPolicies for Node event",
		)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(policies.Items))
	for i := range policies.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: policies.Items[i].Name,
			},
		})
	}

	return requests
}

func nodeWarmupPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return true
		},
		UpdateFunc: func(update event.UpdateEvent) bool {
			oldNode, oldOK := update.ObjectOld.(*corev1.Node)
			newNode, newOK := update.ObjectNew.(*corev1.Node)
			return oldOK && newOK &&
				nodeChangeAffectsWarmup(oldNode, newNode)
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

func nodeChangeAffectsWarmup(oldNode, newNode *corev1.Node) bool {
	if oldNode == nil || newNode == nil {
		return true
	}

	if oldNode.UID != newNode.UID ||
		!maps.Equal(oldNode.Labels, newNode.Labels) ||
		oldNode.Spec.Unschedulable != newNode.Spec.Unschedulable {
		return true
	}

	oldEligible, oldReason := evaluateNodeEligibility(oldNode)
	newEligible, newReason := evaluateNodeEligibility(newNode)
	return oldEligible != newEligible || oldReason != newReason
}
