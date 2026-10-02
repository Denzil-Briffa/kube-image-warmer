package controller

import (
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

func TestBuildWarmupJobSetsIdentity(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testTalosNodeName,
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	runID := "identity-run"

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		runID,
	)

	wantName := warmupJobName(
		policy,
		runID,
		targetNode,
		image,
	)

	if job.Name != wantName {
		t.Errorf(
			"expected Job name %q, got %q",
			wantName,
			job.Name,
		)
	}

	if job.Namespace != image.Namespace {
		t.Errorf(
			"expected Job namespace %q, got %q",
			image.Namespace,
			job.Namespace,
		)
	}
}

func TestBuildWarmupJobConfiguresContainer(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testTalosNodeName,
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		"warmup-test",
	)

	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf(
			"expected one container, got %d",
			len(job.Spec.Template.Spec.Containers),
		)
	}

	container := job.Spec.Template.Spec.Containers[0]

	if container.Image != image.Image {
		t.Errorf(
			"expected image %q, got %q",
			image.Image,
			container.Image,
		)
	}

	if container.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf(
			"expected pull policy %q, got %q",
			corev1.PullIfNotPresent,
			container.ImagePullPolicy,
		)
	}

	wantCommand := []string{"/bin/sh", "-c", "exit 0"}

	if !slices.Equal(container.Command, wantCommand) {
		t.Errorf(
			"expected command %v, got %v",
			wantCommand,
			container.Command,
		)
	}

	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf(
			"expected restart policy %q, got %q",
			corev1.RestartPolicyNever,
			job.Spec.Template.Spec.RestartPolicy,
		)
	}
}

func TestBuildWarmupJobTargetsNode(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: testWarmupPolicyName,
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testTalosNodeName,
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: defaultObjectName,
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		"warmup-test",
	)

	affinity := job.Spec.Template.Spec.Affinity
	if affinity == nil {
		t.Fatal("expected Pod affinity to be configured")
	}

	nodeAffinity := affinity.NodeAffinity
	if nodeAffinity == nil {
		t.Fatal("expected node affinity to be configured")
	}

	required := nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		t.Fatal("expected required node affinity to be configured")
	}

	if len(required.NodeSelectorTerms) != 1 {
		t.Fatalf(
			"expected one node selector term, got %d",
			len(required.NodeSelectorTerms),
		)
	}

	matchFields := required.NodeSelectorTerms[0].MatchFields
	if len(matchFields) != 1 {
		t.Fatalf(
			"expected one node match field, got %d",
			len(matchFields),
		)
	}

	requirement := matchFields[0]

	if requirement.Key != "metadata.name" {
		t.Errorf(
			"expected match field key %q, got %q",
			"metadata.name",
			requirement.Key,
		)
	}

	if requirement.Operator != corev1.NodeSelectorOpIn {
		t.Errorf(
			"expected operator %q, got %q",
			corev1.NodeSelectorOpIn,
			requirement.Operator,
		)
	}

	if !slices.Equal(requirement.Values, []string{targetNode.Name}) {
		t.Errorf(
			"expected node values %v, got %v",
			[]string{targetNode.Name},
			requirement.Values,
		)
	}
}

func TestBuildWarmupJobConfiguresPullCredentials(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "credential-policy",
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "credential-worker",
		},
	}

	wantSecrets := []corev1.LocalObjectReference{
		{Name: testPrimaryPullSecretName},
		{Name: testFallbackPullSecretName},
	}

	image := discovery.DiscoveredImage{
		Image:            testRegistryApplicationImage,
		Namespace:        testApplicationName,
		ImagePullSecrets: wantSecrets,
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		"credential-warmup",
	)

	automountToken :=
		job.Spec.Template.Spec.AutomountServiceAccountToken

	if automountToken == nil {
		t.Fatal(
			"expected automountServiceAccountToken to be explicitly configured",
		)
	}

	if *automountToken {
		t.Error(
			"expected automountServiceAccountToken to be false",
		)
	}

	if !slices.Equal(
		job.Spec.Template.Spec.ImagePullSecrets,
		wantSecrets,
	) {
		t.Errorf(
			"expected image pull secrets %v, got %v",
			wantSecrets,
			job.Spec.Template.Spec.ImagePullSecrets,
		)
	}
}

