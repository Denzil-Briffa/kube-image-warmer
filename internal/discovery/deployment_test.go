package discovery

import (
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestImagesFromDeploymentScaledToZero(t *testing.T) {
	replicas := int32(0)

	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "example/app:v1"},
						{Name: "metrics", Image: "example/metrics:v1"},
					},
					InitContainers: []corev1.Container{
						{Name: "setup1", Image: "example/setup:v1"},
					},
				},
			},
		},
	}

	got := ImagesFromDeployment(deployment)

	want := []string{
		"example/app:v1",
		"example/metrics:v1",
		"example/setup:v1",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("ImagesFromDeployment() = %v, want %v", got, want)
	}
}
