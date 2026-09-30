package controller

import (
	"testing"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

func TestWarmupJobNameIsDeterministic(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
			UID:  types.UID("policy-uid"),
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testTalosNodeName,
			UID:  types.UID("node-uid"),
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "registry-credentials"},
		},
	}

	firstName := warmupJobName(
		policy,
		"initial-run",
		targetNode,
		image,
	)

	secondName := warmupJobName(
		policy,
		"initial-run",
		targetNode,
		image,
	)

	if firstName == "" {
		t.Fatal("expected a non-empty Job name")
	}

	if firstName != secondName {
		t.Errorf(
			"expected deterministic names, got %q and %q",
			firstName,
			secondName,
		)
	}
}
func TestWarmupJobNameIgnoresPullSecretOrder(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("secret-order-policy"),
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("secret-order-node"),
		},
	}

	firstImage := discovery.DiscoveredImage{
		Image:     testRegistryApplicationImage,
		Namespace: testApplicationName,
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: testPrimaryPullSecretName},
			{Name: testFallbackPullSecretName},
		},
	}

	secondImage := discovery.DiscoveredImage{
		Image:     firstImage.Image,
		Namespace: firstImage.Namespace,
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: testFallbackPullSecretName},
			{Name: testPrimaryPullSecretName},
		},
	}

	firstName := warmupJobName(
		policy,
		"secret-order-run",
		targetNode,
		firstImage,
	)

	secondName := warmupJobName(
		policy,
		"secret-order-run",
		targetNode,
		secondImage,
	)

	if firstName != secondName {
		t.Errorf(
			"expected secret ordering to produce the same name, got %q and %q",
			firstName,
			secondName,
		)
	}
}
func TestWarmupJobNameChangesWhenIdentityChanges(t *testing.T) {
	type nameInput struct {
		policyUID  types.UID
		runID      string
		nodeUID    types.UID
		image      string
		namespace  string
		secretName string
	}

	baseInput := nameInput{
		policyUID:  types.UID("base-policy-uid"),
		runID:      "base-run",
		nodeUID:    types.UID("base-node-uid"),
		image:      testRegistryApplicationImage,
		namespace:  testApplicationName,
		secretName: "registry-credentials",
	}

	nameFor := func(input nameInput) string {
		policy := &cachev1alpha1.ImageWarmupPolicy{
			ObjectMeta: metav1.ObjectMeta{
				UID: input.policyUID,
			},
		}

		targetNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				UID: input.nodeUID,
			},
		}

		image := discovery.DiscoveredImage{
			Image:     input.image,
			Namespace: input.namespace,
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: input.secretName},
			},
		}

		return warmupJobName(
			policy,
			input.runID,
			targetNode,
			image,
		)
	}

	baseName := nameFor(baseInput)

	tests := []struct {
		name   string
		change func(*nameInput)
	}{
		{
			name: "policy UID",
			change: func(input *nameInput) {
				input.policyUID =
					types.UID("different-policy-uid")
			},
		},
		{
			name: "run ID",
			change: func(input *nameInput) {
				input.runID = "different-run"
			},
		},
		{
			name: "node UID",
			change: func(input *nameInput) {
				input.nodeUID =
					types.UID("different-node-uid")
			},
		},
		{
			name: "image",
			change: func(input *nameInput) {
				input.image =
					"registry.example.com/application:v2"
			},
		},
		{
			name: "namespace",
			change: func(input *nameInput) {
				input.namespace = "different-namespace"
			},
		},
		{
			name: "pull secret",
			change: func(input *nameInput) {
				input.secretName =
					"different-registry-credentials"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedInput := baseInput
			test.change(&changedInput)

			changedName := nameFor(changedInput)

			if changedName == baseName {
				t.Errorf(
					"expected changing %s to produce a different name",
					test.name,
				)
			}
		})
	}
}
