package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type warmupJobCreateErrorClient struct {
	client.Client
	createError error
}

func (c *warmupJobCreateErrorClient) Create(
	ctx context.Context,
	object client.Object,
	options ...client.CreateOption,
) error {
	return c.createError
}

func TestEnsureWarmupJobCreatesJobForHealthyNode(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	if err := cachev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add policy API to scheme: %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testHealthyWorkerName,
			UID:  types.UID("healthy-worker-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
			UID:  types.UID("policy-uid"),
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node).
			Build(),
	}

	result, err := reconciler.ensureWarmupJob(
		context.Background(),
		policy,
		"initial-run",
		node,
		image,
	)

	if err != nil {
		t.Fatalf("ensureWarmupJob returned an error: %v", err)
	}

	if !result.created {
		t.Fatal("expected a new Job to be created")
	}

	if result.job == nil {
		t.Fatal("expected the created Job to be returned")
	}

	if result.skipReason != "" {
		t.Errorf(
			"expected no skip reason, got %q",
			result.skipReason,
		)
	}

	storedJob := &batchv1.Job{}

	err = reconciler.Get(
		context.Background(),
		client.ObjectKey{
			Name:      result.job.Name,
			Namespace: result.job.Namespace,
		},
		storedJob,
	)
	if err != nil {
		t.Fatalf("get created Job: %v", err)
	}
	if len(storedJob.Spec.Template.Spec.InitContainers) != 1 ||
		storedJob.Spec.Template.Spec.InitContainers[0].Image != testWarmupHelperImage {
		t.Fatal("configured helper image was not propagated to the created Job")
	}
	secondResult, err := reconciler.ensureWarmupJob(
		context.Background(),
		policy,
		"initial-run",
		node,
		image,
	)
	if err != nil {
		t.Fatalf(
			"second ensureWarmupJob returned an error: %v",
			err,
		)
	}

	if secondResult.created {
		t.Error(
			"expected the existing Job to be reused",
		)
	}

	if secondResult.job == nil {
		t.Fatal("expected the existing Job to be returned")
	}

	if secondResult.job.Name != result.job.Name {
		t.Errorf(
			"expected existing Job %q, got %q",
			result.job.Name,
			secondResult.job.Name,
		)
	}

	jobList := &batchv1.JobList{}

	err = reconciler.List(
		context.Background(),
		jobList,
		client.InNamespace(image.Namespace),
	)
	if err != nil {
		t.Fatalf("list warming Jobs: %v", err)
	}

	if len(jobList.Items) != 1 {
		t.Errorf(
			"expected exactly one Job, got %d",
			len(jobList.Items),
		)
	}

}
func TestEnsureWarmupJobSkipsUnhealthyNode(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNotReadyWorkerName,
			UID:  types.UID("not-ready-worker-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
			UID:  types.UID("policy-uid"),
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node).
			Build(),
	}

	result, err := reconciler.ensureWarmupJob(
		context.Background(),
		policy,
		"initial-run",
		node,
		image,
	)
	if err != nil {
		t.Fatalf("ensureWarmupJob returned an error: %v", err)
	}

	if result.job != nil {
		t.Errorf(
			"expected no Job, got %q",
			result.job.Name,
		)
	}

	if result.created {
		t.Error("expected no Job to be created")
	}

	if result.skipReason != nodeReasonNotReady {
		t.Errorf(
			"expected skip reason %q, got %q",
			nodeReasonNotReady,
			result.skipReason,
		)
	}

	jobList := &batchv1.JobList{}

	err = reconciler.List(
		context.Background(),
		jobList,
		client.InNamespace(image.Namespace),
	)
	if err != nil {
		t.Fatalf("list warming Jobs: %v", err)
	}

	if len(jobList.Items) != 0 {
		t.Errorf(
			"expected no Jobs, got %d",
			len(jobList.Items),
		)
	}
}

func TestEnsureWarmupJobRejectsUnownedExistingJob(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testHealthyWorkerName,
			UID:  types.UID("healthy-worker-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
			UID:  types.UID("policy-uid"),
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	runID := "ownership-conflict-run"

	existingJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: warmupJobName(
				policy,
				runID,
				node,
				image,
			),
			Namespace: image.Namespace,
		},
	}

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node, existingJob).
			Build(),
	}

	result, err := reconciler.ensureWarmupJob(
		context.Background(),
		policy,
		runID,
		node,
		image,
	)

	if err == nil {
		t.Fatal(
			"expected an error for an unowned existing Job",
		)
	}

	if !strings.Contains(err.Error(), "is not controlled by policy") {
		t.Errorf(
			"expected ownership error, got %v",
			err,
		)
	}

	if result.job != nil {
		t.Errorf(
			"expected no accepted Job, got %q",
			result.job.Name,
		)
	}

	if result.created {
		t.Error("expected no Job to be created")
	}
}

