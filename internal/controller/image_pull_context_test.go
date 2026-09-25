package controller

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

const (
	contextTestImage          = "private.example.com/application:v1"
	contextTestOtherImage     = "private.example.com/worker:v1"
	contextTestNamespace      = "context-test"
	contextTestOtherNamespace = "context-test-other"
	contextTestSecretA        = "registry-a"
	contextTestSecretB        = "registry-b"
)

func TestDeduplicateImagePullContexts(t *testing.T) {
	discoveredImages := []discovery.DiscoveredImage{
		{
			Image:              contextTestImage,
			Namespace:          contextTestNamespace,
			SourceKind:         "Deployment",
			SourceName:         "frontend",
			ServiceAccountName: "frontend-account",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: contextTestSecretA},
				{Name: contextTestSecretB},
			},
		},
		{
			Image:              contextTestImage,
			Namespace:          contextTestNamespace,
			SourceKind:         "StatefulSet",
			SourceName:         "database",
			ServiceAccountName: "database-account",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: contextTestSecretB},
				{Name: contextTestSecretA},
			},
		},
		{
			Image:              contextTestImage,
			Namespace:          contextTestOtherNamespace,
			ServiceAccountName: "other-account",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: contextTestSecretA},
				{Name: contextTestSecretB},
			},
		},
		{
			Image:              contextTestImage,
			Namespace:          contextTestNamespace,
			ServiceAccountName: "limited-account",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: contextTestSecretA},
			},
		},
		{
			Image:              contextTestOtherImage,
			Namespace:          contextTestNamespace,
			ServiceAccountName: "frontend-account",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: contextTestSecretA},
				{Name: contextTestSecretB},
			},
		},
	}

	got := deduplicateImagePullContexts(discoveredImages)

	want := []discovery.DiscoveredImage{
		discoveredImages[0],
		discoveredImages[2],
		discoveredImages[3],
		discoveredImages[4],
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"expected deduplicated contexts %#v, got %#v",
			want,
			got,
		)
	}
}

func TestDeduplicateImagePullContextsHandlesEmptyInput(t *testing.T) {
	if got := deduplicateImagePullContexts(nil); len(got) != 0 {
		t.Fatalf("expected no contexts for nil input, got %#v", got)
	}

	emptyInput := []discovery.DiscoveredImage{}

	if got := deduplicateImagePullContexts(emptyInput); len(got) != 0 {
		t.Fatalf("expected no contexts for empty input, got %#v", got)
	}
}
