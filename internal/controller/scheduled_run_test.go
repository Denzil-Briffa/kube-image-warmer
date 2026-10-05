package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/scheduling"
)

const (
	testFiveMinuteSchedule   = "*/5 * * * *"
	testInitialActiveRunID   = "initial-active-run"
	testOperatorLocationName = "operator-local"
)

func TestEnsureScheduledWarmupRunStateInitializesNextRun(
	t *testing.T,
) {
	schedule := mustParseTestSchedule(t)
	operatorLocation := time.FixedZone(testOperatorLocationName, 2*60*60)
	now := time.Date(
		2026, time.September, 30, 15, 2, 30, 0,
		operatorLocation,
	)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID: testInitialActiveRunID,
		},
	}

	changed := ensureScheduledWarmupRunState(
		policy,
		schedule,
		now,
	)

	if !changed {
		t.Fatal("expected next scheduled run time to be initialized")
	}

	want := time.Date(
		2026, time.September, 30, 15, 5, 0, 0,
		operatorLocation,
	)
	assertNextScheduledRunTime(t, policy, want)

	if policy.Status.CurrentRunID != testInitialActiveRunID {
		t.Errorf(
			"expected active run to be preserved, got %q",
			policy.Status.CurrentRunID,
		)
	}
}

func TestEnsureScheduledWarmupRunStateStartsDueRun(
	t *testing.T,
) {
	schedule := mustParseTestSchedule(t)
	operatorLocation := time.FixedZone(testOperatorLocationName, 2*60*60)
	dueTime := time.Date(
		2026, time.September, 30, 15, 5, 0, 0,
		operatorLocation,
	)
	now := time.Date(
		2026, time.September, 30, 15, 26, 0, 0,
		operatorLocation,
	)
	metav1DueTime := metav1.NewTime(dueTime)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			NextScheduledRunTime:     &metav1DueTime,
			CurrentRunTargetCount:    9,
			CurrentRunActiveCount:    8,
			CurrentRunSucceededCount: 7,
			CurrentRunFailedCount:    6,
		},
	}

	changed := ensureScheduledWarmupRunState(
		policy,
		schedule,
		now,
	)

	if !changed {
		t.Fatal("expected overdue scheduled run to start")
	}

	wantRunID := scheduledWarmupRunID(dueTime)
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

	if policy.Status.CurrentRunTargetCount != 0 ||
		policy.Status.CurrentRunActiveCount != 0 ||
		policy.Status.CurrentRunSucceededCount != 0 ||
		policy.Status.CurrentRunFailedCount != 0 {
		t.Error("expected new scheduled-run counters to be zero")
	}

	wantNextRun := time.Date(
		2026, time.September, 30, 15, 30, 0, 0,
		operatorLocation,
	)
	assertNextScheduledRunTime(t, policy, wantNextRun)

	if ensureScheduledWarmupRunState(policy, schedule, now) {
		t.Error("expected repeated reconciliation to preserve run state")
	}
}

func TestEnsureScheduledWarmupRunStateDefersForActiveRun(
	t *testing.T,
) {
	schedule := mustParseTestSchedule(t)
	operatorLocation := time.FixedZone(testOperatorLocationName, 2*60*60)
	dueTime := time.Date(
		2026, time.September, 30, 15, 5, 0, 0,
		operatorLocation,
	)
	now := dueTime.Add(time.Minute)
	metav1DueTime := metav1.NewTime(dueTime)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID:         testInitialActiveRunID,
			CurrentRunTrigger:    cachev1alpha1.ImageWarmupRunTriggerInitial,
			NextScheduledRunTime: &metav1DueTime,
		},
	}

	if ensureScheduledWarmupRunState(policy, schedule, now) {
		t.Error("expected active run to defer scheduled run")
	}

	if policy.Status.CurrentRunID != testInitialActiveRunID {
		t.Errorf(
			"expected active run to remain unchanged, got %q",
			policy.Status.CurrentRunID,
		)
	}

	assertNextScheduledRunTime(t, policy, dueTime)
}

func TestScheduledRunRequeueAfter(t *testing.T) {
	now := time.Date(
		2026, time.September, 30, 15, 0, 0, 0,
		time.FixedZone(testOperatorLocationName, 2*60*60),
	)
	futureTime := metav1.NewTime(now.Add(2 * time.Minute))
	overdueTime := metav1.NewTime(now.Add(-time.Minute))

	tests := []struct {
		name   string
		policy *cachev1alpha1.ImageWarmupPolicy
		want   time.Duration
	}{
		{
			name:   "missing schedule state",
			policy: &cachev1alpha1.ImageWarmupPolicy{},
			want:   0,
		},
		{
			name: "future run",
			policy: &cachev1alpha1.ImageWarmupPolicy{
				Status: cachev1alpha1.ImageWarmupPolicyStatus{
					NextScheduledRunTime: &futureTime,
				},
			},
			want: 2 * time.Minute,
		},
		{
			name: "overdue with active run",
			policy: &cachev1alpha1.ImageWarmupPolicy{
				Status: cachev1alpha1.ImageWarmupPolicyStatus{
					CurrentRunID:         "active-run",
					NextScheduledRunTime: &overdueTime,
				},
			},
			want: activeRunScheduleRequeue,
		},
		{
			name: "overdue without active run",
			policy: &cachev1alpha1.ImageWarmupPolicy{
				Status: cachev1alpha1.ImageWarmupPolicyStatus{
					NextScheduledRunTime: &overdueTime,
				},
			},
			want: overdueScheduleRequeue,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := scheduledRunRequeueAfter(test.policy, now)
			if got != test.want {
				t.Errorf(
					"expected requeue after %v, got %v",
					test.want,
					got,
				)
			}
		})
	}
}

func TestCurrentTimeUsesInjectedClock(t *testing.T) {
	want := time.Date(
		2026, time.September, 30, 15, 0, 0, 0,
		time.FixedZone(testOperatorLocationName, 2*60*60),
	)
	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Now: func() time.Time {
			return want
		},
	}

	if got := reconciler.currentTime(); !got.Equal(want) {
		t.Errorf("expected current time %v, got %v", want, got)
	}
}

func mustParseTestSchedule(t *testing.T) scheduling.Schedule {
	t.Helper()

	schedule, err := scheduling.Parse(testFiveMinuteSchedule)
	if err != nil {
		t.Fatalf("parse test schedule: %v", err)
	}

	return schedule
}

func assertNextScheduledRunTime(
	t *testing.T,
	policy *cachev1alpha1.ImageWarmupPolicy,
	want time.Time,
) {
	t.Helper()

	if policy.Status.NextScheduledRunTime == nil {
		t.Fatal("expected next scheduled run time")
	}

	got := policy.Status.NextScheduledRunTime.Time
	if !got.Equal(want) {
		t.Errorf(
			"expected next scheduled run time %v, got %v",
			want,
			got,
		)
	}

	if got.Location() != want.Location() {
		t.Errorf(
			"expected next run location %q, got %q",
			want.Location(),
			got.Location(),
		)
	}
}
