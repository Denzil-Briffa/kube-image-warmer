package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type nodeRevalidationErrorClient struct {
	client.Client
	getError error
}

func (c *nodeRevalidationErrorClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	object client.Object,
	options ...client.GetOption,
) error {
	return c.getError
}
func TestRevalidateNodeForWarmupReturnsHealthyNode(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testHealthyWorkerName,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node).
			Build(),
	}

	gotNode, reason, err :=
		reconciler.revalidateNodeForWarmup(
			context.Background(),
			node.Name,
		)

	if err != nil {
		t.Fatalf(
			"revalidateNodeForWarmup returned an error: %v",
			err,
		)
	}

	if gotNode == nil {
		t.Fatal("expected the healthy Node to be returned")
	}

	if gotNode.Name != node.Name {
		t.Errorf(
			"expected Node %q, got %q",
			node.Name,
			gotNode.Name,
		)
	}

	if reason != nodeReasonEligible {
		t.Errorf(
			"expected reason %q, got %q",
			nodeReasonEligible,
			reason,
		)
	}
}

func TestRevalidateNodeForWarmupSkipsUnavailableNodes(
	t *testing.T,
) {
	tests := []struct {
		name       string
		nodeName   string
		node       *corev1.Node
		wantReason string
	}{
		{
			name:     "Node became NotReady",
			nodeName: testNotReadyWorkerName,
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNotReadyWorkerName,
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   corev1.NodeDiskPressure,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			wantReason: nodeReasonNotReady,
		},
		{
			name:       "Node disappeared",
			nodeName:   "missing-worker",
			node:       nil,
			wantReason: nodeReasonMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()

			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf(
					"add core API to scheme: %v",
					err,
				)
			}

			clientBuilder :=
				fake.NewClientBuilder().WithScheme(scheme)

			if test.node != nil {
				clientBuilder =
					clientBuilder.WithObjects(test.node)
			}

			reconciler := &ImageWarmupPolicyReconciler{
				WarmupHelperImage: testWarmupHelperImage,
				Client:            clientBuilder.Build(),
			}

			gotNode, reason, err :=
				reconciler.revalidateNodeForWarmup(
					context.Background(),
					test.nodeName,
				)

			if err != nil {
				t.Fatalf(
					"expected no controller error, got %v",
					err,
				)
			}

			if gotNode != nil {
				t.Errorf(
					"expected no eligible Node, got %q",
					gotNode.Name,
				)
			}

			if reason != test.wantReason {
				t.Errorf(
					"expected reason %q, got %q",
					test.wantReason,
					reason,
				)
			}
		})
	}
}

func TestRevalidateNodeForWarmupReturnsAPIError(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	getError := errors.New("Kubernetes API unavailable")

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: &nodeRevalidationErrorClient{
			Client:   baseClient,
			getError: getError,
		},
	}

	gotNode, reason, err :=
		reconciler.revalidateNodeForWarmup(
			context.Background(),
			"worker-1",
		)

	if err == nil {
		t.Fatal("expected an API error")
	}

	if !errors.Is(err, getError) {
		t.Errorf(
			"expected wrapped error %v, got %v",
			getError,
			err,
		)
	}

	if gotNode != nil {
		t.Errorf(
			"expected no Node, got %q",
			gotNode.Name,
		)
	}

	if reason != "" {
		t.Errorf(
			"expected an empty reason, got %q",
			reason,
		)
	}
}
