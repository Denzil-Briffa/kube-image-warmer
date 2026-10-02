package controller

import (
	"context"
	"slices"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/scheduling"
)

const nodeRunSecondImage = "example/node-trigger:v2"

type nodeTriggerTestFixture struct {
	reconciler *ImageWarmupPolicyReconciler
	policyKey  types.NamespacedName
	baseline   *corev1.Node
	now        time.Time
}

func newNodeTriggerTestFixture(t *testing.T, twoImages bool) *nodeTriggerTestFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, batchv1.AddToScheme,
		cachev1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}

	cronSchedule, err := scheduling.Parse(nodeRunYearlySchedule)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &nodeTriggerTestFixture{
		policyKey: types.NamespacedName{Name: testWarmupPolicyName},
		baseline:  nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID),
		now:       time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC),
	}
	nextRun := metav1.NewTime(cronSchedule.Next(fixture.now))
	maxConcurrentJobs := int32(1)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: fixture.policyKey.Name, UID: types.UID(nodeRunPolicyUID),
		},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			MaxConcurrentJobs: &maxConcurrentJobs,
			NamespaceList:     []string{defaultObjectName},
			WorkloadSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{nodeRunLabelKey: nodeRunLabelValue},
			},
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			NodeCoverageInitialized: true,
			HandledNodeUIDs:         []string{nodeRunBaselineUID},
			NextScheduledRunTime:    &nextRun,
		},
	}
	source := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeRunLabelKey, Namespace: defaultObjectName,
			Labels: map[string]string{nodeRunLabelKey: nodeRunLabelValue},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "source-one", Image: nodeRunImage}},
				},
			},
		},
	}
	if twoImages {
		source.Spec.Template.Spec.Containers = append(
			source.Spec.Template.Spec.Containers,
			corev1.Container{Name: "source-two", Image: nodeRunSecondImage},
		)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(policy, &corev1.Node{}, &batchv1.Job{}).
		WithObjects(
			policy, fixture.baseline, source,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: defaultObjectName}},
			&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
				Name: defaultObjectName, Namespace: defaultObjectName,
			}},
		).Build()
	fixture.reconciler = &ImageWarmupPolicyReconciler{
		Client: fakeClient, Scheme: scheme, Schedule: cronSchedule,
		Now: func() time.Time { return fixture.now },
	}
	return fixture
}

func (f *nodeTriggerTestFixture) reconcile(t *testing.T) cachev1alpha1.ImageWarmupPolicy {
	t.Helper()
	if _, err := f.reconciler.Reconcile(
		context.Background(), ctrl.Request{NamespacedName: f.policyKey},
	); err != nil {
		t.Fatal(err)
	}
	var policy cachev1alpha1.ImageWarmupPolicy
	if err := f.reconciler.Get(context.Background(), f.policyKey, &policy); err != nil {
		t.Fatal(err)
	}
	return policy
}

func (f *nodeTriggerTestFixture) jobs(t *testing.T) []batchv1.Job {
	t.Helper()
	var jobs batchv1.JobList
	if err := f.reconciler.List(context.Background(), &jobs, client.MatchingLabels{
		managedByLabelKey: managedByLabelValue, policyUIDLabelKey: nodeRunPolicyUID,
	}); err != nil {
		t.Fatal(err)
	}
	return jobs.Items
}

func (f *nodeTriggerTestFixture) createNode(t *testing.T, node *corev1.Node) {
	t.Helper()
	if err := f.reconciler.Create(context.Background(), node); err != nil {
		t.Fatal(err)
	}
}

func (f *nodeTriggerTestFixture) setReady(
	t *testing.T, node *corev1.Node, status corev1.ConditionStatus,
) {
	t.Helper()
	if err := f.reconciler.Get(
		context.Background(), client.ObjectKeyFromObject(node), node,
	); err != nil {
		t.Fatal(err)
	}
	node.Status.Conditions[0].Status = status
	if err := f.reconciler.Status().Update(context.Background(), node); err != nil {
		t.Fatal(err)
	}
}

