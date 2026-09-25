package discovery

import appsv1 "k8s.io/api/apps/v1"

func ImagesFromDaemonSet(daemonset *appsv1.DaemonSet) []string {
	if daemonset == nil {
		return nil
	}

	return ImagesFromPodSpec(&daemonset.Spec.Template.Spec)
}

func DiscoveredImagesFromDaemonSet(
	daemonset *appsv1.DaemonSet,
) []DiscoveredImage {
	if daemonset == nil {
		return nil
	}

	return DiscoveredImagesFromPodSpec(
		&daemonset.Spec.Template.Spec,
		daemonset.Namespace,
		daemonSetSourceKind,
		daemonset.Name,
	)
}
