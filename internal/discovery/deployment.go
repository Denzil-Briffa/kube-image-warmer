package discovery

import appsv1 "k8s.io/api/apps/v1"

func ImagesFromDeployment(deployment *appsv1.Deployment) []string {
	if deployment == nil {
		return nil
	}

	return ImagesFromPodSpec(&deployment.Spec.Template.Spec)
}

func DiscoveredImagesFromDeployment(
	deployment *appsv1.Deployment,
) []DiscoveredImage {
	if deployment == nil {
		return nil
	}

	return DiscoveredImagesFromPodSpec(
		&deployment.Spec.Template.Spec,
		deployment.Namespace,
		deploymentSourceKind,
		deployment.Name,
	)
}
