package controller

import (
	corev1 "k8s.io/api/core/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
)

const (
	nodeReasonEligible              = "eligible"
	nodeReasonMissing               = "node-missing"
	nodeReasonReplaced              = "node-replaced"
	nodeReasonNoLongerSelected      = "node-no-longer-selected"
	nodeReasonUnschedulable         = "node-unschedulable"
	nodeReasonReadyConditionMissing = "ready-condition-missing"
	nodeReasonNotReady              = "node-not-ready"
	nodeReasonReadyUnknown          = "node-readiness-unknown"
	nodeReasonDiskConditionMissing  = "disk-pressure-condition-missing"
	nodeReasonDiskPressure          = "node-disk-pressure"
	nodeReasonDiskPressureUnknown   = "disk-pressure-unknown"
	nodeReasonSkipSelector          = "node-skip-selector"
)

func evaluateNodeEligibility(node *corev1.Node) (bool, string) {
	if node == nil {
		return false, nodeReasonMissing
	}

	if node.Spec.Unschedulable {
		return false, nodeReasonUnschedulable
	}

	readyStatus := corev1.ConditionUnknown
	diskPressureStatus := corev1.ConditionUnknown

	readyConditionFound := false
	diskPressureConditionFound := false

	for _, condition := range node.Status.Conditions {
		switch condition.Type {
		case corev1.NodeReady:
			readyConditionFound = true
			readyStatus = condition.Status
		case corev1.NodeDiskPressure:
			diskPressureConditionFound = true
			diskPressureStatus = condition.Status
		}
	}

	if !readyConditionFound {
		return false, nodeReasonReadyConditionMissing
	}

	switch readyStatus {
	case corev1.ConditionTrue:
		// The Node passed the readiness check.
	case corev1.ConditionFalse:
		return false, nodeReasonNotReady
	default:
		return false, nodeReasonReadyUnknown
	}

	if !diskPressureConditionFound {
		return false, nodeReasonDiskConditionMissing
	}

	switch diskPressureStatus {
	case corev1.ConditionFalse:
		// The Node passed the disk-pressure check.
	case corev1.ConditionTrue:
		return false, nodeReasonDiskPressure
	default:
		return false, nodeReasonDiskPressureUnknown
	}

	return true, nodeReasonEligible
}

func filterHealthyNodes(
	nodes []corev1.Node,
	skipSelectorEnabled bool,
	skipSelector klabels.Selector,
) ([]*corev1.Node, map[string]int) {
	healthyNodes := make([]*corev1.Node, 0, len(nodes))
	skippedReasons := make(map[string]int)

	for i := range nodes {
		node := &nodes[i]

		if skipSelectorEnabled &&
			skipSelector.Matches(klabels.Set(node.Labels)) {
			skippedReasons[nodeReasonSkipSelector]++
			continue
		}

		eligible, reason := evaluateNodeEligibility(node)
		if !eligible {
			skippedReasons[reason]++
			continue
		}

		healthyNodes = append(healthyNodes, node)
	}

	return healthyNodes, skippedReasons
}
