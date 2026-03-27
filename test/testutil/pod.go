/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (tc *TestCase) TestSinglePod(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, expectedAllocatedDevice []string, expectedPodPhase v1.PodPhase) {
	// create expectedAllocation
	expectedAllocation := make(map[string]bool)
	for _, dev := range expectedAllocatedDevice {
		expectedAllocation[dev] = true
	}
	By("deploying pod")
	testPods := BuildPods(1, tc.Prefix, tc.TestNamespace, tc.ResourceName, tc.Quantity, tc.NodeName, true)
	deployedPods := deployPods(ctx, k8sClientset, testPods, map[v1.PodPhase]int{expectedPodPhase: 1})
	Expect(deployedPods).To(HaveLen(1))

	By(fmt.Sprintf("checking Spyre node state, expect %v", expectedAllocatedDevice))
	if len(expectedAllocatedDevice) > 0 {
		checkSpyreNodeState(ctx, spyreV2Client, expectedAllocation, tc.NodeName)
		By("checking senlib config")
		checkPodLog(ctx, k8sClientset, tc.Prefix, tc.TestNamespace, expectedAllocation)
	}

	By("deleting pod")
	deletePods(ctx, k8sClientset, testPods)

	By("checking Spyre node state after pod deletion")
	checkSpyreNodeState(ctx, spyreV2Client, map[string]bool{}, tc.NodeName)
}

func (tc *TestCase) TestNSpyrePfPod(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, n int, expectedPodPhase map[v1.PodPhase]int) {
	// create expectedAllocation
	expectedAdded := expectedPodPhase[v1.PodRunning] * int(tc.Quantity)
	By("deploying pod")
	testPods := BuildPods(n, tc.Prefix, tc.TestNamespace, tc.ResourceName, tc.Quantity, tc.NodeName, true)
	deployedPods := deployPods(ctx, k8sClientset, testPods, expectedPodPhase)
	Expect(deployedPods).To(HaveLen(n))

	By("checking Spyre node state on node")
	allocationList := checkSpyreNodeStateWithN(ctx, spyreV2Client, tc.NodeName, expectedAdded)

	By("checking senlib config")
	checkPodsLogFromAllocationList(ctx, k8sClientset, allocationList)

	By("deleting pods")
	deletePods(ctx, k8sClientset, testPods)

	By("checking Spyre node state after pod deletion")
	checkSpyreNodeState(ctx, spyreV2Client, map[string]bool{}, tc.NodeName)
}

func (tc *MixedResourceTestCase) TestDeployPods(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, expectedPodPhase map[v1.PodPhase]int) []*v1.Pod {
	By("building pods")
	testPods := []*v1.Pod{}
	requestIndex := 0
	for request, n := range tc.Requests {
		prefix := fmt.Sprintf("%s%d", tc.Prefix, requestIndex)
		pods := BuildPods(n, prefix, tc.TestNamespace, request.ResourceName, request.Quantity, tc.NodeName, true)
		testPods = append(testPods, pods...)
		requestIndex += 1
	}
	for i := range testPods {
		testPods[i].Spec.RestartPolicy = tc.RestartPolicy
	}
	By("deploying pods")
	deployedPods := deployPods(ctx, k8sClientset, testPods, expectedPodPhase)
	return deployedPods
}

