package discovery

import (
	"slices"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestImagesFromCronJobWithoutActiveJobs(t *testing.T) {
	cronJob := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{
			Schedule: "0 */6 * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{Name: "backup", Image: "example/backup:v1"},
							},
							InitContainers: []corev1.Container{
								{Name: "prepare", Image: "example/prepare:v2"},
							},
						},
					},
				},
			},
		},
	}

	got := ImagesFromCronJob(cronJob)

	want := []string{
		"example/backup:v1",
		"example/prepare:v2",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("ImagesFromCronJob() = %v, want %v", got, want)
	}
}

func TestImagesFromCronJobNil(t *testing.T) {
	got := ImagesFromCronJob(nil)

	if got != nil {
		t.Fatalf("ImagesFromCronJob(nil) = %v, want nil", got)
	}
}
