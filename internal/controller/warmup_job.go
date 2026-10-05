package controller

import (
	"maps"
	"slices"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

const (
	managedByLabelKey     = "app.kubernetes.io/managed-by"
	managedByLabelValue   = "kube-image-warmer"
	policyUIDLabelKey     = "cache.denzil-briffa.github.io/policy-uid"
	targetNodeUIDLabelKey = "cache.denzil-briffa.github.io/target-node-uid"
	runIDLabelKey         = "cache.denzil-briffa.github.io/run-id"
	warmupHelperVolume    = "warmup-helper"
	warmupHelperMount     = "/image-warmer"
	warmupHelperPath      = warmupHelperMount + "/helper"
)

func buildWarmupJob(
	policy *cachev1alpha1.ImageWarmupPolicy,
	targetNode *corev1.Node,
	image discovery.DiscoveredImage,
	runID string,
	helperImage string,
) *batchv1.Job {
	automountServiceAccountToken := false
	nonRoot := true
	helperUser := int64(65532)
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	securityContext := &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
	backoffLimit := int32(0)
	if policy.Spec.BackoffLimit != nil {
		backoffLimit = *policy.Spec.BackoffLimit
	}

	activeDeadlineSeconds := int64(300)
	if policy.Spec.ActiveDeadlineSeconds != nil {
		activeDeadlineSeconds = *policy.Spec.ActiveDeadlineSeconds
	}

	labels := map[string]string{
		managedByLabelKey:     managedByLabelValue,
		policyUIDLabelKey:     string(policy.UID),
		targetNodeUIDLabelKey: string(targetNode.UID),
		runIDLabelKey:         runID,
	}

	jobName := warmupJobName(
		policy,
		runID,
		targetNode,
		image,
	)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: image.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(
					policy,
					cachev1alpha1.GroupVersion.WithKind("ImageWarmupPolicy"),
				),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &backoffLimit,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: maps.Clone(labels),
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: &automountServiceAccountToken,
					ImagePullSecrets:             slices.Clone(image.ImagePullSecrets),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &nonRoot, RunAsUser: &helperUser, RunAsGroup: &helperUser, FSGroup: &helperUser,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Volumes: []corev1.Volume{{Name: warmupHelperVolume,
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: resource.NewQuantity(16*1024*1024, resource.BinarySI),
						}},
					}},
					InitContainers: []corev1.Container{{
						Name: "install-warmup-helper", Image: helperImage, ImagePullPolicy: corev1.PullIfNotPresent,
						Command: []string{"/warmup-helper"}, Args: []string{"--install", warmupHelperPath},
						SecurityContext: securityContext.DeepCopy(),
						VolumeMounts:    []corev1.VolumeMount{{Name: warmupHelperVolume, MountPath: warmupHelperMount}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("16Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("32Mi"),
							},
						},
					}},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      "metadata.name",
												Operator: corev1.NodeSelectorOpIn,
												Values: []string{
													targetNode.Name,
												},
											},
										},
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "image-puller",
							Image:           image.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{warmupHelperPath},
							WorkingDir:      warmupHelperMount,
							Args:            []string{"--complete"},
							SecurityContext: securityContext.DeepCopy(),
							VolumeMounts: []corev1.VolumeMount{{
								Name: warmupHelperVolume, MountPath: warmupHelperMount, ReadOnly: true,
							}},
						},
					},
				},
			},
		},
	}
}
