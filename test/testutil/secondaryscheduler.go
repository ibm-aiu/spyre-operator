/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ForcePullLatestSchedulerImage creates a Pod in "default" namespace to force-pull scheduler image on all of
// control-plane nodes.
// note: This function may leave puller Pods, and therefore caller must use CleanupForcePullPods() in DeferCleanup().
func ForcePullLatestSchedulerImage(ctx context.Context, k8sClientset *kubernetes.Clientset, schedImage string) {
	nodes, err := k8sClientset.CoreV1().Nodes().List(ctx, v1.ListOptions{
		LabelSelector: "node-role.kubernetes.io/worker",
	})
	Expect(err).To(BeNil())

	for _, node := range nodes.Items {
		if !IsNodeReady(node) {
			_, err = fmt.Fprintf(GinkgoWriter, "Skip pulling secondary scheduler image on %s: NotReady\n", node.Name)
			Expect(err).To(BeNil())
			continue
		}
		pod := &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: "force-sched-image-puller-",
				Namespace:    "default",
				Labels: map[string]string{
					"app": schedForcePullPodLabel,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "sched",
						Image:           schedImage,
						ImagePullPolicy: corev1.PullAlways,
						Command:         []string{"/bin/echo", "hello"},
					},
				},
				NodeSelector: map[string]string{
					"kubernetes.io/hostname": node.Name,
				},
				RestartPolicy: corev1.RestartPolicyNever,
			},
		}
		_, err := k8sClientset.CoreV1().Pods("default").Create(ctx, pod, v1.CreateOptions{})
		Expect(err).To(BeNil())
	}
	DeferCleanup(CleanupForcePullPods, ctx, k8sClientset)

	Eventually(func(g Gomega) {
		pods, err := k8sClientset.CoreV1().Pods("default").List(ctx, v1.ListOptions{
			LabelSelector: "app=" + schedForcePullPodLabel,
		})

		g.Expect(err).To(BeNil())
		// not equal but grater than - because old Pod may remain.
		g.Expect(len(pods.Items)).Should(BeNumerically(">=", len(nodes.Items)))

		for _, pod := range pods.Items {
			message := getPodMessage(pod)
			g.Expect(pod.Status.Phase).Should(Equal(corev1.PodSucceeded), message)
		}
	}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
}

func CleanupForcePullPods(ctx context.Context, k8sClientset *kubernetes.Clientset) {
	By("Cleaning up image-puller pods")
	Eventually(func(g Gomega) {
		pods, err := k8sClientset.CoreV1().Pods("default").List(ctx, v1.ListOptions{
			LabelSelector: "app=" + schedForcePullPodLabel,
		})
		g.Expect(err).To(BeNil())

		for _, pod := range pods.Items {
			err = k8sClientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
			g.Expect(err).To(BeNil())
		}
		pods, err = k8sClientset.CoreV1().Pods("default").List(ctx, v1.ListOptions{
			LabelSelector: "app=" + schedForcePullPodLabel,
		})
		g.Expect(err).To(BeNil())
		g.Expect(len(pods.Items)).Should(Equal(0))
	}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
}
