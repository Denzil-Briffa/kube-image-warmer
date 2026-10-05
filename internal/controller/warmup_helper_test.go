package controller

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

const testWarmupHelperImage = "example/helper:test"

func TestBuildWarmupJobInstallsShellIndependentHelper(t *testing.T) {
	job := buildWarmupJob(
		&cachev1alpha1.ImageWarmupPolicy{},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testTalosNodeName}},
		discovery.DiscoveredImage{Image: "example/scratch:test"},
		"helper-run",
		testWarmupHelperImage,
	)
	pod := job.Spec.Template.Spec
	if len(pod.InitContainers) != 1 || len(pod.Volumes) != 1 || len(pod.Containers) != 1 {
		t.Fatalf("expected one installer, volume and target container, got %v", pod)
	}
	installer := pod.InitContainers[0]
	target := pod.Containers[0]
	if installer.Image != testWarmupHelperImage ||
		!slices.Equal(installer.Command, []string{"/warmup-helper"}) ||
		!slices.Equal(installer.Args, []string{"--install", warmupHelperPath}) {
		t.Fatalf("unexpected installer: %v", installer)
	}
	if target.WorkingDir != warmupHelperMount ||
		!slices.Equal(target.Command, []string{warmupHelperPath}) ||
		!slices.Equal(target.Args, []string{"--complete"}) {
		t.Fatalf("target must override both application entrypoint and arguments: %v", target)
	}
	if len(installer.VolumeMounts) != 1 || installer.VolumeMounts[0].ReadOnly ||
		len(target.VolumeMounts) != 1 || !target.VolumeMounts[0].ReadOnly ||
		installer.VolumeMounts[0].Name != target.VolumeMounts[0].Name {
		t.Fatal("installer needs a writable shared volume; target needs the same volume read-only")
	}
	volume := pod.Volumes[0]
	if volume.EmptyDir == nil || volume.EmptyDir.Medium != "" ||
		volume.EmptyDir.SizeLimit == nil || volume.EmptyDir.SizeLimit.Value() != 16*1024*1024 {
		t.Fatalf("expected bounded disk-backed emptyDir, got %v", volume)
	}
	assertWarmupHelperSecurity(t, pod)
}

func assertWarmupHelperSecurity(t *testing.T, pod corev1.PodSpec) {
	t.Helper()
	if pod.SecurityContext == nil || pod.SecurityContext.FSGroup == nil ||
		pod.SecurityContext.RunAsUser == nil ||
		*pod.SecurityContext.FSGroup != *pod.SecurityContext.RunAsUser {
		t.Fatal("installer must have access to the shared volume without root")
	}
	for _, container := range slices.Concat(pod.InitContainers, pod.Containers) {
		security := container.SecurityContext
		if security == nil || security.AllowPrivilegeEscalation == nil ||
			*security.AllowPrivilegeEscalation || security.ReadOnlyRootFilesystem == nil ||
			!*security.ReadOnlyRootFilesystem || security.Capabilities == nil ||
			!slices.Equal(security.Capabilities.Drop, []corev1.Capability{"ALL"}) {
			t.Fatalf("container lacks required security settings: %v", container)
		}
	}
}

func TestEnsureWarmupJobRequiresHelperImage(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: testTalosNodeName},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
		}},
	}
	reconciler := &ImageWarmupPolicyReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build(),
	}
	result, err := reconciler.ensureWarmupJob(
		context.Background(), &cachev1alpha1.ImageWarmupPolicy{}, "helper-run",
		node, discovery.DiscoveredImage{Image: testBusyBoxImage},
	)
	if err == nil || result.created || result.job != nil {
		t.Fatal("missing helper configuration must fail before submitting a Job")
	}
}
