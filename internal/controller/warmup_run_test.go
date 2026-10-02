package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

func TestEnsureInitialWarmupRunStateInitializesOnce(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "initial-run-policy",
			UID:  types.UID("initial-run-policy-uid"),
		},
	}

	changed := ensureInitialWarmupRunState(policy)

	if !changed {
		t.Fatal("expected initial run state to be created")
	}

	wantRunID := "initial-" + string(policy.UID)

	if policy.Status.CurrentRunID != wantRunID {
		t.Errorf(
			"expected run ID %q, got %q",
			wantRunID,
			policy.Status.CurrentRunID,
		)
	}

	if policy.Status.CurrentRunTrigger !=
		cachev1alpha1.ImageWarmupRunTriggerInitial {
		t.Errorf(
			"expected trigger %q, got %q",
			cachev1alpha1.ImageWarmupRunTriggerInitial,
			policy.Status.CurrentRunTrigger,
		)
	}

	changed = ensureInitialWarmupRunState(policy)

	if changed {
		t.Error("expected existing run state to remain unchanged")
	}

	if policy.Status.CurrentRunID != wantRunID {
		t.Errorf(
			"expected stable run ID %q, got %q",
			wantRunID,
			policy.Status.CurrentRunID,
		)
	}
}

func TestEnsureInitialWarmupRunStatePreservesExistingRun(
	t *testing.T,
) {
	wantRunID := "scheduled-existing-run"

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-run-policy",
			UID:  types.UID("existing-run-policy-uid"),
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID:      wantRunID,
			CurrentRunTrigger: cachev1alpha1.ImageWarmupRunTriggerScheduled,
		},
	}

	changed := ensureInitialWarmupRunState(policy)

	if changed {
		t.Error("expected existing run state to be preserved")
	}

	if policy.Status.CurrentRunID != wantRunID {
		t.Errorf(
			"expected run ID %q, got %q",
			wantRunID,
			policy.Status.CurrentRunID,
		)
	}

	if policy.Status.CurrentRunTrigger !=
		cachev1alpha1.ImageWarmupRunTriggerScheduled {
		t.Errorf(
			"expected trigger %q, got %q",
			cachev1alpha1.ImageWarmupRunTriggerScheduled,
			policy.Status.CurrentRunTrigger,
		)
	}
}

func TestEnsureInitialWarmupRunStateDoesNotRestartAfterScheduling(
	t *testing.T,
) {
	nextScheduledRunTime := metav1.Now()
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("scheduled-policy-uid"),
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			NextScheduledRunTime: &nextScheduledRunTime,
			LastFinishedRun: &cachev1alpha1.ImageWarmupRunSummary{
				ID:      "scheduled-finished-run",
				Trigger: cachev1alpha1.ImageWarmupRunTriggerScheduled,
			},
		},
	}

	if ensureInitialWarmupRunState(policy) {
		t.Error("expected persisted schedule state to prevent another initial run")
	}

	if policy.Status.CurrentRunID != "" {
		t.Errorf(
			"expected no current run, got %q",
			policy.Status.CurrentRunID,
		)
	}
}
func TestEnsureInitialWarmupRunStateHandlesNilPolicy(
	t *testing.T,
) {
	if ensureInitialWarmupRunState(nil) {
		t.Error("expected nil policy to remain unchanged")
	}
}

func TestUpdateCurrentWarmupRunProgress(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID: "progress-run",
		},
	}

	counts := warmupRunJobCounts{
		activeCount:    2,
		succeededCount: 3,
		failedCount:    1,
	}

	changed := updateCurrentWarmupRunProgress(
		policy,
		6,
		counts,
	)

	if !changed {
		t.Fatal("expected run progress to change")
	}

	if policy.Status.CurrentRunTargetCount != 6 {
		t.Errorf(
			"expected 6 targets, got %d",
			policy.Status.CurrentRunTargetCount,
		)
	}

	if policy.Status.CurrentRunActiveCount != 2 {
		t.Errorf(
			"expected 2 active Jobs, got %d",
			policy.Status.CurrentRunActiveCount,
		)
	}

	if policy.Status.CurrentRunSucceededCount != 3 {
		t.Errorf(
			"expected 3 successful Jobs, got %d",
			policy.Status.CurrentRunSucceededCount,
		)
	}

	if policy.Status.CurrentRunFailedCount != 1 {
		t.Errorf(
			"expected 1 failed Job, got %d",
			policy.Status.CurrentRunFailedCount,
		)
	}

	changed = updateCurrentWarmupRunProgress(
		policy,
		6,
		counts,
	)

	if changed {
		t.Error("expected unchanged progress to avoid another status write")
	}
}

func TestUpdateCurrentWarmupRunProgressRequiresActiveRun(
	t *testing.T,
) {
	policy := &cachev1alpha1.ImageWarmupPolicy{}

	changed := updateCurrentWarmupRunProgress(
		policy,
		1,
		warmupRunJobCounts{
			activeCount: 1,
		},
	)

	if changed {
		t.Error("expected policy without a current run to remain unchanged")
	}

	if updateCurrentWarmupRunProgress(nil, 1, warmupRunJobCounts{}) {
		t.Error("expected nil policy to remain unchanged")
	}
}