func (tc *MixedResourceTestCase) DeleteRunningPodAndExpectPendingRunIfExists(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, testPods []*v1.Pod, expectedPodPhase map[v1.PodPhase]int) ([]*v1.Pod, map[v1.PodPhase]int) {
	remainingPods := []*v1.Pod{}
	deleted := false
	for _, testPod := range testPods {
		pod, err := k8sClientset.CoreV1().Pods(testPod.Namespace).Get(ctx, testPod.Name, metav1.GetOptions{})
		if !deleted && err == nil && pod.Status.Phase == v1.PodRunning {
			By("deleting running pod")
			err = k8sClientset.CoreV1().Pods(testPod.Namespace).Delete(ctx, testPod.Name, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
			deleted = true
		} else {
			remainingPods = append(remainingPods, testPod)
		}
	}
	Expect(deleted).To(BeTrue())

	_, found := expectedPodPhase[v1.PodRunning]
	Expect(found).To(BeTrue())
	if count, found := expectedPodPhase[v1.PodPending]; found && count > 0 {
		expectedPodPhase[v1.PodPending] -= 1
		if expectedPodPhase[v1.PodPending] == 0 {
			delete(expectedPodPhase, v1.PodPending)
		}
	} else {
		expectedPodPhase[v1.PodRunning] -= 1
		if expectedPodPhase[v1.PodRunning] == 0 {
			delete(expectedPodPhase, v1.PodRunning)
		}
	}
	By(fmt.Sprintf("Checking pod phases %v", expectedPodPhase))
	checkPodPhases(ctx, k8sClientset, remainingPods, expectedPodPhase)
	return remainingPods, expectedPodPhase
}

func BuildPods(n int, prefix string, namespace string, resourceName string, quantity int64, nodeName string, schedulerEnabled bool) []*v1.Pod {
	pods := []*v1.Pod{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s-%d", prefix, i)
		if n == 1 {
			name = prefix
		}
		pod := BuildPod(name, namespace, resourceName, quantity, nodeName, schedulerEnabled)
		pods = append(pods, pod)
	}
	Expect(len(pods)).To(Equal(n))
	return pods
}

func deployPods(ctx context.Context, k8sClientset *kubernetes.Clientset, pods []*v1.Pod, expectedPodPhase map[v1.PodPhase]int) []*v1.Pod {
	for _, pod := range pods {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			g.Expect(err).To(BeNil())
		}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
	totalNum := 0
	for _, num := range expectedPodPhase {
		totalNum += num
	}
	Expect(totalNum).To(Equal(len(pods)))
	By("checking pod phases")
	checkPodPhases(ctx, k8sClientset, pods, expectedPodPhase)

	deployedPods := make([]*v1.Pod, len(pods))
	for i, pod := range pods {
		deployedPod, err := k8sClientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		Expect(err).To(BeNil())
		Expect(deployedPod).ToNot(BeNil())
		deployedPods[i] = deployedPod
	}
	return deployedPods
}

func deletePods(ctx context.Context, k8sClientset *kubernetes.Clientset, pods []*v1.Pod) {
	for _, pod := range pods {
		Eventually(func(g Gomega) {
			err := k8sClientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
			if err == nil {
				_, err = k8sClientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
			}
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
}

func GetSenlibConfig(ctx context.Context, k8sClientset *kubernetes.Clientset, name, namespace string) controllers.SenlibConfig {
	req := k8sClientset.CoreV1().Pods(namespace).GetLogs(name, &v1.PodLogOptions{Container: "app"})
	podLog, err := req.Stream(ctx)
	Expect(err).To(BeNil())

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLog)
	Expect(err).To(BeNil())

	var senlibConfig controllers.SenlibConfig
	err = json.Unmarshal(buf.Bytes(), &senlibConfig)
	Expect(err).To(BeNil())
	return senlibConfig
}

func checkPodLog(ctx context.Context, k8sClientset *kubernetes.Clientset, name, namespace string, expectedAllocation map[string]bool) {
	senlibConfig := GetSenlibConfig(ctx, k8sClientset, name, namespace)
	totalAllocation := 0
	for _, allocated := range expectedAllocation {
		if allocated {
			totalAllocation += 1
		}
	}
	Expect(len(senlibConfig.General.PciAddresses)).To(Equal(totalAllocation))
	for _, dev := range senlibConfig.General.PciAddresses {
		_, found := expectedAllocation[dev]
		Expect(found).To(BeTrue())
	}
	Expect(senlibConfig.General.Doom).To(BeFalse(), "Doom Mode should be false")
}

func checkPodsLogFromAllocationList(ctx context.Context, k8sClientset *kubernetes.Clientset, allocationList []spyrev1alpha1.Allocation) {
	for _, allocation := range allocationList {
		req := k8sClientset.CoreV1().Pods(allocation.Pod.Namespace).GetLogs(allocation.Pod.Name, &v1.PodLogOptions{Container: "app"})
		podLog, err := req.Stream(ctx)
		Expect(err).To(BeNil())

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, podLog)
		Expect(err).To(BeNil())

		var senlibConfig controllers.SenlibConfig
		err = json.Unmarshal(buf.Bytes(), &senlibConfig)
		Expect(err).To(BeNil())
		Expect(senlibConfig.General.PciAddresses).To(Equal(allocation.DeviceList))
	}
}

func GetPodsWithLabels(ctx context.Context, k8sClientset *kubernetes.Clientset, g Gomega, namespace, label string, nodeName string) []v1.Pod {
	listOptions := metav1.ListOptions{
		LabelSelector: label,
	}
	// add field selector of spec.nodeName to the list option only the target node if specified
	if nodeName != "" {
		listOptions.FieldSelector = fields.Set{"spec.nodeName": nodeName}.AsSelector().String()
	}
	pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, listOptions)
	g.Expect(err).To(BeNil())
	return pods.Items
}

func EnsureDeletedPodWithLabels(ctx context.Context, k8sClientset *kubernetes.Clientset, g Gomega, namespace, label string) {
	listOptions := metav1.ListOptions{
		LabelSelector: label,
	}
	pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, listOptions)
	g.Expect(err).To(BeNil())
	g.Expect(len(pods.Items)).To(Equal(0))
}

