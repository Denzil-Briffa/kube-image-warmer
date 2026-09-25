package discovery

import appsv1 "k8s.io/api/apps/v1"

func ImagesFromStatefulSet(statefulSet *appsv1.StatefulSet) []string {
	if statefulSet == nil {
		return nil
	}

	return ImagesFromPodSpec(&statefulSet.Spec.Template.Spec)
}

func DiscoveredImagesFromStatefulSet(
	statefulSet *appsv1.StatefulSet,
) []DiscoveredImage {
	if statefulSet == nil {
		return nil
	}

	return DiscoveredImagesFromPodSpec(
		&statefulSet.Spec.Template.Spec,
		statefulSet.Namespace,
		statefulSetSourceKind,
		statefulSet.Name,
	)
}
