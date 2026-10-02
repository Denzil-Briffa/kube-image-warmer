package controller

import (
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

const (
	nodeRunBaselineUID    = "baseline-node-uid"
	nodeRunNewUID         = "new-node-uid"
	nodeRunReplacementUID = "replacement-node-uid"
	nodeRunPolicyUID      = "node-trigger-policy-uid"
	nodeRunImage          = "example/node-trigger:v1"
	nodeRunLabelKey       = "node-trigger-source"
	nodeRunLabelValue     = "true"
	nodeRunYearlySchedule = "0 0 1 1 *"
)

func nodeRunTestNode(name, uid string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(uid)},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
			},
		},
	}
}

func TestSyncNodeWarmupQueueBaselinesAndPreservesHandledNodes(t *testing.T) {
	existing := nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID)
	policy := &cachev1alpha1.ImageWarmupPolicy{}
	nodes := []*corev1.Node{existing}

	if !syncNodeWarmupQueue(policy, nodes, nodes) {
		t.Fatal("expected first observation to initialize Node coverage")
	}
	if !policy.Status.NodeCoverageInitialized ||
		!slices.Equal(policy.Status.HandledNodeUIDs, []string{nodeRunBaselineUID}) ||
		len(policy.Status.PendingNodeUIDs) != 0 {
		t.Fatalf("unexpected baseline: %+v", policy.Status)
	}
	if syncNodeWarmupQueue(policy, nodes, nil) {
		t.Fatal("a temporary health change must preserve a handled Node UID")
	}
}

func TestSyncNodeWarmupQueueDetectsOfflineArrivalAndReplacement(t *testing.T) {
	existing := nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			NodeCoverageInitialized: true,
			HandledNodeUIDs:         []string{nodeRunBaselineUID},
		},
	}
	matched := []*corev1.Node{existing, added}

	// A registered Node is not queued until it becomes eligible.
	if syncNodeWarmupQueue(policy, matched, []*corev1.Node{existing}) {
		t.Fatal("expected no work for an ineligible Node")
	}
	if !syncNodeWarmupQueue(policy, matched, matched) ||
		!slices.Equal(policy.Status.PendingNodeUIDs, []string{nodeRunNewUID}) {
		t.Fatalf("expected unseen healthy UID to be queued: %+v", policy.Status)
	}
	if syncNodeWarmupQueue(policy, matched, matched) {
		t.Fatal("repeated observations must not duplicate pending work")
	}

	// The same name with a different UID represents a replacement Node.
	replacement := nodeRunTestNode(existing.Name, nodeRunReplacementUID)
	if !syncNodeWarmupQueue(
		policy, []*corev1.Node{replacement}, []*corev1.Node{replacement},
	) {
		t.Fatal("expected replacement Node to change the queue")
	}
	if len(policy.Status.HandledNodeUIDs) != 0 ||
		!slices.Equal(policy.Status.PendingNodeUIDs, []string{nodeRunReplacementUID}) {
		t.Fatalf("expected old UIDs pruned and replacement queued: %+v", policy.Status)
	}
}

func TestEnsureNodeWarmupRunStateTargetsOneHealthyPendingUID(t *testing.T) {
	existing := nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			NodeCoverageInitialized: true,
			PendingNodeUIDs:         []string{nodeRunNewUID},
		},
	}
	if ensureNodeWarmupRunState(policy, nil) {
		t.Fatal("an unhealthy pending Node must wait")
	}
	if !ensureNodeWarmupRunState(policy, []*corev1.Node{existing, added}) {
		t.Fatal("expected a node-triggered run")
	}
	if policy.Status.CurrentRunID != nodeWarmupRunPrefix+nodeRunNewUID ||
		policy.Status.CurrentRunTrigger != cachev1alpha1.ImageWarmupRunTriggerNode ||
		policy.Status.CurrentRunNodeUID != nodeRunNewUID ||
		len(policy.Status.PendingNodeUIDs) != 0 {
		t.Fatalf("unexpected node run: %+v", policy.Status)
	}
	if ensureNodeWarmupRunState(policy, []*corev1.Node{added}) {
		t.Fatal("an active run must retain its identity")
	}
	targets := nodesForCurrentWarmupRun(policy, []*corev1.Node{existing, added})
	if len(targets) != 1 || targets[0].UID != added.UID {
		t.Fatalf("expected only the new Node, got %v", targets)
	}
	if len(nodesForCurrentWarmupRun(policy, []*corev1.Node{existing})) != 0 {
		t.Fatal("a missing target must not redirect work to another Node")
	}
}

func TestRecordCompletedRunNodeCoverage(t *testing.T) {
	for _, trigger := range []cachev1alpha1.ImageWarmupRunTrigger{
		cachev1alpha1.ImageWarmupRunTriggerInitial,
		cachev1alpha1.ImageWarmupRunTriggerScheduled,
		cachev1alpha1.ImageWarmupRunTriggerNode,
	} {
		t.Run(string(trigger), func(t *testing.T) {
			existing := nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID)
			added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
			policy := &cachev1alpha1.ImageWarmupPolicy{
				Status: cachev1alpha1.ImageWarmupPolicyStatus{
					HandledNodeUIDs: []string{nodeRunBaselineUID},
					PendingNodeUIDs: []string{nodeRunNewUID},
				},
			}
			if !recordCompletedRunNodeCoverage(
				policy, trigger, nodeRunNewUID, []*corev1.Node{existing, added},
			) {
				t.Fatal("expected completed coverage to be recorded")
			}
			if !slices.Equal(policy.Status.HandledNodeUIDs,
				[]string{nodeRunBaselineUID, nodeRunNewUID}) ||
				len(policy.Status.PendingNodeUIDs) != 0 {
				t.Fatalf("unexpected completed coverage: %+v", policy.Status)
			}
			if recordCompletedRunNodeCoverage(
				policy, trigger, nodeRunNewUID, []*corev1.Node{existing, added},
			) {
				t.Fatal("recording the same completion must be idempotent")
			}
		})
	}
}

func TestNodeRunRequeueAfter(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{}
	if nodeRunRequeueAfter(policy) != 0 {
		t.Fatal("idle policy should not request a node retry")
	}
	policy.Status.PendingNodeUIDs = []string{nodeRunNewUID}
	if nodeRunRequeueAfter(policy) != pendingNodeRequeue {
		t.Fatal("pending work should request a bounded retry")
	}
	policy.Status.PendingNodeUIDs = nil
	policy.Status.CurrentRunTrigger = cachev1alpha1.ImageWarmupRunTriggerNode
	if nodeRunRequeueAfter(policy) != pendingNodeRequeue {
		t.Fatal("active Node work should retry while waiting for health or capacity")
	}
}

func TestMinimumPositiveDuration(t *testing.T) {
	tests := []struct {
		name                string
		first, second, want time.Duration
	}{
		{name: "neither", want: 0},
		{name: "schedule only", first: time.Hour, want: time.Hour},
		{name: "node only", second: pendingNodeRequeue, want: pendingNodeRequeue},
		{name: "node sooner", first: time.Hour, second: pendingNodeRequeue, want: pendingNodeRequeue},
		{name: "schedule sooner", first: time.Second, second: pendingNodeRequeue, want: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := minimumPositiveDuration(tt.first, tt.second); got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}
