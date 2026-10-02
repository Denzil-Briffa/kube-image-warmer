/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"

	"github.com/Denzil-Briffa/kube-image-warmer/internal/discovery"
	"github.com/Denzil-Briffa/kube-image-warmer/internal/scheduling"
)

// ImageWarmupPolicyReconciler reconciles a ImageWarmupPolicy object
type ImageWarmupPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Schedule scheduling.Schedule
	Now      func() time.Time
}

// +kubebuilder:rbac:groups=cache.denzil-briffa.github.io,resources=imagewarmuppolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=cache.denzil-briffa.github.io,resources=imagewarmuppolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.denzil-briffa.github.io,resources=imagewarmuppolicies/finalizers,verbs=update

// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ImageWarmupPolicyReconciler) Reconcile(
	ctx context.Context, req ctrl.Request,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var policy cachev1alpha1.ImageWarmupPolicy

	err := r.Get(ctx, req.NamespacedName, &policy)
	if err != nil {
		logger.Error(err, "unable to fetch ImageWarmupPolicy")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if policy.Spec.Suspend {
		cleanupEnabledCount, cleanupErr :=
			r.enableFinishedWarmupJobCleanup(
				ctx,
				&policy,
			)
		if cleanupErr != nil {
			logger.Error(
				cleanupErr,
				"Could not enable cleanup for finished warming Jobs",
			)
			return ctrl.Result{}, cleanupErr
		}

		logger.Info(
			"Policy is suspended",
			"policy", policy.Name,
			"cleanupEnabledWarmupJobs", cleanupEnabledCount,
		)
		return ctrl.Result{}, nil
	}

	// Check the NamespaceSelector and NamespaceList to determine which namespaces to scan for workloads.
	namespaceSelector, err := metav1.LabelSelectorAsSelector(
		&policy.Spec.NamespaceSelector,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	var namespaces corev1.NamespaceList

	err = r.List(
		ctx,
		&namespaces,
		client.MatchingLabelsSelector{Selector: namespaceSelector},
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	allowedNamespaces := make(map[string]bool)

	for _, namespace := range namespaces.Items {
		if slices.Contains(policy.Spec.NamespaceSkipList, namespace.Name) {
			continue
		}

		if len(policy.Spec.NamespaceList) > 0 && !slices.Contains(policy.Spec.NamespaceList, namespace.Name) {
			continue
		}

		allowedNamespaces[namespace.Name] = true
	}

	nodeSelection, err := r.selectHealthyNodes(
		ctx,
		&policy,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Build a list of workloads to go through and discover images from.
	var deployments appsv1.DeploymentList
	var statefulSets appsv1.StatefulSetList
	var daemonSets appsv1.DaemonSetList
	var cronJobs batchv1.CronJobList

	workLoadLists := []client.ObjectList{
		&deployments,
		&statefulSets,
		&daemonSets,
		&cronJobs,
	}

	// Check the WorkLoadSelector and WorkloadSkipSelector to determine which workloads to scan for images.
	workloadSelector, err := metav1.LabelSelectorAsSelector(
		&policy.Spec.WorkloadSelector,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	workloadSkipSelector, err := metav1.LabelSelectorAsSelector(
		&policy.Spec.WorkloadSkipSelector,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	workloadSkipSelectorEnabled := len(policy.Spec.WorkloadSkipSelector.MatchLabels) > 0 || len(policy.Spec.WorkloadSkipSelector.MatchExpressions) > 0

	for _, workLoadList := range workLoadLists {
		err = r.List(
			ctx,
			workLoadList,
			client.MatchingLabelsSelector{Selector: workloadSelector},
		)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	uniqueImages := make(map[string]bool)
	var discoveredImages []discovery.DiscoveredImage

	// Check for unique images in each of the workload
	for i := range deployments.Items {
		deployment := &deployments.Items[i]

		if !shouldDiscoverWorkload(
			deployment.Namespace,
			deployment.Labels,
			allowedNamespaces,
			workloadSkipSelectorEnabled,
			workloadSkipSelector,
		) {
			continue
		}

		discoveredImages = addDiscoveredImages(
			discoveredImages,
			uniqueImages,
			discovery.DiscoveredImagesFromDeployment(deployment),
		)
	}

	for i := range statefulSets.Items {
		statefulSet := &statefulSets.Items[i]

		if !shouldDiscoverWorkload(
			statefulSet.Namespace,
			statefulSet.Labels,
			allowedNamespaces,
			workloadSkipSelectorEnabled,
			workloadSkipSelector,
		) {
			continue
		}

		discoveredImages = addDiscoveredImages(
			discoveredImages,
			uniqueImages,
			discovery.DiscoveredImagesFromStatefulSet(statefulSet),
		)
	}

	for i := range daemonSets.Items {
		daemonSet := &daemonSets.Items[i]

		if !shouldDiscoverWorkload(
			daemonSet.Namespace,
			daemonSet.Labels,
			allowedNamespaces,
			workloadSkipSelectorEnabled,
			workloadSkipSelector,
		) {
			continue
		}

		discoveredImages = addDiscoveredImages(
			discoveredImages,
			uniqueImages,
			discovery.DiscoveredImagesFromDaemonSet(daemonSet),
		)
	}

	for i := range cronJobs.Items {
		cronJob := &cronJobs.Items[i]

		if !shouldDiscoverWorkload(
			cronJob.Namespace,
			cronJob.Labels,
			allowedNamespaces,
			workloadSkipSelectorEnabled,
			workloadSkipSelector,
		) {
			continue
		}

		discoveredImages = addDiscoveredImages(
			discoveredImages,
			uniqueImages,
			discovery.DiscoveredImagesFromCronJob(cronJob),
		)
	}

	imageCount := int32(len(uniqueImages))

	statusChanged := false

	if policy.Status.DiscoveredImageCount != imageCount {
		policy.Status.DiscoveredImageCount = imageCount
		statusChanged = true
	}

	resolvedImages, err := resolveImagePullSecrets(
		ctx,
		r.Client,
		discoveredImages,
	)
	if err != nil {
		logger.Error(err, "Could not resolve image pull secrets")
		return ctrl.Result{}, err
	}

	imagePullContexts := deduplicateImagePullContexts(resolvedImages)

	warmupSummary, runStatusChanged, err := r.reconcileWarmupRun(
		ctx,
		&policy,
		nodeSelection,
		imagePullContexts,
	)
	if err != nil {
		logger.Error(err, "Could not reconcile warming Jobs")
		return ctrl.Result{}, err
	}

	if runStatusChanged {
		statusChanged = true
	}

	cleanupEnabledCount, err :=
		r.persistWarmupStatusAndEnableCleanup(
			ctx,
			&policy,
			statusChanged,
		)
	if err != nil {
		logger.Error(
			err,
			"Could not persist warming status or enable Job cleanup",
		)
		return ctrl.Result{}, err
	}

	requeueAfter := minimumPositiveDuration(
		scheduledRunRequeueAfter(&policy, r.currentTime()),
		nodeRunRequeueAfter(&policy),
	)

	logger.Info(
		"Image discovery completed",
		"policy", policy.Name,
		"selectedNamespaces", len(allowedNamespaces),
		"matchedNodes", nodeSelection.matchedNodeCount,
		"selectedNodes", nodeSelection.selectedNodeCount,
		"healthyNodes", len(nodeSelection.healthyNodes),
		"skippedNodeReasons", nodeSelection.skippedReasons,
		"deployments", len(deployments.Items),
		"statefulSets", len(statefulSets.Items),
		"daemonSets", len(daemonSets.Items),
		"cronJobs", len(cronJobs.Items),
		"uniqueImages", imageCount,
		"resolvedImageContexts", len(resolvedImages),
		"uniqueImagePullContexts", len(imagePullContexts),
		"createdWarmupJobs", warmupSummary.createdCount,
		"existingWarmupJobs", warmupSummary.existingCount,
		"warmupSkippedReasons", warmupSummary.skippedReasons,
		"cleanupEnabledWarmupJobs", cleanupEnabledCount,
		"nextScheduledRunTime", policy.Status.NextScheduledRunTime,
		"requeueAfter", requeueAfter,
		"handledNodes", len(policy.Status.HandledNodeUIDs),
		"pendingNodes", len(policy.Status.PendingNodeUIDs),
		"currentRunNodeUID", policy.Status.CurrentRunNodeUID,
	)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func shouldDiscoverWorkload(
	namespace string,
	workloadLabels map[string]string,
	allowedNamespaces map[string]bool,
	skipSelectorEnabled bool,
	skipSelector klabels.Selector,
) bool {
	if !allowedNamespaces[namespace] {
		return false
	}

	if skipSelectorEnabled &&
		skipSelector.Matches(klabels.Set(workloadLabels)) {
		return false
	}

	return true
}

func addDiscoveredImages(
	discoveredImages []discovery.DiscoveredImage,
	uniqueImages map[string]bool,
	newImages []discovery.DiscoveredImage,
) []discovery.DiscoveredImage {
	for _, discoveredImage := range newImages {
		uniqueImages[discoveredImage.Image] = true
	}

	return append(discoveredImages, newImages...)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageWarmupPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.ImageWarmupPolicy{}).
		Owns(&batchv1.Job{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.mapNodeToPolicies),
			builder.WithPredicates(nodeWarmupPredicate()),
		).
		Named("imagewarmuppolicy").
		Complete(r)
}
