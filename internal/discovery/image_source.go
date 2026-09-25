package discovery

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
)

const (
	deploymentSourceKind  = "Deployment"
	statefulSetSourceKind = "StatefulSet"
	daemonSetSourceKind   = "DaemonSet"
	cronJobSourceKind     = "CronJob"
)

type DiscoveredImage struct {
	Image              string
	Namespace          string
	SourceKind         string
	SourceName         string
	ServiceAccountName string
	ImagePullSecrets   []corev1.LocalObjectReference
}

func DiscoveredImagesFromPodSpec(
	podSpec *corev1.PodSpec,
	namespace string,
	sourceKind string,
	sourceName string,
) []DiscoveredImage {
	if podSpec == nil {
		return nil
	}

	serviceAccountName := podSpec.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}

	images := ImagesFromPodSpec(podSpec)
	discoveredImages := make([]DiscoveredImage, 0, len(images))

	for _, image := range images {
		discoveredImages = append(
			discoveredImages,
			DiscoveredImage{
				Image:              image,
				Namespace:          namespace,
				SourceKind:         sourceKind,
				SourceName:         sourceName,
				ServiceAccountName: serviceAccountName,
				ImagePullSecrets:   slices.Clone(podSpec.ImagePullSecrets),
			},
		)
	}

	return discoveredImages
}
