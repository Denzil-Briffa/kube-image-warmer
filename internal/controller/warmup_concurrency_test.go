package controller

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEffectiveMaxConcurrentJobs(t *testing.T) {
	configuredLimit := int32(7)

	tests := []struct {
		name       string
		policy     *cachev1alpha1.ImageWarmupPolicy
		wantResult int
	}{
		{
			name:       "nil policy uses fallback",
			policy:     nil,
			wantResult: defaultMaxConcurrentJobs,
		},
		{
			name:       "omitted value uses fallback",
			policy:     &cachev1alpha1.ImageWarmupPolicy{},
			wantResult: defaultMaxConcurrentJobs,
		},
		{
			name: "configured value is returned",
			policy: &cachev1alpha1.ImageWarmupPolicy{
				Spec: cachev1alpha1.ImageWarmupPolicySpec{
					MaxConcurrentJobs: &configuredLimit,
				},
			},
			wantResult: int(configuredLimit),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := effectiveMaxConcurrentJobs(test.policy)

			if got != test.wantResult {
				t.Errorf(
					"expected concurrency %d, got %d",
					test.wantResult,
					got,
				)
			}
		})
	}
}

func TestIsWarmupJobTerminal(t *testing.T) {
	tests := []struct {
		name string
		job  *batchv1.Job
		want bool
	}{
		{
			name: "nil Job is not terminal",
			job:  nil,
			want: false,
		},
		{
			name: "Job without conditions is active",
			job:  &batchv1.Job{},
			want: false,
		},
		{
			name: "Complete true is terminal",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "Failed true is terminal",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "Complete false is still active",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isWarmupJobTerminal(test.job)

			if got != test.want {
				t.Errorf(
					"expected terminal=%t, got %t",
					test.want,
					got,
				)
			}
		})
	}
}

func TestCountActiveWarmupJobs(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "concurrency-policy",
			UID:  types.UID("concurrency-policy-uid"),
		},
	}

	ownedJob := func(
		name string,
		conditions []batchv1.JobCondition,
	) batchv1.Job {
		return batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(
						policy,
						cachev1alpha1.GroupVersion.WithKind(
							"ImageWarmupPolicy",
						),
					),
				},
			},
			Status: batchv1.JobStatus{
				Conditions: conditions,
			},
		}
	}

	jobs := []batchv1.Job{
		ownedJob("active-job", nil),
		ownedJob(
			"complete-false-job",
			[]batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionFalse,
				},
			},
		),
		ownedJob(
			"complete-job",
			[]batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		),
		ownedJob(
			"failed-job",
			[]batchv1.JobCondition{
				{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
				},
			},
		),
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: testForeignJobName,
			},
		},
	}

	got := countActiveWarmupJobs(policy, jobs)

	if got != 2 {
		t.Errorf(
			"expected 2 active owned Jobs, got %d",
			got,
		)
	}

	if gotForNilPolicy :=
		countActiveWarmupJobs(nil, jobs); gotForNilPolicy != 0 {
		t.Errorf(
			"expected 0 Jobs for a nil policy, got %d",
			gotForNilPolicy,
		)
	}
}

