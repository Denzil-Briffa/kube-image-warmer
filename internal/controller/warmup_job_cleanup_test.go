package controller

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

const (
	testFinishedRunID    = "finished-run"
	testCleanupPolicyUID = "cleanup-policy-uid"
)

func TestEffectiveTTLSecondsAfterFinished(t *testing.T) {
	if got := effectiveTTLSecondsAfterFinished(nil); got != 3600 {
		t.Errorf("expected default TTL 3600, got %d", got)
	}

	configuredTTL := int32(7200)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			TTLSecondsAfterFinished: &configuredTTL,
		},
	}

	if got := effectiveTTLSecondsAfterFinished(policy); got != configuredTTL {
		t.Errorf(
			"expected configured TTL %d, got %d",
			configuredTTL,
			got,
		)
	}
}

func TestEnableFinishedWarmupJobCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to test scheme: %v", err)
	}

	configuredTTL := int32(7200)
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
			UID:  types.UID(testCleanupPolicyUID),
		},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			TTLSecondsAfterFinished: &configuredTTL,
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			LastFinishedRun: &cachev1alpha1.ImageWarmupRunSummary{
				ID: testFinishedRunID,
			},
		},
	}

	alreadyConfiguredTTL := configuredTTL
	completedJob := cleanupTestJob(
		"cleanup-completed",
		policy,
		testFinishedRunID,
		batchv1.JobComplete,
		nil,
	)
	failedJob := cleanupTestJob(
		"cleanup-failed",
		policy,
		testFinishedRunID,
		batchv1.JobFailed,
		nil,
	)
	activeJob := cleanupTestJob(
		"cleanup-active",
		policy,
		testFinishedRunID,
		"",
		nil,
	)
	otherRunJob := cleanupTestJob(
		"cleanup-other-run",
		policy,
		"older-run",
		batchv1.JobComplete,
		nil,
	)
	alreadyConfiguredJob := cleanupTestJob(
		"cleanup-already-configured",
		policy,
		testFinishedRunID,
		batchv1.JobComplete,
		&alreadyConfiguredTTL,
	)
	foreignJob := cleanupTestJob(
		"cleanup-foreign",
		policy,
		testFinishedRunID,
		batchv1.JobComplete,
		nil,
	)
	foreignJob.OwnerReferences = nil

	objects := []client.Object{
		completedJob,
		failedJob,
		activeJob,
		otherRunJob,
		alreadyConfiguredJob,
		foreignJob,
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client:            fakeClient,
		Scheme:            scheme,
	}

	patchedCount, err := reconciler.enableFinishedWarmupJobCleanup(
		context.Background(),
		policy,
	)
	if err != nil {
		t.Fatalf("enable cleanup for finished Jobs: %v", err)
	}

	if patchedCount != 2 {
		t.Errorf(
			"expected 2 Jobs to have cleanup enabled, got %d",
			patchedCount,
		)
	}

	assertCleanupJobTTL(
		t,
		fakeClient,
		completedJob.Name,
		&configuredTTL,
	)
	assertCleanupJobTTL(
		t,
		fakeClient,
		failedJob.Name,
		&configuredTTL,
	)
	assertCleanupJobTTL(t, fakeClient, activeJob.Name, nil)
	assertCleanupJobTTL(t, fakeClient, otherRunJob.Name, nil)
	assertCleanupJobTTL(
		t,
		fakeClient,
		alreadyConfiguredJob.Name,
		&configuredTTL,
	)
	assertCleanupJobTTL(t, fakeClient, foreignJob.Name, nil)
}

func cleanupTestJob(
	name string,
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	conditionType batchv1.JobConditionType,
	ttlSecondsAfterFinished *int32,
) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: defaultObjectName,
			Labels: map[string]string{
				managedByLabelKey: managedByLabelValue,
				policyUIDLabelKey: string(policy.UID),
				runIDLabelKey:     runID,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(
					policy,
					cachev1alpha1.GroupVersion.WithKind(
						"ImageWarmupPolicy",
					),
				),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ttlSecondsAfterFinished,
		},
	}

	if conditionType != "" {
		job.Status.Conditions = []batchv1.JobCondition{
			{
				Type:   conditionType,
				Status: corev1.ConditionTrue,
			},
		}
	}

	return job
}

func assertCleanupJobTTL(
	t *testing.T,
	kubeClient client.Client,
	name string,
	want *int32,
) {
	t.Helper()

	var job batchv1.Job
	err := kubeClient.Get(
		context.Background(),
		client.ObjectKey{
			Name:      name,
			Namespace: defaultObjectName,
		},
		&job,
	)
	if err != nil {
		t.Fatalf("get Job %q: %v", name, err)
	}

	got := job.Spec.TTLSecondsAfterFinished

	if want == nil {
		if got != nil {
			t.Errorf(
				"expected Job %q to have no TTL, got %d",
				name,
				*got,
			)
		}
		return
	}

	if got == nil {
		t.Errorf(
			"expected Job %q to have TTL %d, got nil",
			name,
			*want,
		)
		return
	}

	if *got != *want {
		t.Errorf(
			"expected Job %q to have TTL %d, got %d",
			name,
			*want,
			*got,
		)
	}
}
