/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/controllers"
	v1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func InstallOperator(ctx context.Context, testConfig TestConfig, k8sClientset *kubernetes.Clientset, dynClient dynamic.Interface) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: OperatorNamespace,
		},
	}
	_, err := k8sClientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		Expect(apiErrors.IsAlreadyExists(err)).To(BeTrue())
	}
	deployCatalogSource(ctx, testConfig, k8sClientset, dynClient)
	deploySubscription(ctx, testConfig, k8sClientset, dynClient)
	waitForOperatorToBeReady(ctx, testConfig, k8sClientset)
}

func UninstallOperator(ctx context.Context, k8sClientset *kubernetes.Clientset, dynClient dynamic.Interface, spyreV2Client client.Client, hasDevice bool, nodeNames []string) {
	var result *multierror.Error

	clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClusterPolicyName,
		},
	}
	By("deleting cluster policy")
	err := spyreV2Client.Delete(ctx, clusterPolicy, &client.DeleteOptions{})
	target := &apiutil.ErrResourceDiscoveryFailed{}
	if !(apiErrors.IsNotFound(err) || errors.As(err, &target) || meta.IsNoMatchError(err)) {
		result = multierror.Append(result, err)
	}
	err = spyreV2Client.Get(ctx, types.NamespacedName{Name: ClusterPolicyName, Namespace: OperatorNamespace}, clusterPolicy)
	if err == nil {
		// found, need to wait until deleted
		Eventually(func(g Gomega) {
			err00 := spyreV2Client.Get(ctx, types.NamespacedName{Name: ClusterPolicyName, Namespace: OperatorNamespace}, clusterPolicy)
			g.Expect(apiErrors.IsNotFound(err00)).To(BeTrue(), fmt.Sprintf("expect not found error but got %v", err00))
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
	// get csvName before deleting subscription
	csvName, err := getCsvName(ctx, dynClient)
	if err != nil {
		result = multierror.Append(result, err)
	}
	By("deleting subscription")
	if err := DeleteResource(ctx, dynClient, OperatorNamespace, SubscriptionName, subscriptionResource); err != nil && !apiErrors.IsNotFound(err) {
		result = multierror.Append(result, err)
	}
	if csvName != "" {
		By("deleting cluster service version")
		_, _ = fmt.Fprintf(GinkgoWriter, "cluster service version name: %s\n", csvName)
		if err := DeleteResource(ctx, dynClient, OperatorNamespace, csvName, clusterServiceVersionResource); err != nil && !apiErrors.IsNotFound(err) {
			result = multierror.Append(result, err)
		}
	}

	By("deleting install plans")
	installPlans, _ := ListCustomResource(ctx, dynClient, OperatorNamespace, installPlanResource)
	for _, ip := range installPlans {
		if err := DeleteResource(ctx, dynClient, OperatorNamespace, ip, installPlanResource); err != nil && !apiErrors.IsNotFound(err) {
			result = multierror.Append(result, err)
		}
	}

	By("deleting operator group")
	ogs, _ := ListCustomResource(ctx, dynClient, OperatorNamespace, operatorGroupResource)
	for _, og := range ogs {
		if err := DeleteResource(ctx, dynClient, OperatorNamespace, og, operatorGroupResource); err != nil && !apiErrors.IsNotFound(err) {
			result = multierror.Append(result, err)
		}
	}

	By("deleting catalog source")
	if err := DeleteResource(ctx, dynClient, MarketPlaceNamespace, CatalogSourceName, catalogSourceResource); err != nil && !apiErrors.IsNotFound(err) {
		result = multierror.Append(result, err)
	}

	By("restarting packageserver")
	if err := DeletePodsWithLabels(ctx, k8sClientset, OperatorLifecycleManagerNamespace, packageServerLabel, false); err != nil && !apiErrors.IsNotFound(err) {
		result = multierror.Append(result, err)
	}

	result = deleteOperatorPods(ctx, k8sClientset, result)

	By("deleting Spyre operator CRDs")
	for _, spyreCrd := range spyreCrds {
		if err := DeleteResource(ctx, dynClient, metav1.NamespaceAll, spyreCrd, customresourcedefinitionResource); err != nil &&
			!(apiErrors.IsNotFound(err) || meta.IsNoMatchError(err)) {
			result = multierror.Append(result, err)
		}
	}

	if !hasDevice {
		By("cleaning up the node")
		for _, nodeName := range nodeNames {
			CleanUpNode(ctx, k8sClientset, nodeName)
		}
	}
	Expect(result.ErrorOrNil()).To(BeNil())
}

func RestartOperator(g Gomega, ctx context.Context, testConfig TestConfig, k8sClientset *kubernetes.Clientset) {
	operatorPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, operatorLabel, "")
	for _, pod := range operatorPods {
		err := k8sClientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
	}
	waitForOperatorToBeReady(ctx, testConfig, k8sClientset)
}

func deployCatalogSource(ctx context.Context, testConfig TestConfig, k8sClientset *kubernetes.Clientset, dynClient dynamic.Interface) {
	By("deploying catalog source")
	catalogSource := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1alpha1",
		"kind":       "CatalogSource",
		"metadata": map[string]interface{}{
			"name":      CatalogSourceName,
			"namespace": MarketPlaceNamespace,
		},
		"spec": map[string]interface{}{
			"displayName": "Spyre Operators",
			"publisher":   "IBM",
			"sourceType":  "grpc",
			"image":       testConfig.CatalogSource.GetImage(),
			"updateStrategy": map[string]interface{}{
				"registryPoll": map[string]interface{}{
					"interval": "10m",
				},
			},
		},
	}
	_, err := CreateResource(ctx, dynClient, MarketPlaceNamespace, catalogSource, catalogSourceResource)
	Expect(err).To(BeNil())

	deployOperatorGroup(ctx, dynClient)

	Eventually(func(g Gomega) {
		_, err := GetResource(ctx, dynClient, MarketPlaceNamespace, CatalogSourceName, catalogSourceResource)
		g.Expect(err).To(BeNil())
		pods := GetPodsWithLabels(ctx, k8sClientset, g, MarketPlaceNamespace, "olm.catalogSource=ibm-spyre-operators", "")
		g.Expect(pods).To(HaveLen(1))
		g.Expect(pods[0].Status.Phase).To(BeEquivalentTo(v1.PodRunning))
	}).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func deployOperatorGroup(ctx context.Context, dynClient dynamic.Interface) {
	By("deploying operator group")
	operatorGroup := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1",
		"kind":       "OperatorGroup",
		"metadata": map[string]interface{}{
			"name":      OperatorGroupName,
			"namespace": OperatorNamespace,
		},
	}
	_, err := CreateResource(ctx, dynClient, OperatorNamespace, operatorGroup, operatorGroupResource)
	Expect(err).To(BeNil())

	Eventually(func(g Gomega) {
		_, err := GetResource(ctx, dynClient, OperatorNamespace, OperatorGroupName, operatorGroupResource)
		g.Expect(err).To(BeNil())
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func deploySubscription(ctx context.Context, testConfig TestConfig, k8sClientset *kubernetes.Clientset, dynClient dynamic.Interface) {
	By("deploying subscription")
	subscription := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1alpha1",
		"kind":       "Subscription",
		"metadata": map[string]interface{}{
			"name":      SubscriptionName,
			"namespace": OperatorNamespace,
		},
		"spec": map[string]interface{}{
			"channel":         testConfig.Channel,
			"name":            SubscriptionName,
			"source":          CatalogSourceName,
			"sourceNamespace": MarketPlaceNamespace,
		},
	}
	_, err := CreateResource(ctx, dynClient, OperatorNamespace, subscription, subscriptionResource)
	Expect(err).To(BeNil())
	Eventually(func(g Gomega) {
		subscription, err := GetResource(ctx, dynClient, OperatorNamespace, SubscriptionName, subscriptionResource)
		g.Expect(err).To(BeNil())
		status := subscription.Object["status"]
		g.Expect(status).ToNot(BeNil())
		By("getting subscription state")
		conditions, hasConditions := status.(map[string]any)["conditions"]
		g.Expect(hasConditions).To(BeTrue(), "Expecting to have conditions")
		g.Expect(conditions).ToNot(BeNil(), "Expecting subscription status conditions object")
		_, err = fmt.Fprintf(GinkgoWriter, "subscription conditions: %v\n", conditions)
		g.Expect(err).To(BeNil())
		csvNameIface, found := status.(map[string]any)["currentCSV"]
		g.Expect(found).To(BeTrue())
		g.Expect(csvNameIface).ToNot(BeNil(), "Expecting a cluster service version name")
		g.Expect(csvNameIface.(string)).NotTo(BeEquivalentTo(""))
		_, err = k8sClientset.AppsV1().Deployments(OperatorNamespace).Get(ctx, OperatorName, metav1.GetOptions{})
		g.Expect(err).To(BeNil())
	}).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func waitForOperatorToBeReady(ctx context.Context, testConfig TestConfig, k8sClientset *kubernetes.Clientset) {
	By("waiting for the operator pod to be ready")
	Eventually(func(g Gomega) {
		operatorPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, operatorLabel, "")
		checked := false
		for _, operatorPod := range operatorPods {
			for _, container := range operatorPod.Spec.Containers {
				printMessageIfPodNotRunning(operatorPod)
				if container.Name == managerContainerName {
					g.Expect(container.Image).To(BeEquivalentTo(testConfig.Operator.GetImage()))
					checked = true
				}
				g.Expect(operatorPod.Status.Phase).To(BeEquivalentTo(v1.PodRunning))
			}
			g.Expect(checked).To(BeTrue())
		}
	}).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func GetExporterPort(ctx context.Context, k8sClientset *kubernetes.Clientset) int {
	tmplConfigMap, err := k8sClientset.CoreV1().ConfigMaps(OperatorNamespace).Get(ctx, spyreconst.SenlibConfigTmplName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	senlibConfigContent, ok := tmplConfigMap.Data[spyreconst.DefaultSenlibConfigFilename]
	Expect(ok).To(BeTrue())
	var senlibConfig controllers.SenlibConfig
	err = json.Unmarshal([]byte(senlibConfigContent), &senlibConfig)
	Expect(err).To(BeNil())
	return senlibConfig.Metric.General.Port
}

// WriteLogAndState writes log of operator, device plugin, node, and spyre node state before cleanup
func WriteLogAndState(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, g Gomega, nodeName string) {
	By("Prepare log folder")
	PrepareLogFolder(g)
	By("Write log of operator pod")
	pods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, operatorLabel, "")
	Expect(len(pods)).To(Equal(1))
	content, err := GetPodLog(ctx, k8sClientset, managerContainerName, pods[0])
	g.Expect(err).To(BeNil())
	WriteOperatorLog(g, content)
	By("Write log of device plugin pod")
	pods = GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, devicePluginLabel, nodeName)
	Expect(len(pods)).To(Equal(1))
	content, err = GetPodLog(ctx, k8sClientset, devicePluginContainerName, pods[0])
	g.Expect(err).To(BeNil())
	WriteDevicePluginLog(g, content)
	By("Write spyre node state")
	spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, nodeName)
	g.Expect(err).To(BeNil())
	WriteSpyreNodeState(g, spyrens, nodeName)
	By("Write node state")
	node, err := k8sClientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	g.Expect(err).To(BeNil())
	WriteNode(g, node, nodeName)
}

func getCsvName(ctx context.Context, dynClient dynamic.Interface) (string, error) {
	subscription, err := GetResource(ctx, dynClient, OperatorNamespace, SubscriptionName, subscriptionResource)
	if apiErrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if status := subscription.Object["status"]; status != nil {
		if csvNameIface := status.(map[string]interface{})["currentCSV"]; csvNameIface != nil {
			return csvNameIface.(string), nil
		}
	}
	return subscription.GetName(), nil
}

func deleteOperatorPods(ctx context.Context, k8sClientset *kubernetes.Clientset, result *multierror.Error) *multierror.Error {
	By("deleting spyre-operator deployment")
	result = deleteOperatorDeployment(ctx, k8sClientset, OperatorName, operatorLabel, result)

	By("deleting spyre-device-plugin daemon set")
	result = deleteOperatorDaemonSets(ctx, k8sClientset, devicePluginName, devicePluginLabel, result)

	By("deleting spyre-webhook-validator deployment")
	result = deleteOperatorDeployment(ctx, k8sClientset, podValidatorName, podValidatorLabel, result)

	By("deleting spyre-spyre-metrics-exporter daemon set")
	result = deleteOperatorDaemonSets(ctx, k8sClientset, metricsExporterName, metricsExporterLabel, result)

	By("deleting spyre-card-management deployment")
	result = deleteOperatorDeployment(ctx, k8sClientset, cardManagementName, cardManagementLabel, result)

	return result
}

func deleteOperatorDeployment(ctx context.Context, k8sClientset *kubernetes.Clientset,
	name, label string, result *multierror.Error) *multierror.Error {
	err := k8sClientset.AppsV1().Deployments(OperatorNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apiErrors.IsNotFound(err) {
		result = multierror.Append(result, err)
	} else {
		Eventually(func(g Gomega) {
			_, err00 := k8sClientset.AppsV1().Deployments(OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
			g.Expect(apiErrors.IsNotFound(err00)).To(BeTrue())
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		waitForPodsToBeDeleted(ctx, k8sClientset, label)
	}
	return result
}

func deleteOperatorDaemonSets(ctx context.Context, k8sClientset *kubernetes.Clientset,
	name, label string, result *multierror.Error) *multierror.Error {
	err := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apiErrors.IsNotFound(err) {
		result = multierror.Append(result, err)
	} else {
		Eventually(func(g Gomega) {
			_, err00 := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
			g.Expect(apiErrors.IsNotFound(err00)).To(BeTrue())
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		waitForPodsToBeDeleted(ctx, k8sClientset, label)
	}
	return result
}
func waitForPodsToBeDeleted(ctx context.Context, k8sClientset *kubernetes.Clientset, label string) {
	Eventually(func(g Gomega) {
		operatorPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, label, "")
		g.Expect(operatorPods).To(HaveLen(0))
	}).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}
