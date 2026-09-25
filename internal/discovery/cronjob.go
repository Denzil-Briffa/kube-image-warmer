package discovery

import batchv1 "k8s.io/api/batch/v1"

func ImagesFromCronJob(cronJob *batchv1.CronJob) []string {
	if cronJob == nil {
		return nil
	}

	return ImagesFromPodSpec(&cronJob.Spec.JobTemplate.Spec.Template.Spec)
}

func DiscoveredImagesFromCronJob(
	cronJob *batchv1.CronJob,
) []DiscoveredImage {
	if cronJob == nil {
		return nil
	}

	return DiscoveredImagesFromPodSpec(
		&cronJob.Spec.JobTemplate.Spec.Template.Spec,
		cronJob.Namespace,
		cronJobSourceKind,
		cronJob.Name,
	)
}
