/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"
	"fmt"
	"time"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	. "github.com/ibm-aiu/spyre-operator/internal/state"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	secv1 "github.com/openshift/api/security/v1"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("StateController", Ordered, func() {
	ctx := context.Background()

	It("init values are not nil", func() {
		stateController := newStateController(ctx)
		clusterState := stateController.ClusterState
		Expect(clusterState).NotTo(BeNil())
		Expect(clusterState.OperatorNamespace).To(BeEquivalentTo(OpNs))
		Expect(stateController.SpyreNodeStateState).NotTo(BeNil())
		checkCommonNewDeploymentState(stateController.InitState)
		checkCommonNewDeploymentState(stateController.CoreComponentState)
		checkCommonNewDeploymentState(stateController.PluginComponentState)
	})

	Context("TransformAndSync", Ordered, func() {
		var stateController *StateController
		BeforeEach(func() {
			stateController = newStateController(ctx)
		})

		AfterEach(func() {
			for _, component := range stateController.InitState.GetComponents() {
				err := component.DeleteAll(ctx)
				Expect(err).To(BeNil())
			}
		})

		It("can update owner UUID", func() {
			cpName := "update-uuid-policy"
			firstUUID := uuid.NewUUID()
			secondUUID := uuid.NewUUID()
			Expect(firstUUID).NotTo(Equal(secondUUID))
			cp := &spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: cpName,
					UID:  firstUUID,
				},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{},
			}
			By("checking first uuid")
			cm := transformAndSyncInit(stateController, ctx, cp)
			Expect(cm.OwnerReferences[0].UID).To(BeEquivalentTo(firstUUID))
			By("checking second uuid")
			cp.ObjectMeta.UID = secondUUID
			cm = transformAndSyncInit(stateController, ctx, cp)
			Expect(cm.OwnerReferences[0].UID).To(BeEquivalentTo(secondUUID))
			err := K8sClient.Delete(ctx, cm)
			Expect(err).To(BeNil())
			namespacedName := types.NamespacedName{Name: cm.GetName(), Namespace: cm.GetNamespace()}
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, namespacedName, cm)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		})

		Context("zombie asset", func() {
			It("can remove zombie asset", func() {
				By("creating zombie asset", func() {
					cpName := "zombie-policy"
					cp := &spyrev1alpha1.SpyreClusterPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name: cpName,
							UID:  uuid.NewUUID(),
						},
						Spec: spyrev1alpha1.SpyreClusterPolicySpec{
							DevicePlugin: spyrev1alpha1.DevicePluginSpec{
								DeploymentConfig: ValidDeploymentConfig("device-plugin"),
							},
						},
					}
					By("deploying")
					cm := transformAndSyncInit(stateController, ctx, cp)
					By("removing zombie")
					deletedCount := stateController.RemoveZombieAssets(ctx)
					Expect(deletedCount).To(Equal(3)) // config map + feature rule + non-root scc
					namespacedName := types.NamespacedName{Name: cm.GetName(), Namespace: cm.GetNamespace()}
					Eventually(func(g Gomega) {
						err := K8sClient.Get(ctx, namespacedName, cm)
						g.Expect(errors.IsNotFound(err)).To(BeTrue())
					}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
				})
			})

			It("must not delete assets with owner", func() {
				cpName := "valid-owner"
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: cpName,
						UID:  uuid.NewUUID(),
					},
					Spec: spyrev1alpha1.SpyreClusterPolicySpec{
						DevicePlugin: spyrev1alpha1.DevicePluginSpec{
							DeploymentConfig: ValidDeploymentConfig("device-plugin"),
						},
					},
				}
				err := K8sClient.Create(ctx, cp)
				Expect(err).To(BeNil())
				By("deploying")
				cm := transformAndSyncInit(stateController, ctx, cp)
				By("removing zombie (with owner)")
				deletedCount := stateController.RemoveZombieAssets(ctx)
				Expect(deletedCount).To(Equal(0))
				namespacedName := types.NamespacedName{Name: cm.GetName(), Namespace: cm.GetNamespace()}
				err = K8sClient.Get(ctx, namespacedName, cm)
				Expect(err).To(BeNil())
				Expect(cm.DeletionTimestamp).To(BeNil())
				By("cleaning up")
				err = K8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
				By("removing zombie (without owner)")
				deletedCount = stateController.RemoveZombieAssets(ctx)
				Expect(deletedCount).To(Equal(3))
			})
		})
	})
})

func newStateController(ctx context.Context) *StateController {
	controllerScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(controllerScheme)
	spyrev1alpha1.AddToScheme(controllerScheme)
	nfdv1alpha1.AddToScheme(controllerScheme)
	secv1.AddToScheme(controllerScheme)
	stateController, err := NewStateController(ctx, Cfg, controllerScheme)
	Expect(err).To(BeNil())
	return stateController
}

func transformAndSyncInit(stateController *StateController,
	ctx context.Context, cp *spyrev1alpha1.SpyreClusterPolicy) *corev1.ConfigMap {
	namespace := stateController.ClusterState.OperatorNamespace
	overallStatus, message, err := stateController.InitState.TransformAndSync(ctx, cp, stateController.ClusterState)
	Expect(overallStatus).To(BeTrue(), fmt.Sprintf("%v", message))
	Expect(message).To(HaveLen(0))
	Expect(err).To(BeNil())
	By("checking exist")
	cm := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: "spyre-config", Namespace: namespace}
	Eventually(func(g Gomega) {
		err = K8sClient.Get(ctx, namespacedName, cm)
		g.Expect(err).To(BeNil())
	}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	return cm
}

func checkCommonNewDeploymentState(state *DeploymentState) {
	Expect(state).NotTo(BeNil())
	for _, component := range state.GetComponents() {
		for _, obj := range component.GetObjects() {
			runtimeObj := obj.GetObject()
			objID := obj.GetID()
			By(fmt.Sprintf("checking %s", objID))
			Expect(objID.Namespace).NotTo(BeEquivalentTo(spyreconst.OPERATOR_FILLED))
			Expect(runtimeObj.GetNamespace()).To(BeEquivalentTo(objID.Namespace))
		}
	}
}
