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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
)

var _ = Describe("ImageWarmupPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName = "test-resource"
		)

		ctx := context.Background()

		createDefaultServiceAccount := func(namespace string) {
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultObjectName,
					Namespace: namespace,
				},
			}

			Expect(k8sClient.Create(ctx, serviceAccount)).To(Succeed())
			DeferCleanup(func() {
				Expect(k8sClient.Delete(ctx, serviceAccount)).To(Succeed())
			})
		}

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		imagewarmuppolicy := &cachev1alpha1.ImageWarmupPolicy{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ImageWarmupPolicy")
			err := k8sClient.Get(ctx, typeNamespacedName, imagewarmuppolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &cachev1alpha1.ImageWarmupPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &cachev1alpha1.ImageWarmupPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ImageWarmupPolicy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should count unique images across all four workload types", func() {
			labels := map[string]string{"app": "discovery-count-test"}

			metadata := metav1.ObjectMeta{
				Name:      "discovery-count-test",
				Namespace: defaultObjectName,
			}

			createDefaultServiceAccount(defaultObjectName)

			templateFor := func(image string) corev1.PodTemplateSpec {
				return corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: testApplicationName, Image: image},
						},
						InitContainers: []corev1.Container{
							{Name: "prepare", Image: "example/shared-init:v1"},
						},
					},
				}
			}

			cronTemplate := templateFor("example/scheduled-task:v1")
			cronTemplate.Spec.RestartPolicy = corev1.RestartPolicyNever

			workloads := []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metadata,
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{MatchLabels: labels},
						Template: templateFor("example/web-service:v1"),
					},
				},
				&appsv1.StatefulSet{
					ObjectMeta: metadata,
					Spec: appsv1.StatefulSetSpec{
						ServiceName: "discovery-test-service",
						Selector:    &metav1.LabelSelector{MatchLabels: labels},
						Template:    templateFor("example/data-service:v1"),
					},
				},
				&appsv1.DaemonSet{
					ObjectMeta: metadata,
					Spec: appsv1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{MatchLabels: labels},
						Template: templateFor("example/node-service:v1"),
					},
				},
				&batchv1.CronJob{
					ObjectMeta: metadata,
					Spec: batchv1.CronJobSpec{
						Schedule: "15 * * * *",
						JobTemplate: batchv1.JobTemplateSpec{
							Spec: batchv1.JobSpec{
								Template: cronTemplate,
							},
						},
					},
				},
			}

			for _, workload := range workloads {
				Expect(k8sClient.Create(ctx, workload)).To(Succeed())

				DeferCleanup(func() {
					Expect(k8sClient.Delete(ctx, workload)).To(Succeed())
				})
			}

			controllerReconciler := &ImageWarmupPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var updatedPolicy cachev1alpha1.ImageWarmupPolicy
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updatedPolicy)).
				To(Succeed())

			Expect(updatedPolicy.Status.DiscoveredImageCount).To(Equal(int32(5)))

			Expect(updatedPolicy.Status.CurrentRunID).To(BeEmpty())
			Expect(updatedPolicy.Status.CurrentRunTrigger).To(BeEmpty())
			Expect(updatedPolicy.Status.LastFinishedRun).NotTo(BeNil())

			finishedRun := updatedPolicy.Status.LastFinishedRun

			Expect(finishedRun.ID).To(Equal(
				initialWarmupRunPrefix + string(updatedPolicy.UID),
			))
			Expect(finishedRun.Trigger).To(Equal(
				cachev1alpha1.ImageWarmupRunTriggerInitial,
			))
			Expect(finishedRun.TargetCount).To(Equal(int32(0)))
			Expect(finishedRun.SucceededCount).To(Equal(int32(0)))
			Expect(finishedRun.FailedCount).To(Equal(int32(0)))
		})

		It("should set the image count to zero when no namespaces match", func() {
			var policy cachev1alpha1.ImageWarmupPolicy

			Expect(k8sClient.Get(ctx, typeNamespacedName, &policy)).
				To(Succeed())

			policy.Spec.NamespaceSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					"warmer-test": "never-match",
				},
			}

			Expect(k8sClient.Update(ctx, &policy)).To(Succeed())

			// Fetch again so we have the latest resource version.
			Expect(k8sClient.Get(ctx, typeNamespacedName, &policy)).
				To(Succeed())

			// Simulate a count saved by an earlier discovery.
			policy.Status.DiscoveredImageCount = 9
			Expect(k8sClient.Status().Update(ctx, &policy)).To(Succeed())

			controllerReconciler := &ImageWarmupPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var updatedPolicy cachev1alpha1.ImageWarmupPolicy
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updatedPolicy)).
				To(Succeed())

			Expect(updatedPolicy.Status.DiscoveredImageCount).To(Equal(int32(0)))
		})

		It("should apply namespace selector, list, and skip list", func() {
			const (
				selectorKey       = "scan"
				enabledValue      = "enabled"
				selectedNamespace = "filter-selected"
				skippedNamespace  = "filter-skipped"
				unlistedNamespace = "filter-unlisted"
				wrongNamespace    = "filter-wrong-label"
			)

			createFixture := func(
				namespaceName string,
				selectorValue string,
				image string,
			) {
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: namespaceName,
						Labels: map[string]string{
							selectorKey: selectorValue,
						},
					},
				}

				Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
				DeferCleanup(func() {
					Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
				})

				workloadLabels := map[string]string{
					"app": namespaceName,
				}

				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "filter-test",
						Namespace: namespaceName,
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: workloadLabels,
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: workloadLabels,
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  testApplicationName,
										Image: image,
									},
								},
							},
						},
					},
				}

				Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
				DeferCleanup(func() {
					Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
				})
			}

			createFixture(
				selectedNamespace,
				enabledValue,
				"example/selected:v1",
			)
			createFixture(
				skippedNamespace,
				enabledValue,
				"example/skipped:v1",
			)
			createFixture(
				unlistedNamespace,
				enabledValue,
				"example/unlisted:v1",
			)
			createFixture(
				wrongNamespace,
				"disabled",
				"example/wrong-label:v1",
			)

			createDefaultServiceAccount(selectedNamespace)

			var policy cachev1alpha1.ImageWarmupPolicy
			Expect(k8sClient.Get(ctx, typeNamespacedName, &policy)).
				To(Succeed())

			policy.Spec.NamespaceSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: enabledValue,
				},
			}
			policy.Spec.NamespaceList = []string{
				selectedNamespace,
				skippedNamespace,
				wrongNamespace,
			}
			policy.Spec.NamespaceSkipList = []string{
				skippedNamespace,
			}

			Expect(k8sClient.Update(ctx, &policy)).To(Succeed())

			controllerReconciler := &ImageWarmupPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			var updatedPolicy cachev1alpha1.ImageWarmupPolicy
			Expect(k8sClient.Get(ctx, typeNamespacedName, &updatedPolicy)).
				To(Succeed())

			Expect(updatedPolicy.Status.DiscoveredImageCount).To(Equal(int32(1)))
		})

		It("should apply workload include and skip selectors", func() {
			const (
				warmingLabelKey         = "warming"
				warmingEnabledValue     = "enabled"
				warmingSkipLabelKey     = "warming-skip"
				warmingSkipValue        = "true"
				workloadPodLabelKey     = "workload-app"
				workloadContainerName   = "workload-test-container"
				workloadTargetNamespace = defaultObjectName
			)

			createDeployment := func(
				name string,
				image string,
				metadataLabels map[string]string,
			) {
				podLabels := map[string]string{
					workloadPodLabelKey: name,
				}

				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: workloadTargetNamespace,
						Labels:    metadataLabels,
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: podLabels,
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: podLabels,
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  workloadContainerName,
										Image: image,
									},
								},
							},
						},
					},
				}

				Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

				DeferCleanup(func() {
					Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
				})
			}

			createDefaultServiceAccount(workloadTargetNamespace)

			// Matches the inclusion selector and is not skipped.
			createDeployment(
				"workload-included",
				"example/included:v1",
				map[string]string{
					warmingLabelKey: warmingEnabledValue,
				},
			)

			// Does not match the inclusion selector.
			createDeployment(
				"workload-nonmatching",
				"example/nonmatching:v1",
				map[string]string{
					warmingLabelKey: "disabled",
				},
			)

			// Matches both selectors, so the skip selector wins.
			createDeployment(
				"workload-skipped",
				"example/skipped:v1",
				map[string]string{
					warmingLabelKey:     warmingEnabledValue,
					warmingSkipLabelKey: warmingSkipValue,
				},
			)

			var policy cachev1alpha1.ImageWarmupPolicy

			Expect(k8sClient.Get(ctx, typeNamespacedName, &policy)).
				To(Succeed())

			policy.Spec.WorkloadSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					warmingLabelKey: warmingEnabledValue,
				},
			}

			policy.Spec.WorkloadSkipSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					warmingSkipLabelKey: warmingSkipValue,
				},
			}

			Expect(k8sClient.Update(ctx, &policy)).To(Succeed())

			controllerReconciler := &ImageWarmupPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(
				ctx,
				reconcile.Request{
					NamespacedName: typeNamespacedName,
				},
			)
			Expect(err).NotTo(HaveOccurred())

			var updatedPolicy cachev1alpha1.ImageWarmupPolicy

			Expect(k8sClient.Get(
				ctx,
				typeNamespacedName,
				&updatedPolicy,
			)).To(Succeed())

			Expect(updatedPolicy.Status.DiscoveredImageCount).
				To(Equal(int32(1)))
		})
	})
})
