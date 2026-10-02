package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

func (r *ImageWarmupPolicyReconciler) reconcileWarmupRun(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
	selection nodeSelectionResult,
	images []discovery.DiscoveredImage,
) (warmupExecutionSummary, bool, error) {
	changed := ensureInitialWarmupRunState(policy)
	if ensureScheduledWarmupRunState(policy, r.Schedule, r.currentTime()) {
		changed = true
	}
	if syncNodeWarmupQueue(policy, selection.matchedNodes, selection.healthyNodes) {
		changed = true
	}
	if ensureNodeWarmupRunState(policy, selection.healthyNodes) {
		changed = true
	}

	runID := policy.Status.CurrentRunID
	trigger := policy.Status.CurrentRunTrigger
	nodeUID := policy.Status.CurrentRunNodeUID
	runNodes := nodesForCurrentWarmupRun(policy, selection.healthyNodes)

	summary, err := r.ensureCurrentWarmupRunJobs(ctx, policy, runNodes, images)
	if err != nil {
		return summary, changed, err
	}

	if trigger == cachev1alpha1.ImageWarmupRunTriggerNode &&
		nodeUIDMap(selection.selectedNodes)[nodeUID] == nil {
		summary.targetCount = summary.runCounts.activeCount +
			summary.runCounts.succeededCount + summary.runCounts.failedCount
		summary.skippedReasons[nodeReasonNoLongerSelected]++
	}

	if applyWarmupExecutionSummary(
		policy,
		summary,
		metav1.NewTime(r.currentTime()),
	) {
		changed = true
	}

	if runID != "" && policy.Status.CurrentRunID == "" &&
		recordCompletedRunNodeCoverage(
			policy, trigger, nodeUID, selection.healthyNodes,
		) {
		changed = true
	}

	return summary, changed, nil
}