func (f *nodeTriggerTestFixture) completeJob(t *testing.T, job *batchv1.Job) {
	t.Helper()
	job.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
	}
	if err := f.reconciler.Status().Update(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileNodeWarmupBeforeCronSurvivesRestartAndCleanup(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, false)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	added.Status.Conditions[0].Status = corev1.ConditionFalse
	fixture.createNode(t, added)
	waiting := fixture.reconcile(t)
	if len(fixture.jobs(t)) != 0 || waiting.Status.CurrentRunID != "" {
		t.Fatal("a NotReady Node must not receive a warming Job")
	}
	fixture.setReady(t, added, corev1.ConditionTrue)
	running := fixture.reconcile(t)
	jobs := fixture.jobs(t)
	if len(jobs) != 1 || jobs[0].Labels[targetNodeUIDLabelKey] != nodeRunNewUID ||
		running.Status.CurrentRunTrigger != cachev1alpha1.ImageWarmupRunTriggerNode {
		t.Fatalf("expected one Job on the new Node, got %v and %+v", jobs, running.Status)
	}
	if !running.Status.NextScheduledRunTime.Equal(waiting.Status.NextScheduledRunTime) ||
		!fixture.now.Before(running.Status.NextScheduledRunTime.Time) {
		t.Fatal("Node warming must occur before and preserve the future cron tick")
	}

	// A new reconciler uses only persisted state and existing Jobs.
	previous := fixture.reconciler
	fixture.reconciler = &ImageWarmupPolicyReconciler{
		Client: previous.Client, Scheme: previous.Scheme, Schedule: previous.Schedule,
		Now: previous.Now,
	}
	fixture.reconcile(t)
	if len(fixture.jobs(t)) != 1 {
		t.Fatal("a restart must not duplicate the active Node Job")
	}
	fixture.completeJob(t, &jobs[0])
	finished := fixture.reconcile(t)
	if finished.Status.CurrentRunID != "" || finished.Status.LastFinishedRun == nil ||
		finished.Status.LastFinishedRun.NodeUID != nodeRunNewUID ||
		finished.Status.LastFinishedRun.SucceededCount != 1 {
		t.Fatalf("expected finished Node summary: %+v", finished.Status)
	}
	if !slices.Contains(finished.Status.HandledNodeUIDs, nodeRunNewUID) {
		t.Fatal("completed Node coverage must be persisted")
	}
	jobs = fixture.jobs(t)
	if jobs[0].Spec.TTLSecondsAfterFinished == nil {
		t.Fatal("finished Node Job should receive the deferred TTL")
	}
	if err := fixture.reconciler.Delete(context.Background(), &jobs[0]); err != nil {
		t.Fatal(err)
	}
	fixture.reconcile(t)
	if len(fixture.jobs(t)) != 0 {
		t.Fatal("TTL cleanup must not recreate completed Node work")
	}
}

func TestReconcileNodeWarmupReplacementAndOfflineArrival(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, false)
	if err := fixture.reconciler.Delete(context.Background(), fixture.baseline); err != nil {
		t.Fatal(err)
	}
	replacement := nodeRunTestNode(fixture.baseline.Name, nodeRunReplacementUID)
	fixture.createNode(t, replacement)
	// No watch event is required: reconciliation finds UIDs that appeared offline.
	running := fixture.reconcile(t)
	jobs := fixture.jobs(t)
	if len(jobs) != 1 || running.Status.CurrentRunNodeUID != nodeRunReplacementUID ||
		jobs[0].Labels[targetNodeUIDLabelKey] != nodeRunReplacementUID {
		t.Fatalf("expected replacement UID warming, got %v and %+v", jobs, running.Status)
	}
	if slices.Contains(running.Status.HandledNodeUIDs, nodeRunBaselineUID) {
		t.Fatal("deleted Node UID must be pruned")
	}
}

func TestReconcileNewNodeSharesActiveScheduledRunCapacity(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, false)
	initialized := fixture.reconcile(t)
	fixture.now = initialized.Status.NextScheduledRunTime.Time
	fullRun := fixture.reconcile(t)
	if fullRun.Status.CurrentRunTrigger != cachev1alpha1.ImageWarmupRunTriggerScheduled {
		t.Fatal("expected a scheduled run")
	}
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	fixture.createNode(t, added)
	fixture.setReady(t, fixture.baseline, corev1.ConditionFalse)
	atCapacity := fixture.reconcile(t)
	jobs := fixture.jobs(t)
	if len(jobs) != 1 || atCapacity.Status.CurrentRunID != fullRun.Status.CurrentRunID ||
		!slices.Contains(atCapacity.Status.PendingNodeUIDs, nodeRunNewUID) {
		t.Fatal("new Node must join the full run while respecting its occupied slot")
	}
	fixture.completeJob(t, &jobs[0])
	withNewJob := fixture.reconcile(t)
	jobs = fixture.jobs(t)
	if len(jobs) != 2 || withNewJob.Status.CurrentRunActiveCount != 1 ||
		withNewJob.Status.CurrentRunTargetCount != 2 {
		t.Fatalf("expected one new target using the freed slot: %+v", withNewJob.Status)
	}
	for i := range jobs {
		if jobs[i].Labels[runIDLabelKey] != fullRun.Status.CurrentRunID {
			t.Fatal("new Node should reuse the active scheduled run ID")
		}
		if !isWarmupJobTerminal(&jobs[i]) {
			if jobs[i].Labels[targetNodeUIDLabelKey] != nodeRunNewUID {
				t.Fatal("the active Job must target the new Node")
			}
			fixture.completeJob(t, &jobs[i])
		}
	}
	finished := fixture.reconcile(t)
	if finished.Status.CurrentRunID != "" ||
		len(finished.Status.PendingNodeUIDs) != 0 ||
		!slices.Contains(finished.Status.HandledNodeUIDs, nodeRunNewUID) {
		t.Fatalf("expected new Node covered by finished full run: %+v", finished.Status)
	}
	fixture.reconcile(t)
	if len(fixture.jobs(t)) != 2 {
		t.Fatal("full-run coverage must prevent a duplicate node-specific run")
	}
}

