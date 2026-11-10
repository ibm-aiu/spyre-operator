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
	"slices"
	"time"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	. "github.com/ibm-aiu/spyre-operator/internal/state"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	secv1 "github.com/openshift/api/security/v1"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	It("lets a component tolerate the taints of the components in the same and later states only", func() {
		stateController := newStateController(ctx)
		names := func(state *DeploymentState) []string {
			result := make([]string, 0, len(state.GetComponents()))
			for _, component := range state.GetComponents() {
				result = append(result, component.GetName())
			}
			return result
		}
		initNames, coreNames, pluginNames := names(stateController.InitState),
			names(stateController.CoreComponentState), names(stateController.PluginComponentState)
		Expect(initNames).NotTo(BeEmpty())
		Expect(coreNames).NotTo(BeEmpty())
		Expect(pluginNames).NotTo(BeEmpty())
		for _, component := range stateController.InitState.GetComponents() {
			Expect(component.HoldTolerated()).To(ConsistOf(slices.Concat(initNames, coreNames, pluginNames)))
		}
		for _, component := range stateController.CoreComponentState.GetComponents() {
			Expect(component.HoldTolerated()).To(ConsistOf(slices.Concat(coreNames, pluginNames)))
		}
		for _, component := range stateController.PluginComponentState.GetComponents() {
			Expect(component.HoldTolerated()).To(ConsistOf(pluginNames))
		}
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
			cp.UID = secondUUID
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
								Mode:             spyrev1alpha1.DevicePluginModeClassic,
								DeploymentConfig: ValidDeploymentConfig("device-plugin"),
							},
						},
					}
					By("deploying")
					cm := transformAndSyncInit(stateController, ctx, cp)
					By("removing zombie")
					deletedCount := stateController.RemoveZombieAssets(ctx)
					Expect(deletedCount).To(Equal(9)) // config map + feature rule + non-root scc + 4 TLS cert resources + template + dashboard
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
							Mode:             spyrev1alpha1.DevicePluginModeClassic,
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
				Expect(deletedCount).To(Equal(9)) // config map + feature rule + non-root scc + 4 TLS cert resources + template + dashboard
			})

			It("can sync cluster policy successfully", func() {
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sync-test-policy",
						UID:  "test-uid",
					},
					Spec: spyrev1alpha1.SpyreClusterPolicySpec{
						DevicePlugin: spyrev1alpha1.DevicePluginSpec{
							Mode:             spyrev1alpha1.DevicePluginModeClassic,
							DeploymentConfig: ValidDeploymentConfig("device-plugin"),
						},
					},
				}
				err := K8sClient.Create(ctx, cp)
				Expect(err).To(BeNil())

				state, message, err := stateController.Sync(ctx, cp)
				Expect(err).To(BeNil())
				// State should be NoSpyreNodes or NoNFD since we don't have real nodes
				Expect(state).To(Or(Equal(spyrev1alpha1.NoSpyreNodes), Equal(spyrev1alpha1.NoNFD)))
				Expect(message).NotTo(BeEmpty())

				err = K8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
			})

			It("can clear all states", func() {
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "clear-test-policy",
						UID:  "test-uid-clear",
					},
					Spec: spyrev1alpha1.SpyreClusterPolicySpec{
						DevicePlugin: spyrev1alpha1.DevicePluginSpec{
							Mode:             spyrev1alpha1.DevicePluginModeClassic,
							DeploymentConfig: ValidDeploymentConfig("device-plugin"),
						},
					},
				}
				err := K8sClient.Create(ctx, cp)
				Expect(err).To(BeNil())

				// Sync first to create some resources
				_, _, err = stateController.Sync(ctx, cp)
				Expect(err).To(BeNil())

				// Clear all states
				err = stateController.Clear(ctx)
				Expect(err).To(BeNil())

				err = K8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
			})

			It("should handle DRA mode transition", func() {
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dra-test-policy",
						UID:  "test-uid-dra",
					},
					Spec: spyrev1alpha1.SpyreClusterPolicySpec{
						DevicePlugin: spyrev1alpha1.DevicePluginSpec{
							DeploymentConfig: ValidDeploymentConfig("device-plugin"),
							Mode:             spyrev1alpha1.DevicePluginModeDRA,
						},
					},
				}
				err := K8sClient.Create(ctx, cp)
				Expect(err).To(BeNil())

				// Sync with DRA enabled
				state, message, err := stateController.Sync(ctx, cp)
				Expect(err).To(BeNil())
				// Should succeed or return NoSpyreNodes/NoNFD
				Expect(state).To(Or(
					Equal(spyrev1alpha1.Ready),
					Equal(spyrev1alpha1.NoSpyreNodes),
					Equal(spyrev1alpha1.NoNFD),
				))
				_ = message

				err = K8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
			})
		})
	})

	Context("mode switch between the device plugin and the DRA driver", func() {
		var (
			stateController *StateController
			created         []client.Object
		)
		policy := func(mode spyrev1alpha1.DevicePluginMode) *spyrev1alpha1.SpyreClusterPolicy {
			return &spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "mode-switch-policy"},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					DevicePlugin: spyrev1alpha1.DevicePluginSpec{
						Mode: mode, DeploymentConfig: ValidDeploymentConfig("device-plugin"),
					},
				},
			}
		}
		podSpec := corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "image"}}}
		// deployDriver creates the DaemonSet of the driver component as if the previous mode had deployed it
		deployDriver := func(componentName string) {
			for _, component := range stateController.CoreComponentState.GetComponents() {
				if component.GetName() != componentName {
					continue
				}
				for _, obj := range component.GetObjects() {
					if obj.GetID().Kind != "DaemonSet" {
						continue
					}
					labels := map[string]string{"app": componentName}
					ds := &appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{Name: obj.GetID().Name, Namespace: OpNs},
						Spec: appsv1.DaemonSetSpec{
							Selector: &metav1.LabelSelector{MatchLabels: labels},
							Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: podSpec},
						},
					}
					Expect(K8sClient.Create(ctx, ds)).To(Succeed())
					created = append(created, ds)
					return
				}
			}
			Fail("no DaemonSet found in " + componentName)
		}
		createRunnerPod := func() *corev1.Pod {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mode-switch-runner", Namespace: OpNs,
					Labels: map[string]string{
						spyreconst.CardManagementRunnerLabelKey: spyreconst.CardManagementRunnerLabelValue},
				},
				Spec: podSpec,
			}
			Expect(K8sClient.Create(ctx, pod)).To(Succeed())
			created = append(created, pod)
			return pod
		}

		BeforeEach(func() {
			stateController = newStateController(ctx)
		})

		AfterEach(func() {
			for _, obj := range created {
				Expect(client.IgnoreNotFound(StateClient.Delete(ctx, obj))).To(Succeed())
			}
			created = nil
		})

		It("deletes card management once before switching to DRA", func() {
			deployDriver(spyreconst.DevicePluginResourceName)
			runner := createRunnerPod()

			switching, state, message, err := stateController.PrepareModeSwitch(ctx, policy(spyrev1alpha1.DevicePluginModeDRA))
			Expect(err).To(BeNil())
			Expect(switching).To(BeTrue())
			Expect(state).To(BeEmpty(), message)
			Expect(stateController.PluginComponentState.Suspended()).To(ConsistOf(spyreconst.CardManagementResourceName))
			By("expecting card management Pods are already gone, without waiting")
			err = K8sClient.Get(ctx, client.ObjectKeyFromObject(runner), &corev1.Pod{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("does not suspend card management when no switch is pending", func() {
			createRunnerPod()

			switching, state, message, err := stateController.PrepareModeSwitch(ctx, policy(spyrev1alpha1.DevicePluginModeDRA))
			Expect(err).To(BeNil())
			Expect(switching).To(BeFalse())
			Expect(state).To(BeEmpty(), message)
			Expect(stateController.PluginComponentState.Suspended()).To(BeEmpty())
			err = K8sClient.Get(ctx, client.ObjectKey{Name: "mode-switch-runner", Namespace: OpNs}, &corev1.Pod{})
			Expect(err).To(BeNil())
		})

		It("treats an existing SpyreNodeState as an old resource when switching to DRA", func() {
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{Name: "mode-switch-node"},
				Spec:       spyrev1alpha1.SpyreNodeStateSpec{NodeName: "mode-switch-node"},
			}
			Expect(K8sClient.Create(ctx, nodeState)).To(Succeed())
			created = append(created, nodeState)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "mode-switch-reserved", Namespace: "default"},
				Spec:       podSpec,
			}
			Expect(K8sClient.Create(ctx, pod)).To(Succeed())
			created = append(created, pod)
			nodeState.Status.ReserveDevices("spyre_pf",
				spyrev1alpha1.Pod{Name: pod.Name, Namespace: pod.Namespace}, []string{"0001"}, metav1.Now())
			Expect(K8sClient.Status().Update(ctx, nodeState)).To(Succeed())

			By("blocking while a Pod holds a reservation, even though the device plugin is not deployed")
			switching, state, message, err := stateController.PrepareModeSwitch(ctx, policy(spyrev1alpha1.DevicePluginModeDRA))
			Expect(err).To(HaveOccurred())
			Expect(switching).To(BeTrue())
			Expect(state).To(Equal(spyrev1alpha1.NotReady))
			Expect(message).To(ContainSubstring("default/mode-switch-reserved"))
			Expect(K8sClient.Get(ctx, client.ObjectKeyFromObject(nodeState), &spyrev1alpha1.SpyreNodeState{})).To(Succeed())

			By("deleting the SpyreNodeState once the Pod is gone")
			Expect(StateClient.Delete(ctx, pod)).To(Succeed())
			switching, state, message, err = stateController.PrepareModeSwitch(ctx, policy(spyrev1alpha1.DevicePluginModeDRA))
			Expect(err).To(BeNil())
			Expect(switching).To(BeTrue())
			Expect(state).To(BeEmpty(), message)
			Eventually(func() bool {
				err := K8sClient.Get(ctx, client.ObjectKeyFromObject(nodeState), &spyrev1alpha1.SpyreNodeState{})
				return errors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("blocks switching to the device plugin while Spyre ResourceClaims are allocated", func() {
			deployDriver(spyreconst.DRADriverResourceName)
			claim := &resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "mode-switch-claim", Namespace: "default"},
				Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
					Requests: []resourcev1.DeviceRequest{{
						Name: "req", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "spyre-pf"}}}}},
			}
			Expect(StateClient.Create(ctx, claim)).To(Succeed())
			created = append(created, claim)
			claim.Status.Allocation = &resourcev1.AllocationResult{Devices: resourcev1.DeviceAllocationResult{
				Results: []resourcev1.DeviceRequestAllocationResult{{
					Request: "req", Driver: spyreconst.DRADriverName, Pool: "node1", Device: "dev0"}}}}
			Expect(StateClient.Status().Update(ctx, claim)).To(Succeed())

			switching, state, message, err := stateController.PrepareModeSwitch(ctx, policy(spyrev1alpha1.DevicePluginModeClassic))
			Expect(err).To(HaveOccurred())
			Expect(switching).To(BeTrue())
			Expect(state).To(Equal(spyrev1alpha1.NotReady))
			Expect(message).To(ContainSubstring("default/mode-switch-claim"))
			Expect(stateController.PluginComponentState.Suspended()).To(ConsistOf(spyreconst.CardManagementResourceName))

			By("passing once the claim is released")
			Expect(StateClient.Delete(ctx, claim)).To(Succeed())
			switching, state, message, err = stateController.PrepareModeSwitch(ctx, policy(spyrev1alpha1.DevicePluginModeClassic))
			Expect(err).To(BeNil())
			Expect(switching).To(BeTrue())
			Expect(state).To(BeEmpty(), message)
		})

		It("blocks clearing core components while Spyre ResourceClaims are allocated", func() {
			deployDriver(spyreconst.DRADriverResourceName)
			claim := &resourcev1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "clear-claim", Namespace: "default"},
				Spec: resourcev1.ResourceClaimSpec{Devices: resourcev1.DeviceClaim{
					Requests: []resourcev1.DeviceRequest{{
						Name: "req", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: "spyre-pf"}}}}},
			}
			Expect(StateClient.Create(ctx, claim)).To(Succeed())
			created = append(created, claim)
			claim.Status.Allocation = &resourcev1.AllocationResult{Devices: resourcev1.DeviceAllocationResult{
				Results: []resourcev1.DeviceRequestAllocationResult{{
					Request: "req", Driver: spyreconst.DRADriverName, Pool: "node1", Device: "dev0"}}}}
			Expect(StateClient.Status().Update(ctx, claim)).To(Succeed())

			err := stateController.Clear(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot clear core components"))
			Expect(err.Error()).To(ContainSubstring("default/clear-claim"))

			By("clearing successfully once the claim is deleted")
			Expect(StateClient.Delete(ctx, claim)).To(Succeed())
			err = stateController.Clear(ctx)
			Expect(err).To(BeNil())
		})

		DescribeTable("decides whether the cluster is running and its state when not all the components are ready",
			func(previous spyrev1alpha1.State, switching, expectRunning bool, expectState spyrev1alpha1.State) {
				cp := policy(spyrev1alpha1.DevicePluginModeDRA)
				cp.Status.State = previous
				running := ClusterRunning(cp, switching)
				Expect(running).To(Equal(expectRunning))
				state, _ := OverallState(running, false, false, []string{"not ready"}, nil, nil)
				Expect(state).To(Equal(expectState))
			},
			Entry("Ready without switching is Degraded",
				spyrev1alpha1.Ready, false, true, spyrev1alpha1.Degraded),
			Entry("Degraded without switching is Degraded",
				spyrev1alpha1.Degraded, false, true, spyrev1alpha1.Degraded),
			Entry("NotReady without switching is NotReady",
				spyrev1alpha1.NotReady, false, false, spyrev1alpha1.NotReady),
			Entry("Ready while switching the mode is NotReady",
				spyrev1alpha1.Ready, true, false, spyrev1alpha1.NotReady),
			Entry("Degraded while switching the mode is NotReady",
				spyrev1alpha1.Degraded, true, false, spyrev1alpha1.NotReady),
		)
	})
})

func newStateController(ctx context.Context) *StateController {
	controllerScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(controllerScheme)
	spyrev1alpha1.AddToScheme(controllerScheme)
	nfdv1alpha1.AddToScheme(controllerScheme)
	secv1.AddToScheme(controllerScheme)
	err := InitializeScheme(controllerScheme)
	Expect(err).To(BeNil())
	stateController, err := NewStateController(ctx, Cfg, controllerScheme)
	Expect(err).To(BeNil())
	return stateController
}

func transformAndSyncInit(stateController *StateController,
	ctx context.Context, cp *spyrev1alpha1.SpyreClusterPolicy) *corev1.ConfigMap {
	namespace := stateController.OperatorNamespace
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
