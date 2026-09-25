package discovery

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestDiscoveredImagesFromPodSpec(t *testing.T) {
	const (
		namespace          = "production"
		sourceKind         = "Deployment"
		sourceName         = "api"
		serviceAccountName = "registry-reader"
		applicationImage   = "example/application:v1"
		setupImage         = "example/setup:v1"
	)

	pullSecrets := []corev1.LocalObjectReference{
		{Name: "registry-credentials"},
	}

	podSpec := &corev1.PodSpec{
		ServiceAccountName: serviceAccountName,
		ImagePullSecrets:   pullSecrets,
		Containers: []corev1.Container{
			{
				Name:  "application",
				Image: applicationImage,
			},
		},
		InitContainers: []corev1.Container{
			{
				Name:  "setup",
				Image: setupImage,
			},
		},
	}

	got := DiscoveredImagesFromPodSpec(
		podSpec,
		namespace,
		sourceKind,
		sourceName,
	)

	want := []DiscoveredImage{
		{
			Image:              applicationImage,
			Namespace:          namespace,
			SourceKind:         sourceKind,
			SourceName:         sourceName,
			ServiceAccountName: serviceAccountName,
			ImagePullSecrets:   pullSecrets,
		},
		{
			Image:              setupImage,
			Namespace:          namespace,
			SourceKind:         sourceKind,
			SourceName:         sourceName,
			ServiceAccountName: serviceAccountName,
			ImagePullSecrets:   pullSecrets,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"DiscoveredImagesFromPodSpec() = %#v, want %#v",
			got,
			want,
		)
	}
}

func TestDiscoveredImagesFromPodSpecUsesDefaultServiceAccount(
	t *testing.T,
) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "application",
				Image: "example/default-service-account:v1",
			},
		},
	}

	got := DiscoveredImagesFromPodSpec(
		podSpec,
		"default",
		"Deployment",
		"application",
	)

	if len(got) != 1 {
		t.Fatalf(
			"DiscoveredImagesFromPodSpec() returned %d items, want 1",
			len(got),
		)
	}

	if got[0].ServiceAccountName != "default" {
		t.Fatalf(
			"ServiceAccountName = %q, want %q",
			got[0].ServiceAccountName,
			"default",
		)
	}
}

func TestDiscoveredImagesFromPodSpecNil(t *testing.T) {
	got := DiscoveredImagesFromPodSpec(
		nil,
		"default",
		"Deployment",
		"application",
	)

	if got != nil {
		t.Fatalf(
			"DiscoveredImagesFromPodSpec(nil) = %#v, want nil",
			got,
		)
	}
}
