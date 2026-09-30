package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

func TestBuildWarmupTargets(t *testing.T) {
	firstNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-a",
		},
	}

	secondNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-b",
		},
	}

	images := []discovery.DiscoveredImage{
		{
			Image:     "example/application:v1",
			Namespace: testTeamANamespace,
		},
		{
			Image:     "example/database:v1",
			Namespace: testTeamBNamespace,
		},
	}

	targets := buildWarmupTargets(
		[]*corev1.Node{
			firstNode,
			nil,
			secondNode,
		},
		images,
	)

	if len(targets) != 4 {
		t.Fatalf(
			"expected 4 targets, got %d",
			len(targets),
		)
	}

	gotPairs := make(map[string]bool)

	for _, target := range targets {
		if target.node == nil {
			t.Fatal("expected nil Nodes to be skipped")
		}

		key :=
			target.node.Name + "\x00" +
				target.image.Namespace + "\x00" +
				target.image.Image

		gotPairs[key] = true
	}

	wantPairs := []string{
		"worker-a\x00team-a\x00example/application:v1",
		"worker-a\x00team-b\x00example/database:v1",
		"worker-b\x00team-a\x00example/application:v1",
		"worker-b\x00team-b\x00example/database:v1",
	}

	for _, wantPair := range wantPairs {
		if !gotPairs[wantPair] {
			t.Errorf(
				"expected target pair %q",
				wantPair,
			)
		}
	}
}

func TestSelectPendingWarmupTargets(t *testing.T) {
	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "planning-policy",
			UID:  types.UID("planning-policy-uid"),
		},
	}

	firstNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-a",
			UID:  types.UID("worker-a-uid"),
		},
	}

	secondNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-b",
			UID:  types.UID("worker-b-uid"),
		},
	}

	images := []discovery.DiscoveredImage{
		{
			Image:     "example/application:v1",
			Namespace: testTeamANamespace,
		},
		{
			Image:     "example/database:v1",
			Namespace: testTeamBNamespace,
		},
	}

	runID := "planning-run"

	targets := buildWarmupTargets(
		[]*corev1.Node{
			firstNode,
			secondNode,
		},
		images,
	)

	existingName := warmupJobName(
		policy,
		runID,
		firstNode,
		images[0],
	)

	existingJobs := map[types.NamespacedName]bool{
		{
			Name:      existingName,
			Namespace: images[0].Namespace,
		}: true,
	}

	pendingTargets := selectPendingWarmupTargets(
		policy,
		runID,
		targets,
		existingJobs,
		2,
	)

	if len(pendingTargets) != 2 {
		t.Fatalf(
			"expected 2 pending targets, got %d",
			len(pendingTargets),
		)
	}

	previousSortKey := ""

	for _, target := range pendingTargets {
		if target.jobName == "" {
			t.Error("expected a deterministic Job name")
		}

		jobKey := types.NamespacedName{
			Name:      target.jobName,
			Namespace: target.image.Namespace,
		}

		if existingJobs[jobKey] {
			t.Errorf(
				"existing Job target %q/%q was selected",
				jobKey.Namespace,
				jobKey.Name,
			)
		}

		sortKey :=
			target.image.Namespace + "\x00" +
				target.jobName

		if previousSortKey != "" &&
			sortKey < previousSortKey {
			t.Errorf(
				"targets are not deterministically sorted",
			)
		}

		previousSortKey = sortKey
	}
}

func TestPlanPendingWarmupTargetsUsesAvailableCapacity(t *testing.T) {
	maxConcurrentJobs := int32(2)

	policy := &cachev1alpha1.ImageWarmupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "capacity-policy",
			UID:  types.UID("capacity-policy-uid"),
		},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			MaxConcurrentJobs: &maxConcurrentJobs,
		},
	}

	firstNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "capacity-worker-a",
			UID:  types.UID("capacity-worker-a-uid"),
		},
	}

	secondNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "capacity-worker-b",
			UID:  types.UID("capacity-worker-b-uid"),
		},
	}

	images := []discovery.DiscoveredImage{
		{
			Image:     "example/frontend:capacity",
			Namespace: "capacity-team-a",
		},
		{
			Image:     "example/backend:capacity",
			Namespace: "capacity-team-b",
		},
	}

	runID := "capacity-run"

	existingName := warmupJobName(
		policy,
		runID,
		firstNode,
		images[0],
	)

	existingKey := types.NamespacedName{
		Name:      existingName,
		Namespace: images[0].Namespace,
	}

	inventory := warmupJobInventory{
		activeCount: 1,
		existingJobs: map[types.NamespacedName]bool{
			existingKey: true,
		},
	}

	pendingTargets := planPendingWarmupTargets(
		policy,
		runID,
		[]*corev1.Node{
			firstNode,
			secondNode,
		},
		images,
		inventory,
	)

	if len(pendingTargets) != 1 {
		t.Fatalf(
			"expected 1 pending target, got %d",
			len(pendingTargets),
		)
	}

	selectedKey := types.NamespacedName{
		Name:      pendingTargets[0].jobName,
		Namespace: pendingTargets[0].image.Namespace,
	}

	if selectedKey == existingKey {
		t.Errorf(
			"existing Job target %q/%q was selected",
			selectedKey.Namespace,
			selectedKey.Name,
		)
	}
}