func TestActiveWarmupJobCountListsManagedPolicyJobs(
	t *testing.T,
) {
	scheme := runtime.NewScheme()

	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch API to scheme: %v", err)
	}

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "concurrency-policy",
			UID:  types.UID("concurrency-policy-uid"),
		},
	}

	managedLabels := func(policyUID types.UID) map[string]string {
		return map[string]string{
			managedByLabelKey: managedByLabelValue,
			policyUIDLabelKey: string(policyUID),
		}
	}

	ownerReferences := []metav1.OwnerReference{
		*metav1.NewControllerRef(
			policy,
			cachev1alpha1.GroupVersion.WithKind(
				"ImageWarmupPolicy",
			),
		),
	}

	firstActiveJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "active-team-a",
			Namespace:       testTeamANamespace,
			Labels:          managedLabels(policy.UID),
			OwnerReferences: ownerReferences,
		},
	}

	secondActiveJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "active-team-b",
			Namespace:       testTeamBNamespace,
			Labels:          managedLabels(policy.UID),
			OwnerReferences: ownerReferences,
		},
	}

	completedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "completed-job",
			Namespace:       testTeamANamespace,
			Labels:          managedLabels(policy.UID),
			OwnerReferences: ownerReferences,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	foreignJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testForeignJobName,
			Namespace: testTeamANamespace,
			Labels:    managedLabels(policy.UID),
		},
	}

	otherPolicyJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-policy-job",
			Namespace: testTeamANamespace,
			Labels: managedLabels(
				types.UID("other-policy-uid"),
			),
			OwnerReferences: ownerReferences,
		},
	}

	reconciler := &ImageWarmupPolicyReconciler{
		WarmupHelperImage: testWarmupHelperImage,
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				firstActiveJob,
				secondActiveJob,
				completedJob,
				foreignJob,
				otherPolicyJob,
			).
			Build(),
	}

	inventory, err := reconciler.loadWarmupJobInventory(
		context.Background(),
		policy,
	)
	if err != nil {
		t.Fatalf(
			"loadWarmupJobInventory returned an error: %v",
			err,
		)
	}

	if inventory.activeCount != 2 {
		t.Errorf(
			"expected 2 active managed Jobs, got %d",
			inventory.activeCount,
		)
	}

	if len(inventory.existingJobs) != 3 {
		t.Errorf(
			"expected 3 existing owned Jobs, got %d",
			len(inventory.existingJobs),
		)
	}

	completedKey := types.NamespacedName{
		Name:      completedJob.Name,
		Namespace: completedJob.Namespace,
	}

	if !inventory.existingJobs[completedKey] {
		t.Errorf(
			"expected completed Job %q/%q in the inventory",
			completedKey.Namespace,
			completedKey.Name,
		)
	}
}

func TestAvailableWarmupJobCapacity(t *testing.T) {
	configuredLimit := int32(3)

	configuredPolicy := &cachev1alpha1.ImageWarmupPolicy{
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			MaxConcurrentJobs: &configuredLimit,
		},
	}

	tests := []struct {
		name       string
		policy     *cachev1alpha1.ImageWarmupPolicy
		activeJobs int
		want       int
	}{
		{
			name:       "default limit with no active Jobs",
			policy:     nil,
			activeJobs: 0,
			want:       defaultMaxConcurrentJobs,
		},
		{
			name:       "default limit with active Jobs",
			policy:     nil,
			activeJobs: 2,
			want:       2,
		},
		{
			name:       "configured limit has available capacity",
			policy:     configuredPolicy,
			activeJobs: 1,
			want:       2,
		},
		{
			name:       "active Jobs equal limit",
			policy:     configuredPolicy,
			activeJobs: 3,
			want:       0,
		},
		{
			name:       "active Jobs exceed limit",
			policy:     configuredPolicy,
			activeJobs: 5,
			want:       0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := availableWarmupJobCapacity(
				test.policy,
				test.activeJobs,
			)

			if got != test.want {
				t.Errorf(
					"expected capacity %d, got %d",
					test.want,
					got,
				)
			}
		})
	}
}

