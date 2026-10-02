package controller

import (
	"cmp"
	"slices"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type warmupTarget struct {
	node    *corev1.Node
	image   discovery.DiscoveredImage
	jobName string
}

func buildWarmupTargets(
	nodes []*corev1.Node,
	images []discovery.DiscoveredImage,
) []warmupTarget {
	targets := make(
		[]warmupTarget,
		0,
		len(nodes)*len(images),
	)

	for _, node := range nodes {
		if node == nil {
			continue
		}

		for i := range images {
			targets = append(
				targets,
				warmupTarget{
					node:  node,
					image: images[i],
				},
			)
		}
	}

	return targets
}

func selectPendingWarmupTargets(
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	targets []warmupTarget,
	existingJobs map[types.NamespacedName]bool,
	capacity int,
) []warmupTarget {
	if policy == nil || capacity <= 0 {
		return nil
	}

	pendingTargets := make(
		[]warmupTarget,
		0,
		len(targets),
	)

	for _, target := range targets {
		if target.node == nil {
			continue
		}

		jobName := warmupJobName(
			policy,
			runID,
			target.node,
			target.image,
		)

		jobKey := types.NamespacedName{
			Name:      jobName,
			Namespace: target.image.Namespace,
		}

		if existingJobs[jobKey] {
			continue
		}

		target.jobName = jobName
		pendingTargets = append(
			pendingTargets,
			target,
		)
	}

	slices.SortFunc(
		pendingTargets,
		func(first warmupTarget, second warmupTarget) int {
			firstIsNewNode := slices.Contains(
				policy.Status.PendingNodeUIDs, string(first.node.UID),
			)
			secondIsNewNode := slices.Contains(
				policy.Status.PendingNodeUIDs, string(second.node.UID),
			)
			if firstIsNewNode != secondIsNewNode {
				if firstIsNewNode {
					return -1
				}
				return 1
			}

			if namespaceComparison := cmp.Compare(
				first.image.Namespace,
				second.image.Namespace,
			); namespaceComparison != 0 {
				return namespaceComparison
			}

			return cmp.Compare(
				first.jobName,
				second.jobName,
			)
		},
	)

	if len(pendingTargets) > capacity {
		pendingTargets = pendingTargets[:capacity]
	}

	return pendingTargets
}

func countDesiredWarmupTargets(
	policy *cachev1alpha1.ImageWarmupPolicy,
	nodes []*corev1.Node,
	images []discovery.DiscoveredImage,
	inventory warmupJobInventory,
) int {
	keys := make(map[types.NamespacedName]bool, len(inventory.currentRunJobs))
	for key := range inventory.currentRunJobs {
		keys[key] = true
	}
	for _, target := range buildWarmupTargets(nodes, images) {
		keys[types.NamespacedName{
			Name:      warmupJobName(policy, policy.Status.CurrentRunID, target.node, target.image),
			Namespace: target.image.Namespace,
		}] = true
	}
	return len(keys)
}

func planPendingWarmupTargets(
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	nodes []*corev1.Node,
	images []discovery.DiscoveredImage,
	inventory warmupJobInventory,
) []warmupTarget {
	capacity := availableWarmupJobCapacity(
		policy,
		inventory.activeCount,
	)

	targets := buildWarmupTargets(
		nodes,
		images,
	)

	return selectPendingWarmupTargets(
		policy,
		runID,
		targets,
		inventory.existingJobs,
		capacity,
	)
}
