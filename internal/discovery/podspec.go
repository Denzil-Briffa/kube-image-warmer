package discovery

import corev1 "k8s.io/api/core/v1"

// ImagesFromPodSpec returns regular and init container image.
func ImagesFromPodSpec(podSpec *corev1.PodSpec) []string {
	if podSpec == nil {
		return nil
	}

	var images []string

	for _, container := range podSpec.Containers {
		images = append(images, container.Image)
	}

	for _, container := range podSpec.InitContainers {
		images = append(images, container.Image)
	}

	return images
}