func TestReconcileNodeWarmupWaitsForHealthWithRemainingTargets(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, true)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	fixture.createNode(t, added)
	fixture.reconcile(t)
	jobs := fixture.jobs(t)
	if len(jobs) != 1 {
		t.Fatalf("expected one Job under concurrency=1, got %d", len(jobs))
	}
	fixture.completeJob(t, &jobs[0])
	fixture.setReady(t, added, corev1.ConditionFalse)
	paused := fixture.reconcile(t)
	if paused.Status.CurrentRunID == "" || paused.Status.CurrentRunTargetCount != 2 ||
		paused.Status.CurrentRunSucceededCount != 1 || len(fixture.jobs(t)) != 1 {
		t.Fatalf("unhealthy Node must retain unfinished work: %+v", paused.Status)
	}
	fixture.setReady(t, added, corev1.ConditionTrue)
	resumed := fixture.reconcile(t)
	if len(fixture.jobs(t)) != 2 || resumed.Status.CurrentRunActiveCount != 1 {
		t.Fatal("remaining target must resume when the Node is healthy")
	}
}

func TestReconcileNodeWarmupFinishesAfterNodeDeletion(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, true)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	fixture.createNode(t, added)
	fixture.reconcile(t)
	jobs := fixture.jobs(t)
	if len(jobs) != 1 {
		t.Fatalf("expected one active Job, got %d", len(jobs))
	}
	if err := fixture.reconciler.Delete(context.Background(), added); err != nil {
		t.Fatal(err)
	}
	fixture.completeJob(t, &jobs[0])
	finished := fixture.reconcile(t)
	if finished.Status.CurrentRunID != "" || finished.Status.LastFinishedRun == nil ||
		finished.Status.LastFinishedRun.TargetCount != 1 {
		t.Fatalf("deleted Node should finish with attempted Job counts: %+v", finished.Status)
	}
}

func TestEnsureWarmupJobRejectsReplacementBetweenPlanningAndCreation(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, false)
	planned := fixture.baseline.DeepCopy()
	if err := fixture.reconciler.Delete(context.Background(), fixture.baseline); err != nil {
		t.Fatal(err)
	}
	replacement := nodeRunTestNode(planned.Name, nodeRunReplacementUID)
	fixture.createNode(t, replacement)
	result, err := fixture.reconciler.ensureWarmupJob(
		context.Background(),
		&cachev1alpha1.ImageWarmupPolicy{ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName, UID: types.UID(nodeRunPolicyUID),
		}},
		nodeWarmupRunPrefix+nodeRunBaselineUID,
		planned,
		discovery.DiscoveredImage{Image: nodeRunImage, Namespace: defaultObjectName},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.created || result.skipReason != nodeReasonReplaced || len(fixture.jobs(t)) != 0 {
		t.Fatalf("stale UID must be skipped before creating a Job: %+v", result)
	}
}

func TestPlanPendingWarmupTargetsPrioritizesNewNode(t *testing.T) {
	existing := nodeRunTestNode(testHealthyWorkerName, nodeRunBaselineUID)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	capacity := int32(1)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID(nodeRunPolicyUID)},
		Spec:       cachev1alpha1.ImageWarmupPolicySpec{MaxConcurrentJobs: &capacity},
		Status:     cachev1alpha1.ImageWarmupPolicyStatus{PendingNodeUIDs: []string{nodeRunNewUID}},
	}
	targets := planPendingWarmupTargets(
		policy, scheduledWarmupRunPrefix+"priority",
		[]*corev1.Node{existing, added},
		[]discovery.DiscoveredImage{
			{Image: nodeRunImage, Namespace: defaultObjectName},
			{Image: nodeRunSecondImage, Namespace: defaultObjectName},
		},
		warmupJobInventory{existingJobs: make(map[types.NamespacedName]bool)},
	)
	if len(targets) != 1 || targets[0].node.UID != added.UID {
		t.Fatalf("the next available slot should prioritize the new Node: %v", targets)
	}
}

func TestReconcileNodeWarmupFinishesAfterSkipSelectorExcludesTarget(t *testing.T) {
	fixture := newNodeTriggerTestFixture(t, true)
	added := nodeRunTestNode(testNotReadyWorkerName, nodeRunNewUID)
	added.Labels = map[string]string{nodeRunLabelKey: nodeRunLabelValue}
	fixture.createNode(t, added)
	policy := fixture.reconcile(t)
	jobs := fixture.jobs(t)
	if len(jobs) != 1 {
		t.Fatalf("expected one active Job, got %d", len(jobs))
	}

	policy.Spec.NodeSkipSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{nodeRunLabelKey: nodeRunLabelValue},
	}
	if err := fixture.reconciler.Update(context.Background(), &policy); err != nil {
		t.Fatal(err)
	}
	fixture.completeJob(t, &jobs[0])
	finished := fixture.reconcile(t)
	if finished.Status.CurrentRunID != "" || finished.Status.LastFinishedRun == nil ||
		finished.Status.LastFinishedRun.TargetCount != 1 || len(fixture.jobs(t)) != 1 {
		t.Fatalf("excluded Node must finish its existing Jobs without creating another: %+v",
			finished.Status)
	}
}