func TestExistingWarmupJobKeys(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "inventory-policy",
			UID:  types.UID("inventory-policy-uid"),
		},
	}

	ownerReference := *metav1.NewControllerRef(
		policy,
		cachev1alpha1.GroupVersion.WithKind(
			"ImageWarmupPolicy",
		),
	)

	activeJob := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "active-job",
			Namespace:       testTeamANamespace,
			OwnerReferences: []metav1.OwnerReference{ownerReference},
		},
	}

	completedJob := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "completed-job",
			Namespace:       testTeamBNamespace,
			OwnerReferences: []metav1.OwnerReference{ownerReference},
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	foreignJob := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testForeignJobName,
			Namespace: testTeamANamespace,
		},
	}

	jobs := []batchv1.Job{
		activeJob,
		completedJob,
		foreignJob,
	}

	got := existingWarmupJobKeys(policy, jobs)

	wantKeys := []types.NamespacedName{
		{
			Name:      activeJob.Name,
			Namespace: activeJob.Namespace,
		},
		{
			Name:      completedJob.Name,
			Namespace: completedJob.Namespace,
		},
	}

	if len(got) != len(wantKeys) {
		t.Fatalf(
			"expected %d existing Jobs, got %d",
			len(wantKeys),
			len(got),
		)
	}

	for _, wantKey := range wantKeys {
		if !got[wantKey] {
			t.Errorf(
				"expected existing Job %q/%q",
				wantKey.Namespace,
				wantKey.Name,
			)
		}
	}

	foreignKey := types.NamespacedName{
		Name:      foreignJob.Name,
		Namespace: foreignJob.Namespace,
	}

	if got[foreignKey] {
		t.Error("expected foreign Job to be excluded")
	}

	if gotForNilPolicy :=
		existingWarmupJobKeys(nil, jobs); len(gotForNilPolicy) != 0 {
		t.Errorf(
			"expected no Jobs for nil policy, got %d",
			len(gotForNilPolicy),
		)
	}
}
func TestCountWarmupRunJobsClassifiesOwnedCurrentRun(
	t *testing.T,
) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run-count-policy",
			UID:  types.UID("run-count-policy-uid"),
		},
	}

	currentRunID := "current-count-run"

	ownedRunJob := func(
		name string,
		runID string,
		conditions []batchv1.JobCondition,
	) batchv1.Job {
		return batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					runIDLabelKey: runID,
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
			Status: batchv1.JobStatus{
				Conditions: conditions,
			},
		}
	}

	jobs := []batchv1.Job{
		ownedRunJob(
			"run-active-job",
			currentRunID,
			nil,
		),
		ownedRunJob(
			"run-successful-job",
			currentRunID,
			[]batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		),
		ownedRunJob(
			"run-failed-job",
			currentRunID,
			[]batchv1.JobCondition{
				{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
				},
			},
		),
		ownedRunJob(
			"other-run-job",
			"other-count-run",
			nil,
		),
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: testForeignJobName,
				Labels: map[string]string{
					runIDLabelKey: currentRunID,
				},
			},
		},
	}

	counts := countWarmupRunJobs(
		policy,
		currentRunID,
		jobs,
	)

	if counts.activeCount != 1 {
		t.Errorf(
			"expected 1 active Job, got %d",
			counts.activeCount,
		)
	}

	if counts.succeededCount != 1 {
		t.Errorf(
			"expected 1 successful Job, got %d",
			counts.succeededCount,
		)
	}

	if counts.failedCount != 1 {
		t.Errorf(
			"expected 1 failed Job, got %d",
			counts.failedCount,
		)
	}
}

func TestBuildWarmupJobInventoryIncludesCurrentRunCounts(
	t *testing.T,
) {
	currentRunID := "inventory-current-run"

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run-inventory-policy",
			UID:  types.UID("run-inventory-policy-uid"),
		},
		Status: cachev1alpha1.ImageWarmupPolicyStatus{
			CurrentRunID: currentRunID,
		},
	}

	ownerReference := *metav1.NewControllerRef(
		policy,
		cachev1alpha1.GroupVersion.WithKind(
			"ImageWarmupPolicy",
		),
	)

	ownedJob := func(
		name string,
		runID string,
		conditions []batchv1.JobCondition,
	) batchv1.Job {
		return batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultObjectName,
				Labels: map[string]string{
					runIDLabelKey: runID,
				},
				OwnerReferences: []metav1.OwnerReference{
					ownerReference,
				},
			},
			Status: batchv1.JobStatus{
				Conditions: conditions,
			},
		}
	}

	jobs := []batchv1.Job{
		ownedJob(
			"inventory-active-current",
			currentRunID,
			nil,
		),
		ownedJob(
			"inventory-complete-current",
			currentRunID,
			[]batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		),
		ownedJob(
			"inventory-active-other",
			"inventory-other-run",
			nil,
		),
	}

	inventory := buildWarmupJobInventory(
		policy,
		jobs,
	)

	if inventory.activeCount != 2 {
		t.Errorf(
			"expected 2 active Jobs across all runs, got %d",
			inventory.activeCount,
		)
	}

	if len(inventory.existingJobs) != 3 {
		t.Errorf(
			"expected 3 existing Jobs, got %d",
			len(inventory.existingJobs),
		)
	}

	if inventory.currentRunCounts.activeCount != 1 {
		t.Errorf(
			"expected 1 active current-run Job, got %d",
			inventory.currentRunCounts.activeCount,
		)
	}

	if inventory.currentRunCounts.succeededCount != 1 {
		t.Errorf(
			"expected 1 successful current-run Job, got %d",
			inventory.currentRunCounts.succeededCount,
		)
	}

	if inventory.currentRunCounts.failedCount != 0 {
		t.Errorf(
			"expected 0 failed current-run Jobs, got %d",
			inventory.currentRunCounts.failedCount,
		)
	}
}