func BuildPod(name, namespace string, resourceName string, quantity int64, nodeName string, schedulerEnabled bool) *v1.Pod {
	arg0 := "cat /etc/aiu/senlib_config.json;tail -f /dev/null"
	return buildPod(name, namespace, resourceName, quantity, nodeName, arg0, schedulerEnabled)
}

func BuildMockUserPod(exporterPort int, testConfig TestConfig, name, namespace string, numOfSpyre int64, nodeName string) *v1.Pod {
	var True bool = true
	copyData := "./user-copy.sh"
	sleepAndRunGetMetric := fmt.Sprintf("sleep 20; curl http://%s.%s:%d; sleep 1000", metricsExporterName, OperatorNamespace, exporterPort)
	arg0 := fmt.Sprintf("%s;%s", copyData, sleepAndRunGetMetric)
	pod := buildPod(name, namespace, "ibm.com/spyre_pf", numOfSpyre, nodeName, arg0, true)
	pod.Spec.RestartPolicy = v1.RestartPolicyOnFailure
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged: &True,
	}
	pod.Spec.Containers[0].Image = testConfig.ExporterMockUser.GetImage()
	pod.Spec.Containers[0].ImagePullPolicy = v1.PullAlways
	pod.Spec.DNSPolicy = v1.DNSClusterFirst
	return pod
}

func BuildPodListFiles(name, namespace, resourceName string, paths []string, nodeName string, schedulerEnabled bool) *v1.Pod {
	args := []string{}
	for _, path := range paths {
		args = append(args, fmt.Sprintf("while [ ! -f %s ]; do echo 'wait for %s';sleep 1; done; cat %s", path, path, path))
	}
	arg0 := strings.Join(args, ";") + "; sleep 1000"
	pod := buildPod(name, namespace, resourceName, 1, nodeName, arg0, schedulerEnabled)
	return pod
}

func buildPod(name, namespace string, resourceName string, quantity int64, nodeName string, arg0 string, schedulerEnabled bool) *v1.Pod {
	resourceRequest := make(v1.ResourceList)
	resourceLimit := make(v1.ResourceList)
	resourceRequest[v1.ResourceName(resourceName)] = *resource.NewQuantity(quantity, resource.DecimalSI)
	resourceLimit[v1.ResourceName(resourceName)] = *resource.NewQuantity(quantity, resource.DecimalSI)
	annotations := make(map[string]string)
	monitorVolume := v1.Volume{
		Name: monitorVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	zeroGracePeriod := int64(0)
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: v1.PodSpec{
			NodeSelector: map[string]string{"kubernetes.io/hostname": nodeName},
			Containers: []v1.Container{
				{
					Name:            "app",
					Image:           containerTestImage,
					ImagePullPolicy: v1.PullIfNotPresent,
					Command: []string{
						"/bin/bash", "-c",
					},
					Args: []string{arg0},
					Resources: v1.ResourceRequirements{
						Requests: resourceRequest,
						Limits:   resourceLimit,
					},
				},
			},
			Volumes:                       []v1.Volume{monitorVolume},
			TerminationGracePeriodSeconds: &zeroGracePeriod,
		},
	}
	if schedulerEnabled {
		pod.Spec.SchedulerName = spyreconst.SpyreSchedulerName
	}
	return pod
}