func TestFinishCurrentWarmupRun(t *testing.T) {
	policyUID := types.UID("finished-run-policy-uid")
	runID := initialWarmupRunPrefix + string(policyUID)
	completionTime := metav1.Now()

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "finished-run-policy",
			UID:  policyUID,
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID:             runID,
			CurrentRunTrigger:        cachev1alpha1.ImageWarmupRunTriggerInitial,
			CurrentRunTargetCount:    3,
			CurrentRunActiveCount:    0,
			CurrentRunSucceededCount: 2,
			CurrentRunFailedCount:    1,
		},
	}

	changed := finishCurrentWarmupRun(
		policy,
		completionTime,
	)

	if !changed {
		t.Fatal("expected the completed run to be finished")
	}

	if policy.Status.LastFinishedRun == nil {
		t.Fatal("expected a finished-run summary")
	}

	summary := policy.Status.LastFinishedRun

	if summary.ID != runID {
		t.Errorf(
			"expected finished run ID %q, got %q",
			runID,
			summary.ID,
		)
	}

	if summary.Trigger != cachev1alpha1.ImageWarmupRunTriggerInitial {
		t.Errorf(
			"expected trigger %q, got %q",
			cachev1alpha1.ImageWarmupRunTriggerInitial,
			summary.Trigger,
		)
	}

	if summary.TargetCount != 3 {
		t.Errorf(
			"expected 3 targets, got %d",
			summary.TargetCount,
		)
	}

	if summary.SucceededCount != 2 {
		t.Errorf(
			"expected 2 successful Jobs, got %d",
			summary.SucceededCount,
		)
	}

	if summary.FailedCount != 1 {
		t.Errorf(
			"expected 1 failed Job, got %d",
			summary.FailedCount,
		)
	}

	if !summary.CompletionTime.Time.Equal(completionTime.Time) {
		t.Errorf(
			"expected completion time %v, got %v",
			completionTime,
			summary.CompletionTime,
		)
	}

	if policy.Status.CurrentRunID != "" {
		t.Errorf(
			"expected current run ID to be cleared, got %q",
			policy.Status.CurrentRunID,
		)
	}

	if policy.Status.CurrentRunTrigger != "" {
		t.Errorf(
			"expected current run trigger to be cleared, got %q",
			policy.Status.CurrentRunTrigger,
		)
	}

	if policy.Status.CurrentRunTargetCount != 0 ||
		policy.Status.CurrentRunActiveCount != 0 ||
		policy.Status.CurrentRunSucceededCount != 0 ||
		policy.Status.CurrentRunFailedCount != 0 {
		t.Errorf(
			"expected current run counters to be cleared, got target=%d active=%d succeeded=%d failed=%d",
			policy.Status.CurrentRunTargetCount,
			policy.Status.CurrentRunActiveCount,
			policy.Status.CurrentRunSucceededCount,
			policy.Status.CurrentRunFailedCount,
		)
	}

	if ensureInitialWarmupRunState(policy) {
		t.Error("expected the finished initial run not to restart")
	}
}

func TestFinishCurrentWarmupRunKeepsIncompleteRun(
	t *testing.T,
) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID:             "incomplete-run",
			CurrentRunTrigger:        cachev1alpha1.ImageWarmupRunTriggerInitial,
			CurrentRunTargetCount:    2,
			CurrentRunActiveCount:    1,
			CurrentRunSucceededCount: 1,
		},
	}

	if finishCurrentWarmupRun(policy, metav1.Now()) {
		t.Error("expected active run to remain unfinished")
	}

	if policy.Status.CurrentRunID != "incomplete-run" {
		t.Errorf(
			"expected incomplete run to remain active, got %q",
			policy.Status.CurrentRunID,
		)
	}

	if policy.Status.LastFinishedRun != nil {
		t.Error("expected no finished-run summary")
	}
}
func TestApplyWarmupExecutionSummaryFinishesRun(t *testing.T) {
	completionTime := metav1.Now()
	runID := "execution-summary-run"

	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID:      runID,
			CurrentRunTrigger: cachev1alpha1.ImageWarmupRunTriggerInitial,
		},
	}

	summary := warmupExecutionSummary{
		targetCount: 2,
		runCounts: warmupRunJobCounts{
			succeededCount: 1,
			failedCount:    1,
		},
	}

	changed := applyWarmupExecutionSummary(
		policy,
		summary,
		completionTime,
	)

	if !changed {
		t.Fatal("expected execution summary to change status")
	}

	if policy.Status.CurrentRunID != "" {
		t.Errorf(
			"expected current run to be cleared, got %q",
			policy.Status.CurrentRunID,
		)
	}

	if policy.Status.LastFinishedRun == nil {
		t.Fatal("expected a finished-run summary")
	}

	finishedRun := policy.Status.LastFinishedRun

	if finishedRun.ID != runID {
		t.Errorf(
			"expected finished run ID %q, got %q",
			runID,
			finishedRun.ID,
		)
	}

	if finishedRun.TargetCount != 2 ||
		finishedRun.SucceededCount != 1 ||
		finishedRun.FailedCount != 1 {
		t.Errorf(
			"unexpected finished counts: target=%d succeeded=%d failed=%d",
			finishedRun.TargetCount,
			finishedRun.SucceededCount,
			finishedRun.FailedCount,
		)
	}

	if !finishedRun.CompletionTime.Time.Equal(completionTime.Time) {
		t.Errorf(
			"expected completion time %v, got %v",
			completionTime,
			finishedRun.CompletionTime,
		)
	}
}
