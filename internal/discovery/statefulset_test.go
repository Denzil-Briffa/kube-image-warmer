package discovery

import (
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestImagesFromStatefulSetScaledToZero(t *testing.T) {
	replicas := int32(0)

	statefulSet := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "database", Image: "example/database:v2"},
					},
					InitContainers: []corev1.Container{
						{Name: "setup2", Image: "example/setup:v2"},
					},
				},
			},
		},
	}

	got := ImagesFromStatefulSet(statefulSet)

	want := []string{
		"example/database:v2",
		"example/setup:v2",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("ImagesFromStatefulSet() = %v, want %v", got, want)
	}
}
