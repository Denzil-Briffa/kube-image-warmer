package controller

import (
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

const (
	nodeWarmupRunPrefix = "node-"
	pendingNodeRequeue  = 30 * time.Second
)

func nodeUIDMap(nodes []*corev1.Node) map[string]*corev1.Node {
	nodesByUID := make(map[string]*corev1.Node, len(nodes))

	for _, node := range nodes {
		if node == nil || node.UID == "" {
			continue
		}

		nodesByUID[string(node.UID)] = node
	}

	return nodesByUID
}

func sortedNodeUIDs(nodesByUID map[string]*corev1.Node) []string {
	uids := make([]string, 0, len(nodesByUID))
	for uid := range nodesByUID {
		uids = append(uids, uid)
	}

	slices.Sort(uids)
	return uids
}

func syncNodeWarmupQueue(
	policy *cachev1alpha1.ImageWarmupPolicy,
	matchedNodes []*corev1.Node,
	healthyNodes []*corev1.Node,
) bool {
	if policy == nil {
		return false
	}

	matchedByUID := nodeUIDMap(matchedNodes)
	healthyByUID := nodeUIDMap(healthyNodes)

	if !policy.Status.NodeCoverageInitialized {
		policy.Status.NodeCoverageInitialized = true
		policy.Status.HandledNodeUIDs =
			sortedNodeUIDs(healthyByUID)
		policy.Status.PendingNodeUIDs = nil
		return true
	}

	handledSet := make(map[string]bool)
	handledUIDs := make([]string, 0, len(policy.Status.HandledNodeUIDs))
	for _, uid := range policy.Status.HandledNodeUIDs {
		if uid == "" || matchedByUID[uid] == nil || handledSet[uid] {
			continue
		}

		handledSet[uid] = true
		handledUIDs = append(handledUIDs, uid)
	}

	pendingSet := make(map[string]bool)
	pendingUIDs := make([]string, 0, len(policy.Status.PendingNodeUIDs))
	for _, uid := range policy.Status.PendingNodeUIDs {
		if uid == "" ||
			matchedByUID[uid] == nil ||
			handledSet[uid] ||
			uid == policy.Status.CurrentRunNodeUID ||
			pendingSet[uid] {
			continue
		}

		pendingSet[uid] = true
		pendingUIDs = append(pendingUIDs, uid)
	}

	for uid := range healthyByUID {
		if handledSet[uid] ||
			pendingSet[uid] ||
			uid == policy.Status.CurrentRunNodeUID {
			continue
		}

		pendingSet[uid] = true
		pendingUIDs = append(pendingUIDs, uid)
	}

	slices.Sort(handledUIDs)
	slices.Sort(pendingUIDs)

	changed := !slices.Equal(
		policy.Status.HandledNodeUIDs,
		handledUIDs,
	) || !slices.Equal(
		policy.Status.PendingNodeUIDs,
		pendingUIDs,
	)

	if changed {
		policy.Status.HandledNodeUIDs = handledUIDs
		policy.Status.PendingNodeUIDs = pendingUIDs
	}

	return changed
}

func ensureNodeWarmupRunState(
	policy *cachev1alpha1.ImageWarmupPolicy,
	healthyNodes []*corev1.Node,
) bool {
	if policy == nil || policy.Status.CurrentRunID != "" {
		return false
	}

	healthyByUID := nodeUIDMap(healthyNodes)
	for index, uid := range policy.Status.PendingNodeUIDs {
		if healthyByUID[uid] == nil {
			continue
		}

		policy.Status.CurrentRunID = nodeWarmupRunPrefix + uid
		policy.Status.CurrentRunTrigger =
			cachev1alpha1.ImageWarmupRunTriggerNode
		policy.Status.CurrentRunNodeUID = uid
		policy.Status.CurrentRunTargetCount = 0
		policy.Status.CurrentRunActiveCount = 0
		policy.Status.CurrentRunSucceededCount = 0
		policy.Status.CurrentRunFailedCount = 0
		policy.Status.PendingNodeUIDs = slices.Delete(
			policy.Status.PendingNodeUIDs,
			index,
			index+1,
		)
		return true
	}

	return false
}

func nodesForCurrentWarmupRun(
	policy *cachev1alpha1.ImageWarmupPolicy,
	healthyNodes []*corev1.Node,
) []*corev1.Node {
	if policy == nil ||
		policy.Status.CurrentRunTrigger !=
			cachev1alpha1.ImageWarmupRunTriggerNode {
		return healthyNodes
	}

	for _, node := range healthyNodes {
		if node != nil &&
			string(node.UID) == policy.Status.CurrentRunNodeUID {
			return []*corev1.Node{node}
		}
	}

	return nil
}

func recordCompletedRunNodeCoverage(
	policy *cachev1alpha1.ImageWarmupPolicy,
	trigger cachev1alpha1.ImageWarmupRunTrigger,
	nodeUID string,
	healthyNodes []*corev1.Node,
) bool {
	if policy == nil {
		return false
	}

	handledSet := make(map[string]bool, len(policy.Status.HandledNodeUIDs))
	for _, uid := range policy.Status.HandledNodeUIDs {
		if uid != "" {
			handledSet[uid] = true
		}
	}

	switch trigger {
	case cachev1alpha1.ImageWarmupRunTriggerNode:
		if nodeUID != "" {
			handledSet[nodeUID] = true
		}
	case cachev1alpha1.ImageWarmupRunTriggerInitial,
		cachev1alpha1.ImageWarmupRunTriggerScheduled:
		for uid := range nodeUIDMap(healthyNodes) {
			handledSet[uid] = true
		}
	}

	handledUIDs := make([]string, 0, len(handledSet))
	for uid := range handledSet {
		handledUIDs = append(handledUIDs, uid)
	}
	slices.Sort(handledUIDs)

	pendingUIDs := make([]string, 0, len(policy.Status.PendingNodeUIDs))
	for _, uid := range policy.Status.PendingNodeUIDs {
		if !handledSet[uid] {
			pendingUIDs = append(pendingUIDs, uid)
		}
	}

	changed := !slices.Equal(
		policy.Status.HandledNodeUIDs,
		handledUIDs,
	) || !slices.Equal(
		policy.Status.PendingNodeUIDs,
		pendingUIDs,
	)

	if changed {
		policy.Status.HandledNodeUIDs = handledUIDs
		policy.Status.PendingNodeUIDs = pendingUIDs
	}

	return changed
}

func nodeRunRequeueAfter(
	policy *cachev1alpha1.ImageWarmupPolicy,
) time.Duration {
	if policy == nil {
		return 0
	}
	if len(policy.Status.PendingNodeUIDs) == 0 &&
		policy.Status.CurrentRunTrigger != cachev1alpha1.ImageWarmupRunTriggerNode {
		return 0
	}

	return pendingNodeRequeue
}

func minimumPositiveDuration(
	first time.Duration,
	second time.Duration,
) time.Duration {
	if first <= 0 {
		return second
	}
	if second <= 0 || first < second {
		return first
	}

	return second
}