func TestBuildWarmupJobConfiguresLifecycleLimits(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lifecycle-policy",
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "lifecycle-worker",
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: "lifecycle-test",
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		"lifecycle-warmup",
	)

	if job.Spec.BackoffLimit == nil {
		t.Fatal("expected backoffLimit to be configured")
	}

	if *job.Spec.BackoffLimit != 0 {
		t.Errorf(
			"expected backoffLimit 0, got %d",
			*job.Spec.BackoffLimit,
		)
	}

	if job.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("expected activeDeadlineSeconds to be configured")
	}

	if *job.Spec.ActiveDeadlineSeconds != 300 {
		t.Errorf(
			"expected activeDeadlineSeconds 300, got %d",
			*job.Spec.ActiveDeadlineSeconds,
		)
	}

	if job.Spec.TTLSecondsAfterFinished != nil {
		t.Errorf(
			"expected ttlSecondsAfterFinished to be deferred, got %d",
			*job.Spec.TTLSecondsAfterFinished,
		)
	}
}

func TestBuildWarmupJobUsesPolicyLifecycleSettings(t *testing.T) {
	backoffLimit := int32(3)
	activeDeadlineSeconds := int64(900)
	ttlSecondsAfterFinished := int32(7200)

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-lifecycle-policy",
		},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &activeDeadlineSeconds,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-lifecycle-worker",
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: "custom-lifecycle-test",
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		"custom-lifecycle-warmup",
	)

	if job.Spec.BackoffLimit == nil {
		t.Fatal("expected backoffLimit to be configured")
	}

	if *job.Spec.BackoffLimit != backoffLimit {
		t.Errorf(
			"expected backoffLimit %d, got %d",
			backoffLimit,
			*job.Spec.BackoffLimit,
		)
	}

	if job.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("expected activeDeadlineSeconds to be configured")
	}

	if *job.Spec.ActiveDeadlineSeconds != activeDeadlineSeconds {
		t.Errorf(
			"expected activeDeadlineSeconds %d, got %d",
			activeDeadlineSeconds,
			*job.Spec.ActiveDeadlineSeconds,
		)
	}

	if job.Spec.TTLSecondsAfterFinished != nil {
		t.Errorf(
			"expected ttlSecondsAfterFinished to be deferred, got %d",
			*job.Spec.TTLSecondsAfterFinished,
		)
	}
}

func TestBuildWarmupJobConfiguresOwnershipAndLabels(t *testing.T) {
	runID := "ownership-warmup"

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ownership-policy",
			UID:  types.UID("policy-uid"),
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ownership-worker",
			UID:  types.UID("node-uid"),
		},
	}

	image := discovery.DiscoveredImage{
		Image:     testBusyBoxImage,
		Namespace: "ownership-test",
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		runID,
	)

	if !metav1.IsControlledBy(job, policy) {
		t.Fatal("expected policy to control the Job")
	}

	controllerReference := metav1.GetControllerOf(job)
	if controllerReference == nil {
		t.Fatal("expected a controller owner reference")
	}

	if controllerReference.APIVersion !=
		cachev1alpha1.GroupVersion.String() {
		t.Errorf(
			"expected owner API version %q, got %q",
			cachev1alpha1.GroupVersion.String(),
			controllerReference.APIVersion,
		)
	}

	if controllerReference.Kind != "ImageWarmupPolicy" {
		t.Errorf(
			"expected owner kind %q, got %q",
			"ImageWarmupPolicy",
			controllerReference.Kind,
		)
	}

	wantLabels := map[string]string{
		"app.kubernetes.io/managed-by":                  "kube-image-warmer",
		"cache.denzil-briffa.github.io/policy-uid":      "policy-uid",
		"cache.denzil-briffa.github.io/target-node-uid": "node-uid",
		"cache.denzil-briffa.github.io/run-id":          runID,
	}

	for key, wantValue := range wantLabels {
		if job.Labels[key] != wantValue {
			t.Errorf(
				"expected Job label %q=%q, got %q",
				key,
				wantValue,
				job.Labels[key],
			)
		}

		if job.Spec.Template.Labels[key] != wantValue {
			t.Errorf(
				"expected Pod label %q=%q, got %q",
				key,
				wantValue,
				job.Spec.Template.Labels[key],
			)
		}
	}
}
