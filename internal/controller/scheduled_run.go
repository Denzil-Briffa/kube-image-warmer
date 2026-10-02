package controller

import (
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/scheduling"
)

const (
	scheduledWarmupRunPrefix = "scheduled-"
	overdueScheduleRequeue   = time.Second
	activeRunScheduleRequeue = 30 * time.Second
)

func (r *ImageWarmupPolicyReconciler) currentTime() time.Time {
	if r.Now != nil {
		return r.Now()
	}

	return time.Now()
}

func scheduledWarmupRunID(scheduledTime time.Time) string {
	return scheduledWarmupRunPrefix + strconv.FormatInt(
		scheduledTime.Unix(),
		10,
	)
}

func ensureScheduledWarmupRunState(
	policy *cachev1alpha1.ImageWarmupPolicy,
	schedule scheduling.Schedule,
	now time.Time,
) bool {
	if policy == nil || schedule == nil {
		return false
	}

	if policy.Status.NextScheduledRunTime == nil {
		nextRunTime := metav1.NewTime(schedule.Next(now))
		policy.Status.NextScheduledRunTime = &nextRunTime
		return true
	}

	nextRunTime := policy.Status.NextScheduledRunTime.Time
	if now.Before(nextRunTime) {
		return false
	}

	if policy.Status.CurrentRunID != "" {
		return false
	}

	policy.Status.CurrentRunID =
		scheduledWarmupRunID(nextRunTime)
	policy.Status.CurrentRunTrigger =
		cachev1alpha1.ImageWarmupRunTriggerScheduled
	policy.Status.CurrentRunTargetCount = 0
	policy.Status.CurrentRunActiveCount = 0
	policy.Status.CurrentRunSucceededCount = 0
	policy.Status.CurrentRunFailedCount = 0

	nextFutureRunTime := metav1.NewTime(schedule.Next(now))
	policy.Status.NextScheduledRunTime = &nextFutureRunTime

	return true
}

func scheduledRunRequeueAfter(
	policy *cachev1alpha1.ImageWarmupPolicy,
	now time.Time,
) time.Duration {
	if policy == nil || policy.Status.NextScheduledRunTime == nil {
		return 0
	}

	requeueAfter := policy.Status.NextScheduledRunTime.Sub(now)
	if requeueAfter > 0 {
		return requeueAfter
	}

	if policy.Status.CurrentRunID != "" {
		return activeRunScheduleRequeue
	}

	return overdueScheduleRequeue
}
