package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
)

type warmupJobResult struct {
	job        *batchv1.Job
	created    bool
	skipReason string
}

type warmupExecutionSummary struct {
	createdCount   int
	existingCount  int
	skippedReasons map[string]int
	targetCount    int
	runCounts      warmupRunJobCounts
}

func (r *ImageWarmupPolicyReconciler) ensureWarmupJob(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	nodeName string,
	image discovery.DiscoveredImage,
) (warmupJobResult, error) {
	targetNode, reason, err :=
		r.revalidateNodeForWarmup(ctx, nodeName)
	if err != nil {
		return warmupJobResult{}, err
	}

	if targetNode == nil {
		return warmupJobResult{
			skipReason: reason,
		}, nil
	}

	job := buildWarmupJob(
		policy,
		targetNode,
		image,
		runID,
	)

	err = r.Create(ctx, job)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return warmupJobResult{}, fmt.Errorf(
				"create warming Job %q/%q %w",
				job.Namespace,
				job.Name,
				err,
			)
		}

		existingJob := &batchv1.Job{}

		err = r.Get(
			ctx,
			types.NamespacedName{
				Name:      job.Name,
				Namespace: job.Namespace,
			},
			existingJob,
		)
		if err != nil {
			return warmupJobResult{}, fmt.Errorf(
				"get existing warming Job %q/%q %w",
				job.Namespace,
				job.Name,
				err,
			)
		}

		if !metav1.IsControlledBy(existingJob, policy) {
			return warmupJobResult{}, fmt.Errorf(
				"existing warming Job %q/%q is not controlled by policy %q",
				job.Name,
				job.Namespace,
				policy.Name,
			)
		}

		return warmupJobResult{
			job:     existingJob,
			created: false,
		}, nil
	}

	return warmupJobResult{
		job:     job,
		created: true,
	}, nil

}

func (r *ImageWarmupPolicyReconciler) executePendingWarmupTargets(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
	runID string,
	targets []warmupTarget,
) (warmupExecutionSummary, error) {
	summary := warmupExecutionSummary{
		skippedReasons: make(map[string]int),
	}

	for _, target := range targets {
		if target.node == nil {
			summary.skippedReasons[nodeReasonMissing]++
			continue
		}

		result, err := r.ensureWarmupJob(
			ctx,
			policy,
			runID,
			target.node.Name,
			target.image,
		)
		if err != nil {
			return summary, fmt.Errorf(
				"ensure warming Job for node %q and image %q: %w",
				target.node.Name,
				target.image.Image,
				err,
			)
		}

		if result.skipReason != "" {
			summary.skippedReasons[result.skipReason]++
			continue
		}

		if result.created {
			summary.createdCount++
			summary.runCounts.activeCount++
			continue
		}

		if result.job != nil {
			summary.existingCount++

			existingCounts := countWarmupRunJobs(
				policy,
				runID,
				[]batchv1.Job{*result.job},
			)

			summary.runCounts.activeCount +=
				existingCounts.activeCount
			summary.runCounts.succeededCount +=
				existingCounts.succeededCount
			summary.runCounts.failedCount +=
				existingCounts.failedCount
		}
	}

	return summary, nil
}

func (r *ImageWarmupPolicyReconciler) ensureCurrentWarmupRunJobs(
	ctx context.Context,
	policy *cachev1alpha1.ImageWarmupPolicy,
	nodes []*corev1.Node,
	images []discovery.DiscoveredImage,
) (warmupExecutionSummary, error) {
	if policy == nil || policy.Status.CurrentRunID == "" {
		return warmupExecutionSummary{
			skippedReasons: make(map[string]int),
		}, nil
	}

	inventory, err := r.loadWarmupJobInventory(
		ctx,
		policy,
	)
	if err != nil {
		return warmupExecutionSummary{}, fmt.Errorf(
			"load warming Job inventory: %w",
			err,
		)
	}

	targetCount := 0

	for _, node := range nodes {
		if node == nil {
			continue
		}

		targetCount += len(images)
	}

	pendingTargets := planPendingWarmupTargets(
		policy,
		policy.Status.CurrentRunID,
		nodes,
		images,
		inventory,
	)

	summary, executionErr := r.executePendingWarmupTargets(
		ctx,
		policy,
		policy.Status.CurrentRunID,
		pendingTargets,
	)

	summary.targetCount = targetCount

	summary.runCounts.activeCount +=
		inventory.currentRunCounts.activeCount

	summary.runCounts.succeededCount +=
		inventory.currentRunCounts.succeededCount

	summary.runCounts.failedCount +=
		inventory.currentRunCounts.failedCount

	return summary, executionErr
}
