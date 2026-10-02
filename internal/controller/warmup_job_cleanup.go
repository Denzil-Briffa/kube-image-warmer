package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

const defaultTTLSecondsAfterFinished int32 = 3600

func effectiveTTLSecondsAfterFinished(
	policy *cachev1alpha1.ImageWarmupPolicy,
) int32 {
	if policy == nil || policy.Spec.TTLSecondsAfterFinished == nil {
		return defaultTTLSecondsAfterFinished
	}

	return *policy.Spec.TTLSecondsAfterFinished
}

func (r *ImageWarmupPolicyReconciler) persistWarmupStatusAndEnableCleanup(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
	statusChanged bool,
) (int, error) {
	if statusChanged {
		if err := r.Status().Update(ctx, policy); err != nil {
			return 0, fmt.Errorf(
				"update status for ImageWarmupPolicy %q: %w",
				policy.Name,
				err,
			)
		}
	}

	return r.enableFinishedWarmupJobCleanup(ctx, policy)
}

func (r *ImageWarmupPolicyReconciler) enableFinishedWarmupJobCleanup(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
) (int, error) {
	if policy == nil ||
		policy.Status.LastFinishedRun == nil ||
		policy.Status.LastFinishedRun.ID == "" {
		return 0, nil
	}

	var jobs batchv1.JobList

	if err := r.List(
		ctx,
		&jobs,
		client.MatchingLabels{
			managedByLabelKey: managedByLabelValue,
			policyUIDLabelKey: string(policy.UID),
			runIDLabelKey:     policy.Status.LastFinishedRun.ID,
		},
	); err != nil {
		return 0, fmt.Errorf(
			"list finished warming Jobs for policy %q: %w",
			policy.Name,
			err,
		)
	}

	ttlSecondsAfterFinished :=
		effectiveTTLSecondsAfterFinished(policy)
	patchedCount := 0

	for i := range jobs.Items {
		job := &jobs.Items[i]

		if !metav1.IsControlledBy(job, policy) ||
			!isWarmupJobTerminal(job) {
			continue
		}

		if job.Spec.TTLSecondsAfterFinished != nil &&
			*job.Spec.TTLSecondsAfterFinished ==
				ttlSecondsAfterFinished {
			continue
		}

		original := job.DeepCopy()
		jobTTLSecondsAfterFinished := ttlSecondsAfterFinished
		job.Spec.TTLSecondsAfterFinished =
			&jobTTLSecondsAfterFinished

		if err := r.Patch(
			ctx,
			job,
			client.MergeFrom(original),
		); err != nil {
			return patchedCount, fmt.Errorf(
				"enable cleanup for warming Job %q/%q: %w",
				job.Namespace,
				job.Name,
				err,
			)
		}

		patchedCount++
	}

	return patchedCount, nil
}
