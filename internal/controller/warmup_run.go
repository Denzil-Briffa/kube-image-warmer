package controller

import (
	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const initialWarmupRunPrefix = "initial-"

func ensureInitialWarmupRunState(
	policy *cachev1alpha1.ImageWarmupPolicy,
) bool {
	if policy == nil {
		return false
	}

	if policy.Status.CurrentRunID != "" {
		return false
	}

	if policy.Status.NextScheduledRunTime != nil {
		return false
	}

	if policy.UID == "" {
		return false
	}

	initialRunID := initialWarmupRunPrefix + string(policy.UID)

	if policy.Status.LastFinishedRun != nil && policy.Status.LastFinishedRun.ID == initialRunID {
		return false
	}

	policy.Status.CurrentRunID = initialRunID
	policy.Status.CurrentRunTrigger =
		cachev1alpha1.ImageWarmupRunTriggerInitial

	return true
}

func updateCurrentWarmupRunProgress(
	policy *cachev1alpha1.ImageWarmupPolicy,
	targetCount int,
	counts warmupRunJobCounts,
) bool {
	if policy == nil || policy.Status.CurrentRunID == "" {
		return false
	}

	changed := false

	updateCount := func(field *int32, value int32) {
		if *field == value {
			return
		}

		*field = value
		changed = true
	}

	updateCount(
		&policy.Status.CurrentRunTargetCount,
		int32(targetCount),
	)

	updateCount(
		&policy.Status.CurrentRunActiveCount,
		int32(counts.activeCount),
	)

	updateCount(
		&policy.Status.CurrentRunSucceededCount,
		int32(counts.succeededCount),
	)

	updateCount(
		&policy.Status.CurrentRunFailedCount,
		int32(counts.failedCount),
	)

	return changed
}

func finishCurrentWarmupRun(
	policy *cachev1alpha1.ImageWarmupPolicy,
	completionTime metav1.Time,
) bool {
	if policy == nil || policy.Status.CurrentRunID == "" {
		return false
	}

	if policy.Status.CurrentRunActiveCount != 0 {
		return false
	}

	finishedCount := policy.Status.CurrentRunSucceededCount + policy.Status.CurrentRunFailedCount

	if finishedCount != policy.Status.CurrentRunTargetCount {
		return false
	}

	policy.Status.LastFinishedRun =
		&cachev1alpha1.ImageWarmupRunSummary{
			ID:             policy.Status.CurrentRunID,
			Trigger:        policy.Status.CurrentRunTrigger,
			TargetCount:    policy.Status.CurrentRunTargetCount,
			SucceededCount: policy.Status.CurrentRunSucceededCount,
			FailedCount:    policy.Status.CurrentRunFailedCount,
			CompletionTime: completionTime,
			NodeUID:        policy.Status.CurrentRunNodeUID,
		}

	policy.Status.CurrentRunID = ""
	policy.Status.CurrentRunTrigger = ""
	policy.Status.CurrentRunTargetCount = 0
	policy.Status.CurrentRunActiveCount = 0
	policy.Status.CurrentRunSucceededCount = 0
	policy.Status.CurrentRunFailedCount = 0
	policy.Status.CurrentRunNodeUID = ""

	return true
}

func applyWarmupExecutionSummary(
	policy *cachev1alpha1.ImageWarmupPolicy,
	summary warmupExecutionSummary,
	observationTime metav1.Time,
) bool {
	changed := updateCurrentWarmupRunProgress(
		policy,
		summary.targetCount,
		summary.runCounts,
	)

	if finishCurrentWarmupRun(
		policy,
		observationTime,
	) {
		return true
	}

	return changed
}
