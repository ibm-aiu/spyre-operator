/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	spyrev2 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// construct list of key:vale of pf runner pod and node name
func RunnerWorkerMap(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, deviceType string, nodeFilter []string) map[string]string {
	wpfmap := make(map[string]string)
	nodes := GetWorkerNodeNames(ctx, k8sClientset)
	for _, node := range nodes {
		if slices.Contains(nodeFilter, node) {
			ns, err := GetSpyreNodeState(ctx, spyreV2Client, node)
			Expect(err).To(BeNil())
			for _, spyreIf := range ns.Spec.SpyreInterfaces {
				pciResName := strings.ReplaceAll(spyreIf.PciAddress, ":", "-")
				pfresName := strings.Join([]string{deviceType, pciResName, node}, "-")
				wpfmap[pfresName] = node
			}
		}
	}
	return wpfmap
}
func VerifyRunner(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string, runnerWrkmap map[string]string) {
	By("runner pods are created")
	Eventually(func(g Gomega) {
		for podname, nodename := range runnerWrkmap {
			pod, err := k8sClientset.CoreV1().Pods(namespace).Get(ctx, podname, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
			g.Expect(pod.Spec.NodeName).To(BeEquivalentTo(nodename))
		}
	}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

	By("runner pods must be running")
	Eventually(func(g Gomega) {
		for podname := range runnerWrkmap {
			pod, err := k8sClientset.CoreV1().Pods(namespace).Get(ctx, podname, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
			g.Expect(pod.Status.Phase).To(BeEquivalentTo("Running"))
		}
	}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
}

func IsAmd64Arch(ctx context.Context, k8sClientset *kubernetes.Clientset) (bool, error) {
	nodes, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("fail to list Nodes: %w", err)
	}
	for _, node := range nodes.Items {
		arch := node.Status.NodeInfo.Architecture
		if arch == "amd64" {
			return true, nil
		}
	}
	return false, nil
}

func IsPpc64LeArch(ctx context.Context, k8sClientset *kubernetes.Clientset) (bool, error) {
	nodes, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("fail to list Nodes: %w", err)
	}
	for _, node := range nodes.Items {
		arch := node.Status.NodeInfo.Architecture
		if arch == "ppc64le" {
			return true, nil
		}
	}
	return false, nil
}

func EnabledCardmgmtForWorkers(ctx context.Context, clusterPolicy *spyrev2.SpyreClusterPolicy, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, nodeFilter string) {
	nodeList, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	Expect(err).To(BeNil())
	By(fmt.Sprintf("Enabled cardmgmt for %s", nodeFilter))
	clusterPolicy.Spec.CardManagement.Enabled = false
	UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeList.Items), spyrev2.Ready)
	err = spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
	Expect(err).To(BeNil())
	*clusterPolicy.Spec.CardManagement.Config.SpyreFilter = nodeFilter
	clusterPolicy.Spec.CardManagement.Enabled = true
	UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeList.Items), spyrev2.Ready)
}

var (
	nospyre = PodTemplateData{
		Name:  "nospyre",
		Image: Ubi9MicroTestImage,
	}
	pf1 = PodTemplateData{
		Name:             "pf1",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_pf",
		ResourceQuantity: "1",
		SidecarName:      "sidecar",
	}
	vfTier02 = PodTemplateData{
		Name:             "vf-tier0-2",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf_tier0",
		ResourceQuantity: "2",
		SidecarName:      "sidecar",
	}
	vfTier14 = PodTemplateData{
		Name:             "vf-tier1-4",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf_tier1",
		ResourceQuantity: "4",
		SidecarName:      "sidecar",
	}
	vfTier24 = PodTemplateData{
		Name:             "vf-tier2-4",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf_tier2",
		ResourceQuantity: "4",
		SidecarName:      "sidecar",
	}
	vf1 = PodTemplateData{
		Name:             "vf1",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf",
		ResourceQuantity: "1",
		SidecarName:      "sidecar",
	}
	vf6 = PodTemplateData{
		Name:             "vf6",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf",
		ResourceQuantity: "6",
		SidecarName:      "sidecar",
	}
)

