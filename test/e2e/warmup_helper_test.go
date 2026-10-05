//go:build e2e

package e2e

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachev1alpha1 "github.com/Denzil-Briffa/kube-image-warmer/api/v1alpha1"
	"github.com/Denzil-Briffa/kube-image-warmer/test/utils"
)

const helperTestNamespace = "warmup-helper-e2e"
const shellLessImage = "example.com/warmup-scratch:v0.0.1"

func verifyShellIndependentWarming() {
	By("selecting a schedulable Linux node")
	output, err := utils.Run(exec.Command("kubectl", "get", "nodes",
		"-l", "kubernetes.io/os=linux", "-o", "json"))
	Expect(err).NotTo(HaveOccurred())
	var nodes corev1.NodeList
	Expect(json.Unmarshal([]byte(output), &nodes)).To(Succeed())
	nodeName := ""
	for _, node := range nodes.Items {
		if !node.Spec.Unschedulable && len(node.Spec.Taints) == 0 {
			nodeName = node.Name
			break
		}
	}
	Expect(nodeName).NotTo(BeEmpty(), "isolated kind must contain an untainted Linux node")

	By("creating an isolated restricted namespace")
	applyHelperFixture(&corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   helperTestNamespace,
			Labels: map[string]string{"pod-security.kubernetes.io/enforce": "restricted"},
		},
	})
	DeferCleanup(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace",
			helperTestNamespace, "--ignore-not-found", "--wait=false"))
	})
	zero := int32(0)
	applyHelperFixture(&appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "helper-sources", Namespace: helperTestNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &zero,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"fixture": "helper"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"fixture": "helper"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "shell-image", Image: "busybox:1.36.1"},
					// Its Dockerfile ENTRYPOINT deliberately fails unless overridden.
					{Name: "scratch-image", Image: shellLessImage},
				}},
			},
		},
	})
	policy := &cachev1alpha1.ImageWarmupPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cachev1alpha1.GroupVersion.String(), Kind: "ImageWarmupPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{Name: helperTestNamespace},
		Spec: cachev1alpha1.ImageWarmupPolicySpec{
			NamespaceList: []string{helperTestNamespace},
			NodeSelector: metav1.LabelSelector{MatchLabels: map[string]string{
				"kubernetes.io/hostname": nodeName,
			}},
		},
	}
	DeferCleanup(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "imagewarmuppolicy",
			helperTestNamespace, "--ignore-not-found", "--wait=false"))
	})
	applyHelperFixture(policy)

	By("waiting for both warming targets to complete")
	Eventually(func(g Gomega) {
		result, getErr := utils.Run(exec.Command("kubectl", "get",
			"imagewarmuppolicy", helperTestNamespace, "-o", "json"))
		g.Expect(getErr).NotTo(HaveOccurred())
		var current cachev1alpha1.ImageWarmupPolicy
		g.Expect(json.Unmarshal([]byte(result), &current)).To(Succeed())
		g.Expect(current.Status.LastFinishedRun).NotTo(BeNil())
		g.Expect(current.Status.LastFinishedRun.TargetCount).To(Equal(int32(2)))
		g.Expect(current.Status.LastFinishedRun.SucceededCount).To(Equal(int32(2)))
		g.Expect(current.Status.LastFinishedRun.FailedCount).To(BeZero())
	}, 5*time.Minute, 2*time.Second).Should(Succeed())

	By("verifying that the mounted helper completed in both target images")
	output, err = utils.Run(exec.Command("kubectl", "get", "pods",
		"-n", helperTestNamespace, "-l", "app.kubernetes.io/managed-by=kube-image-warmer", "-o", "json"))
	Expect(err).NotTo(HaveOccurred())
	var pods corev1.PodList
	Expect(json.Unmarshal([]byte(output), &pods)).To(Succeed())
	Expect(pods.Items).To(HaveLen(2))
	for _, pod := range pods.Items {
		Expect(pod.Spec.NodeName).To(Equal(nodeName))
		Expect(pod.Spec.InitContainers).To(HaveLen(1))
		Expect(pod.Spec.InitContainers[0].Image).To(Equal(managerImage))
		Expect(pod.Spec.Containers).To(HaveLen(1))
		Expect(pod.Spec.Containers[0].Command).To(Equal([]string{"/image-warmer/helper"}))
		Expect(pod.Spec.Containers[0].Args).To(Equal([]string{"--complete"}))
		Expect(pod.Status.Phase).To(Equal(corev1.PodSucceeded))
		Expect(pod.Status.ContainerStatuses).To(HaveLen(1))
		Expect(pod.Status.ContainerStatuses[0].State.Terminated).NotTo(BeNil())
		Expect(pod.Status.ContainerStatuses[0].State.Terminated.ExitCode).To(BeZero())
	}
}

func applyHelperFixture(object any) {
	payload, err := json.Marshal(object)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(payload))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
