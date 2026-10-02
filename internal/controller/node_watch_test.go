package controller

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

func TestNodeWarmupPredicate(t *testing.T) {
	oldNode := nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID)
	tests := []struct {
		name   string
		change func(*corev1.Node)
		want   bool
	}{
		{
			name: "heartbeat only",
			change: func(node *corev1.Node) {
				node.ResourceVersion = "2"
				node.Status.Conditions[0].LastHeartbeatTime = metav1.Now()
			},
		},
		{
			name: "Ready changed",
			change: func(node *corev1.Node) {
				node.Status.Conditions[0].Status = corev1.ConditionFalse
			},
			want: true,
		},
		{
			name: "DiskPressure changed",
			change: func(node *corev1.Node) {
				node.Status.Conditions[1].Status = corev1.ConditionTrue
			},
			want: true,
		},
		{
			name: "labels changed",
			change: func(node *corev1.Node) {
				node.Labels = map[string]string{nodeRunLabelKey: "enabled"}
			},
			want: true,
		},
		{
			name:   "cordoned",
			change: func(node *corev1.Node) { node.Spec.Unschedulable = true },
			want:   true,
		},
		{
			name:   "replacement UID",
			change: func(node *corev1.Node) { node.UID = types.UID(nodeRunReplacementUID) },
			want:   true,
		},
	}
	filter := nodeWarmupPredicate()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newNode := oldNode.DeepCopy()
			tt.change(newNode)
			got := filter.Update(event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode})
			if got != tt.want {
				t.Fatalf("expected update accepted=%t, got %t", tt.want, got)
			}
		})
	}
	if !filter.Create(event.CreateEvent{Object: oldNode}) ||
		!filter.Delete(event.DeleteEvent{Object: oldNode}) ||
		filter.Generic(event.GenericEvent{Object: oldNode}) {
		t.Fatal("expected create/delete events accepted and generic events ignored")
	}
}

func TestMapNodeToPolicies(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := cachev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	policies := []*cachev1alpha1.ImageWarmupPolicy{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-watch-first"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-watch-second"}},
	}
	reconciler := &ImageWarmupPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithObjects(policies[0], policies[1]).Build(),
	}
	requests := reconciler.mapNodeToPolicies(context.Background(), nil)
	names := make([]string, 0, len(requests))
	for _, request := range requests {
		if request.Namespace != "" {
			t.Fatal("policy requests must be cluster scoped")
		}
		names = append(names, request.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{policies[0].Name, policies[1].Name}) {
		t.Fatalf("unexpected policy requests: %v", requests)
	}
}