// Cardmgmt stability test
// Enabled on worker 1
var CardmgmtEnableWorker1TestRunning = []PodTemplateData{nospyre, pf1, vfTier02, vfTier14, vfTier24, vf1, vf6}
var CardmgmtEnableWorker1TestPending = []PodTemplateData{}

// Enabled on worker 2
var CardmgmtEnableWorker2TestRunning = []PodTemplateData{nospyre, pf1, vfTier02, vfTier14, vfTier24, vf1, vf6}

// Enabled on all spyre workers
var CardmgmtEnableAllNodesTestRunning = []PodTemplateData{nospyre, vfTier02, vfTier14, vfTier24, vf1, vf6}
var CardmgmtEnableAllNodesTestPending = []PodTemplateData{pf1}

// Disabled
var CardmgmtDisabledTestRunning = []PodTemplateData{nospyre, pf1, vfTier02, vfTier14, vfTier24, vf1, vf6}

// ---------------------------------------------------------------------------
// Mock aiu-cardmgmt-health-api sidecar helpers (pseudo device mode only)
// ---------------------------------------------------------------------------

// DeployMockCardmgmtSidecar creates the ServiceAccount, RBAC, and DaemonSet
// for the mock aiu-cardmgmt-health-api in the given namespace, then blocks
// until all DaemonSet pods are Running.
//
// The mock runs with PSEUDO_DEVICE_MODE=1: GetCardHealth returns "unhealthy"
// for any PCI slot starting with "0000:41" and "healthy" for all others —
// no RPyC worker is needed.  The socket is written to mockCardmgmtHostPath
// which is the same hostPath already mounted by the health-checker DaemonSet,
// so the health-checker's cardmgmt reporter can reach it immediately.
//
// Call TeardownMockCardmgmtSidecar in AfterAll to clean up.
func DeployMockCardmgmtSidecar(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, image string) {
	By("creating mock cardmgmt sidecar ServiceAccount")
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: MockCardmgmtSidecarName, Namespace: namespace},
	}
	_, err := k8sClientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	Expect(err).To(BeNil())

	By("creating mock cardmgmt sidecar Role + RoleBinding for privileged SCC")
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: MockCardmgmtSidecarName + "-scc", Namespace: namespace},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{"security.openshift.io"},
			Resources:     []string{"securitycontextconstraints"},
			ResourceNames: []string{"privileged"},
			Verbs:         []string{"use"},
		}},
	}
	_, err = k8sClientset.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{})
	Expect(err).To(BeNil())

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: MockCardmgmtSidecarName + "-scc", Namespace: namespace},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      MockCardmgmtSidecarName,
			Namespace: namespace,
		}},
	}
	_, err = k8sClientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
	Expect(err).To(BeNil())

	By("creating mock cardmgmt sidecar DaemonSet")
	ds := buildMockCardmgmtDaemonSet(namespace, image)
	_, err = k8sClientset.AppsV1().DaemonSets(namespace).Create(ctx, ds, metav1.CreateOptions{})
	Expect(err).To(BeNil())

	By("waiting for mock cardmgmt sidecar pods to be Running")
	Eventually(func(g Gomega) {
		pods := GetPodsWithLabels(ctx, k8sClientset, g, namespace, MockCardmgmtSidecarLabel, "")
		g.Expect(len(pods)).To(BeNumerically(">=", 1),
			"expected at least one mock cardmgmt sidecar pod to be scheduled")
		for _, pod := range pods {
			g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning),
				"mock sidecar pod %q is not Running (phase=%s)", pod.Name, pod.Status.Phase)
		}
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

