package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultMaxConcurrentJobs = 4

type warmupJobInventory struct {
	activeCount      int
	existingJobs     map[types.NamespacedName]bool
	currentRunCounts warmupRunJobCounts
}

type warmupRunJobCounts struct {
	activeCount    int
	succeededCount int
	failedCount    int
}

func effectiveMaxConcurrentJobs(
	policy *cachev1alpha1.ImageWarmupPolicy,
) int {
	if policy == nil || policy.Spec.MaxConcurrentJobs == nil {
		return defaultMaxConcurrentJobs
	}

	return int(*policy.Spec.MaxConcurrentJobs)
}

func isWarmupJobTerminal(job *batchv1.Job) bool {
	if job == nil {
		return false
	}

	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}

		switch condition.Type {
		case batchv1.JobComplete, batchv1.JobFailed:
			return true
		}
	}

	return false
}

func countActiveWarmupJobs(
	policy *cachev1alpha1.ImageWarmupPolicy,
	jobs []batchv1.Job,
) int {
	if policy == nil {
		return 0
	}

	activeCount := 0

	for i := range jobs {
		job := &jobs[i]

		if !metav1.IsControlledBy(job, policy) {
			continue
		}

		if isWarmupJobTerminal(job) {
			continue
		}

		activeCount++
	}
	return activeCount
}

func (r *ImageWarmupPolicyReconciler) loadWarmupJobInventory(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
) (warmupJobInventory, error) {
	if policy == nil {
		return buildWarmupJobInventory(nil, nil), nil
	}

	var jobs batchv1.JobList

	err := r.List(
		ctx,
		&jobs,
		client.MatchingLabels{
			managedByLabelKey: managedByLabelValue,
			policyUIDLabelKey: string(policy.UID),
		},
	)
	if err != nil {
		return warmupJobInventory{}, fmt.Errorf(
			"list warming Jobs for policy %q: %w",
			policy.Name,
			err,
		)
	}

	return buildWarmupJobInventory(
		policy,
		jobs.Items,
	), nil
}

func availableWarmupJobCapacity(
	policy *cachev1alpha1.ImageWarmupPolicy,
	activeJobs int,
) int {
	available := effectiveMaxConcurrentJobs(policy) - activeJobs

	if available < 0 {
		return 0
	}

	return available
}

func existingWarmupJobKeys(
	policy *cachev1alpha1.ImageWarmupPolicy,
	jobs []batchv1.Job,
) map[types.NamespacedName]bool {
	existingJobs := make(
		map[types.NamespacedName]bool,
		len(jobs),
	)

	if policy == nil {
		return existingJobs
	}

	for i := range jobs {
		job := &jobs[i]

		if !metav1.IsControlledBy(job, policy) {
			continue
		}

		existingJobs[types.NamespacedName{
			Name:      job.Name,
			Namespace: job.Namespace,
		}] = true
	}

	return existingJobs
}

func buildWarmupJobInventory(
	policy *cachev1alpha1.ImageWarmupPolicy,
	jobs []batchv1.Job,
) warmupJobInventory {
	currentRunID := ""

	if policy != nil {
		currentRunID = policy.Status.CurrentRunID
	}

	return warmupJobInventory{
		activeCount: countActiveWarmupJobs(
			policy,
			jobs,
		),
		existingJobs: existingWarmupJobKeys(
			policy,
			jobs,
		),
		currentRunCounts: countWarmupRunJobs(
			policy,
			currentRunID,
			jobs,
		),
	}
}

func countWarmupRunJobs(
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	jobs []batchv1.Job,
) warmupRunJobCounts {
	var counts warmupRunJobCounts

	if policy == nil || runID == "" {
		return counts
	}

	for i := range jobs {
		job := &jobs[i]

		if !metav1.IsControlledBy(job, policy) {
			continue
		}

		if job.Labels[runIDLabelKey] != runID {
			continue
		}

		failed := false
		succeeded := false

		for _, condition := range job.Status.Conditions {
			if condition.Status != corev1.ConditionTrue {
				continue
			}

			switch condition.Type {
			case batchv1.JobFailed:
				failed = true
			case batchv1.JobComplete:
				succeeded = true
			}
		}

		switch {
		case failed:
			counts.failedCount++
		case succeeded:
			counts.succeededCount++
		default:
			counts.activeCount++
		}
	}

	return counts
}