func TestEnsureWarmupJobReturnsCreateError(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testHealthyWorkerName,
			UID:  types.UID("healthy-worker-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
			UID:  types.UID("policy-uid"),
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	createError := errors.New("Job creation unavailable")

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: &warmupJobCreateErrorClient{
			Client:      baseClient,
			createError: createError,
		},
	}

	result, err := reconciler.ensureWarmupJob(
		context.Background(),
		policy,
		"create-error-run",
		node,
		image,
	)

	if err == nil {
		t.Fatal("expected a Job creation error")
	}

	if !errors.Is(err, createError) {
		t.Errorf(
			"expected wrapped error %v, got %v",
			createError,
			err,
		)
	}

	if result.job != nil {
		t.Errorf(
			"expected no Job, got %q",
			result.job.Name,
		)
	}

	if result.created {
		t.Error("expected no Job to be created")
	}
}

func TestExecutePendingWarmupTargetsCreatesJobs(t *testing.T) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "execution-policy",
			UID:  types.UID("execution-policy-uid"),
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "execution-worker",
			UID:  types.UID("execution-worker-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	image := discovery.DiscoveredImage{
		Image:     "example/execution:v1",
		Namespace: defaultObjectName,
	}

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node).
			Build(),
		Scheme: scheme,
	}

	summary, err := reconciler.executePendingWarmupTargets(
		context.Background(),
		policy,
		"execution-run",
		[]warmupTarget{
			{
				node:  node,
				image: image,
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"executePendingWarmupTargets returned an error: %v",
			err,
		)
	}

	if summary.createdCount != 1 {
		t.Errorf(
			"expected 1 created Job, got %d",
			summary.createdCount,
		)
	}

	if summary.existingCount != 0 {
		t.Errorf(
			"expected 0 existing Jobs, got %d",
			summary.existingCount,
		)
	}

	if len(summary.skippedReasons) != 0 {
		t.Errorf(
			"expected no skipped targets, got %v",
			summary.skippedReasons,
		)
	}

	var jobs batchv1.JobList

	if err := reconciler.List(
		context.Background(),
		&jobs,
		client.InNamespace(defaultObjectName),
	); err != nil {
		t.Fatalf("list created warming Jobs: %v", err)
	}

	if len(jobs.Items) != 1 {
		t.Errorf(
			"expected 1 stored Job, got %d",
			len(jobs.Items),
		)
	}
}

func TestEnsureCurrentWarmupRunJobsRespectsFullCapacity(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core API to scheme: %v", err)
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	maxConcurrentJobs := int32(1)
	runID := "full-capacity-run"

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "full-capacity-policy",
			UID:  types.UID("full-capacity-policy-uid"),
		},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			MaxConcurrentJobs: &maxConcurrentJobs,
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID:      runID,
			CurrentRunTrigger: cachev1alpha1.ImageWarmupRunTriggerInitial,
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "full-capacity-worker",
			UID:  types.UID("full-capacity-worker-uid"),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.NodeDiskPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	image := discovery.DiscoveredImage{
		Image:     "example/full-capacity:v1",
		Namespace: defaultObjectName,
	}
	existingJob := buildWarmupJob(policy, node, image, runID, testWarmupHelperImage)

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node, existingJob).
			Build(),
		Scheme: scheme,
	}

	summary, err := reconciler.ensureCurrentWarmupRunJobs(
		context.Background(),
		policy,
		[]*corev1.Node{node},
		[]discovery.DiscoveredImage{image},
	)
	if err != nil {
		t.Fatalf(
			"ensureCurrentWarmupRunJobs returned an error: %v",
			err,
		)
	}

	if summary.createdCount != 0 {
		t.Errorf(
			"expected no new Jobs, got %d",
			summary.createdCount,
		)
	}
	if summary.targetCount != 1 {
		t.Errorf(
			"expected 1 desired target, got %d",
			summary.targetCount,
		)
	}

	if summary.runCounts.activeCount != 1 {
		t.Errorf(
			"expected 1 active current-run Job, got %d",
			summary.runCounts.activeCount,
		)
	}

	if summary.runCounts.succeededCount != 0 {
		t.Errorf(
			"expected 0 successful current-run Jobs, got %d",
			summary.runCounts.succeededCount,
		)
	}

	if summary.runCounts.failedCount != 0 {
		t.Errorf(
			"expected 0 failed current-run Jobs, got %d",
			summary.runCounts.failedCount,
		)
	}

	var jobs batchv1.JobList

	if err := reconciler.List(
		context.Background(),
		&jobs,
		client.InNamespace(defaultObjectName),
	); err != nil {
		t.Fatalf("list warming Jobs: %v", err)
	}

	if len(jobs.Items) != 1 {
		t.Errorf(
			"expected the existing Job only, got %d Jobs",
			len(jobs.Items),
		)
	}
}
