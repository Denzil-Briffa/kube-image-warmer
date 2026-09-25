package discovery

import (
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestImagesFromDaemonSetTemplate(t *testing.T) {
	daemonSet := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "agent", Image: "example/agent:v3"},
						{Name: "metrics", Image: "example/metrics:v3"},
					},
					InitContainers: []corev1.Container{
						{Name: "setup3", Image: "example/setup:v3"},
					},
				},
			},
		},
	}

	got := ImagesFromDaemonSet(daemonSet)

	want := []string{
		"example/agent:v3",
		"example/metrics:v3",
		"example/setup:v3",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("ImagesFromDaemonSet() = %v, want %v", got, want)
	}
}

func TestImagesFromDaemonSetNil(t *testing.T) {
	got := ImagesFromDaemonSet(nil)

	if got != nil {
		t.Fatalf("ImagesFromDaemonSet(nil) = %v, want nil", got)
	}
}