// TeardownMockCardmgmtSidecar deletes the DaemonSet, Role, RoleBinding, and
// ServiceAccount created by DeployMockCardmgmtSidecar, waiting for all pods
// to terminate before returning.  Errors are ignored so that AfterAll does
// not fail if cleanup was already done or resources never existed.
func TeardownMockCardmgmtSidecar(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) {
	By("deleting mock cardmgmt sidecar DaemonSet")
	_ = k8sClientset.AppsV1().DaemonSets(namespace).Delete(ctx, MockCardmgmtSidecarName, metav1.DeleteOptions{})
	Eventually(func(g Gomega) {
		pods := GetPodsWithLabelsIgnoreError(ctx, k8sClientset, namespace, MockCardmgmtSidecarLabel)
		g.Expect(pods).To(BeEmpty(), "waiting for mock cardmgmt sidecar pods to terminate")
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("deleting mock cardmgmt sidecar RBAC")
	_ = k8sClientset.RbacV1().RoleBindings(namespace).Delete(ctx, MockCardmgmtSidecarName+"-scc", metav1.DeleteOptions{})
	_ = k8sClientset.RbacV1().Roles(namespace).Delete(ctx, MockCardmgmtSidecarName+"-scc", metav1.DeleteOptions{})
	_ = k8sClientset.CoreV1().ServiceAccounts(namespace).Delete(ctx, MockCardmgmtSidecarName, metav1.DeleteOptions{})
}

// GetPodsWithLabelsIgnoreError is like GetPodsWithLabels but returns an empty
// slice on error instead of failing the spec.  Use in teardown paths where
// it is acceptable for the resource to have already been deleted.
func GetPodsWithLabelsIgnoreError(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, label string) []corev1.Pod {
	pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return nil
	}
	return pods.Items
}

// buildMockCardmgmtDaemonSet constructs the DaemonSet object for the mock
// aiu-cardmgmt-health-api sidecar.  It mirrors the structure of
// hack/aiu-cardmgmt-health-check-api-mock-daemonset.yaml but targets the
// same hostPath that the health-checker DaemonSet already mounts
// (mockCardmgmtHostPath) so the health-checker's cardmgmt reporter can
// reach the socket without any reconfiguration.
func buildMockCardmgmtDaemonSet(namespace, image string) *appsv1.DaemonSet {
	spcT := "spc_t"
	runAsUser := int64(1000)
	allowPrivEsc := false
	readOnlyRootFS := false
	socketPath := mockCardmgmtHostPath + "/" + mockCardmgmtSocketFile

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockCardmgmtSidecarName,
			Namespace: namespace,
			Labels:    map[string]string{"app": MockCardmgmtSidecarName},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": MockCardmgmtSidecarName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": MockCardmgmtSidecarName},
				},
				Spec: corev1.PodSpec{
					NodeSelector:       map[string]string{"node-role.kubernetes.io/worker": ""},
					ServiceAccountName: MockCardmgmtSidecarName,
					InitContainers: []corev1.Container{
						{
							// Ensure the hostPath directory is world-writable so the
							// non-root container can create the socket inside it.
							Name:    "open-socket-dir",
							Image:   image,
							Command: []string{"/bin/sh", "-c", "chmod 0777 /var/run/cardmgmt-health-check-api"},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:      func() *int64 { u := int64(0); return &u }(),
								SELinuxOptions: &corev1.SELinuxOptions{Type: spcT},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "health-check-api-socket", MountPath: "/var/run/cardmgmt-health-check-api"},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "health-api",
							Image:           image,
							ImagePullPolicy: corev1.PullAlways,
							Env: []corev1.EnvVar{
								{Name: "API_GRPC_SOCKET_PATH", Value: socketPath},
								{Name: "PSEUDO_DEVICE_MODE", Value: "1"},
								{Name: "LOG_LEVEL", Value: "INFO"},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             func() *bool { b := true; return &b }(),
								RunAsUser:                &runAsUser,
								AllowPrivilegeEscalation: &allowPrivEsc,
								ReadOnlyRootFilesystem:   &readOnlyRootFS,
								Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
								SELinuxOptions:           &corev1.SELinuxOptions{Type: spcT},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "health-check-api-socket", MountPath: "/var/run/cardmgmt-health-check-api"},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"python", "-c",
											fmt.Sprintf("import os,socket; s=socket.socket(socket.AF_UNIX); s.connect('%s'); s.close()", socketPath),
										},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"python", "-c",
											fmt.Sprintf("import os,socket; s=socket.socket(socket.AF_UNIX); s.connect('%s'); s.close()", socketPath),
										},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "health-check-api-socket",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: mockCardmgmtHostPath,
									Type: func() *corev1.HostPathType {
										t := corev1.HostPathDirectoryOrCreate
										return &t
									}(),
								},
							},
						},
					},
				},
			},
		},
	}
}