func checkPodPhases(ctx context.Context, k8sClientset *kubernetes.Clientset, pods []*v1.Pod, expectedStateNum map[v1.PodPhase]int) {
	if len(expectedStateNum) > 0 {
		namespace := pods[0].Namespace
		By(fmt.Sprintf("Waiting for %v", expectedStateNum))
		Eventually(func(g Gomega) {
			count := make(map[v1.PodPhase]int)
			for _, pod := range pods {
				pod, err := k8sClientset.CoreV1().Pods(namespace).Get(ctx, pod.Name, metav1.GetOptions{})
				g.Expect(err).To(BeNil())
				if _, found := count[pod.Status.Phase]; !found {
					count[pod.Status.Phase] = 0
				}
				count[pod.Status.Phase] += 1
				message := getPodMessage(*pod)
				By(fmt.Sprintf("Getting pod count: %s - %s %s", pod.Name, pod.Status.Phase, message))
				if len(pod.OwnerReferences) > 0 {
					g.Expect(pod.Status.Phase).NotTo(Equal(v1.PodFailed))
				} else if pod.Status.Phase == v1.PodFailed {
					By(fmt.Sprintf("recreating failed pod %s", pod.Name))
					DeletePod(ctx, k8sClientset, pod)
					recreatePod(ctx, k8sClientset, pod)
				}
			}
			for phase, num := range expectedStateNum {
				if num == 0 {
					continue
				}
				countNum, found := count[phase]
				g.Expect(found).To(BeTrue())
				g.Expect(countNum).To(Equal(num))
			}
		}).WithTimeout(7 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
}

func recreatePod(ctx context.Context, k8sClientset *kubernetes.Clientset, pod *v1.Pod) {
	pod.ObjectMeta = metav1.ObjectMeta{
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}
	annotations := make(map[string]string)
	for key := range monitoringDisableAnnotations {
		if val, found := pod.Annotations[key]; found {
			annotations[key] = val
		}
	}
	pod.ObjectMeta.Annotations = annotations
	pod.Spec.NodeName = ""
	_, err := k8sClientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	Expect(err).To(BeNil())
}

func WaitForPodRunning(ctx context.Context, k8sClientset *kubernetes.Clientset, pod *v1.Pod) {
	checkPodPhases(ctx, k8sClientset, []*v1.Pod{pod}, map[v1.PodPhase]int{v1.PodRunning: 1})
}

func printMessageIfPodNotRunning(pod v1.Pod) {
	if pod.Status.Phase != v1.PodRunning {
		if message := getPodMessage(pod); message != "" {
			By(fmt.Sprintf("%s is %s: %s", pod.Name, pod.Status.Phase, message))
		}
	}
}

func getPodMessage(pod v1.Pod) string {
	message := ""
	if len(pod.Status.ContainerStatuses) > 0 {
		if pod.Status.ContainerStatuses[0].State.Waiting != nil {
			message = pod.Status.ContainerStatuses[0].State.Waiting.Message
		}
	}
	return message
}

func DeletePod(ctx context.Context, k8sClientset *kubernetes.Clientset, pod *v1.Pod) {
	err := k8sClientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
	By(fmt.Sprintf("deleting pod %s/%s: %v", pod.Name, pod.Namespace, err))
	if err != nil {
		Expect(errors.IsNotFound(err)).To(BeTrue())
		return
	}
	Eventually(func(g Gomega) {
		_, err := k8sClientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		g.Expect(errors.IsNotFound(err)).To(BeTrue())
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func DeletePodsWithLabels(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, label string, ensureDeletion bool) error {
	listOptions := metav1.ListOptions{
		LabelSelector: label,
	}
	err := k8sClientset.CoreV1().Pods(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, listOptions)
	if err != nil {
		return fmt.Errorf("failed to delete Pods with label \"%s\" in namespace %s: %w", label, namespace, err)
	}
	if ensureDeletion {
		Eventually(func(g Gomega) {
			pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, listOptions)
			g.Expect(err).To(BeNil())
			g.Expect(pods.Items).Should(HaveLen(0))
		}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	}
	return nil
}

func WaitForKeywordInPodLog(ctx context.Context, k8sClientset *kubernetes.Clientset, pod *v1.Pod, container string,
	getMetricKeyFuncs []func(namespace, name, nodeName string) string, nodeName string) {
	for _, getMetricFunc := range getMetricKeyFuncs {
		keyword := getMetricFunc(pod.Namespace, pod.Name, nodeName)
		By(fmt.Sprintf("Searching for keyword %s", keyword))
	}
	Eventually(func(g Gomega) {
		bufStr, err := GetPodLog(ctx, k8sClientset, container, *pod)
		g.Expect(err).To(BeNil())
		for _, getMetricFunc := range getMetricKeyFuncs {
			keyword := getMetricFunc(pod.Namespace, pod.Name, nodeName)
			regexp, err := regexp.Compile(keyword)
			g.Expect(err).To(BeNil())
			match := regexp.FindString(bufStr)
			g.Expect(match).NotTo(BeEmpty())
		}
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func WaitUntilNoPod(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, namespace string, nodeName string) {
	By("waiting until no pod and empty Spyre node state")
	Eventually(func(g Gomega) {
		podList, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		Expect(err).To(BeNil())
		g.Expect(len(podList.Items)).To(Equal(0))
		checkSpyreNodeState(ctx, spyreV2Client, map[string]bool{}, nodeName)
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func GetPodLog(ctx context.Context, k8sClientset *kubernetes.Clientset, container string, pod v1.Pod) (string, error) {
	req := k8sClientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{Container: container})
	podLog, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod '%s/%s' log: %w", pod.Namespace, pod.Name, err)
	}
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLog)
	if err != nil {
		return "", fmt.Errorf("failed to copy pod '%s/%s' log: %w", pod.Namespace, pod.Name, err)
	}
	return buf.String(), nil
}

func WaitForDevicePluginEnvUpdate(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, nodeName string, expectedEnv map[string]string) {
	waitForEnvUpdate(ctx, k8sClientset, OperatorNamespace, nodeName, devicePluginLabel, expectedEnv)
}

func WaitForMetricsExporterEnvUpdate(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, nodeName string, expectedEnv map[string]string) {
	waitForEnvUpdate(ctx, k8sClientset, OperatorNamespace, nodeName, metricsExporterLabel, expectedEnv)
}

func waitForEnvUpdate(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, nodeName, label string, expectedEnv map[string]string) {
	Eventually(func(g Gomega) { //nolint:dupl
		pods := GetPodsWithLabels(ctx, k8sClientset, g, namespace, label, nodeName)
		g.Expect(pods).To(HaveLen(1))
		pod := pods[0]
		envs := pod.Spec.Containers[0].Env
		existingEnv := make(map[string]string)
		for _, env := range envs {
			existingEnv[env.Name] = env.Value
		}
		for k, v := range expectedEnv {
			value, found := existingEnv[k]
			g.Expect(found).To(BeTrue())
			g.Expect(value).To(BeEquivalentTo(v))
		}
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func CheckPodListFilesLog(ctx context.Context, k8sClient *kubernetes.Clientset, pod *v1.Pod, expectedTopologyFile bool) {
	Eventually(func(g Gomega) { //nolint:dupl
		log, err := GetPodLog(ctx, k8sClient, pod.Spec.Containers[0].Name, *pod)
		g.Expect(err).To(BeNil())
		found := strings.Contains(log, pod.Name)
		g.Expect(found).To(BeTrue())
		if !expectedTopologyFile {
			// topology keyword: num_devices
			// this keyword must not present in the other testing files.
			found := strings.Contains(log, "num_devices")
			Expect(found).To(BeFalse())
		}
	}).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func GetDevicePluginPod(ctx context.Context, k8sClientset *kubernetes.Clientset, g Gomega, namespace, nodeName string) corev1.Pod {
	pods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, devicePluginLabel, nodeName)
	Expect(len(pods)).To(Equal(1))
	return pods[0]
}

func ExecCommand(ctx context.Context, config *rest.Config, clientset *kubernetes.Clientset, namespace, podName string, command []string) (string, error) {
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec").VersionedParams(
		&v1.PodExecOptions{
			Command: command,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return "", fmt.Errorf("failed to execute command: %s, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
