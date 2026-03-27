/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyrev2 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/internal/state"
	. "github.com/ibm-aiu/spyre-operator/internal/state"
	securityv1 "github.com/openshift/api/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ControlledComponent", Ordered, func() {
	ctx := context.Background()
	cpName := "controlled-component-test-policy"

	BeforeEach(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName},
			Spec: spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					DeploymentConfig: ValidDeploymentConfig("device-plugin"),
				},
				CardManagement: spyrev1alpha1.CardManagementSpec{
					DeploymentConfig: ValidDeploymentConfig("card-management"),
				},
			},
		}
		err := K8sClient.Create(ctx, cp)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName}}
		err := K8sClient.Delete(ctx, cp)
		valid := err == nil || errors.IsNotFound(err)
		Expect(valid).To(BeTrue())
	})

	It("can transform pod validator by SpyreClusterPolicy", func() {
		path := filepath.Join(AssetsPath, "state-init", "spyre-webhook-validator", "0400_deployment.yaml")
		runtimeObj, gvk, err := DecodeFromFile(StateScheme, path)
		Expect(err).To(BeNil())
		defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
		Expect(err).To(BeNil())
		controlledObj, err := NewDeployment(defaultObj, runtimeObj, OpNs)
		Expect(err).To(BeNil())
		cp := &spyrev1alpha1.SpyreClusterPolicy{}
		err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
		Expect(err).To(BeNil())
		cp.Spec.PodValidator = spyrev1alpha1.PodValidatorSpec{
			DeploymentConfig: ValidDeploymentConfig("validator"),
			Enabled:          true,
		}
		err = controlledObj.TransformByPolicy(StateScheme, cp, &ClusterState{OperatorNamespace: OpNs})
		Expect(err).To(BeNil())
		err = StateClient.Create(ctx, controlledObj.GetObject())
		Expect(err).To(BeNil())
		err = StateClient.Delete(ctx, controlledObj.GetObject())
		Expect(err).To(BeNil())
	})

	It("can transform ConfigMap so that user can customize senlib template name by SpyreClusterPolicy", func() {
		senlibTemplatePath := filepath.Join(AssetsPath, "state-core-components", spyreconst.DevicePluginResourceName, "0400_senlib_template.yaml")
		runtimeObj, gvk, err := DecodeFromFile(StateScheme, senlibTemplatePath)
		Expect(err).To(BeNil())
		defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
		Expect(err).To(BeNil())
		controlledObj, err := NewConfigMap(defaultObj, runtimeObj, OpNs)
		Expect(err).To(BeNil())
		_, ok := controlledObj.(*SenlibConfigTemplateConfigMap)
		Expect(ok).To(BeTrue())
		By("applying new config value")
		cp := &spyrev1alpha1.SpyreClusterPolicy{}
		err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
		Expect(err).To(BeNil())
		configName := "custom-config-name.json"
		cp.Spec.DevicePlugin.ConfigName = configName
		err = controlledObj.TransformByPolicy(StateScheme, cp, &ClusterState{OperatorNamespace: OpNs})
		Expect(err).To(BeNil())
		transformedObj := controlledObj.GetObject()
		cmObj, ok := transformedObj.(*corev1.ConfigMap)
		Expect(ok).To(BeTrue())
		_, found := cmObj.Data[configName]
		Expect(found).To(BeTrue())
		By("removing config, checking default value applied")
		cp.Spec.DevicePlugin.ConfigName = ""
		err = controlledObj.TransformByPolicy(StateScheme, cp, &ClusterState{OperatorNamespace: OpNs})
		Expect(err).To(BeNil())
		transformedObj = controlledObj.GetObject()
		cmObj, ok = transformedObj.(*corev1.ConfigMap)
		Expect(ok).To(BeTrue())
		_, found = cmObj.Data[configName]
		Expect(found).To(BeFalse())
		_, found = cmObj.Data[spyreconst.DefaultSenlibConfigFilename]
		Expect(found).To(BeTrue())
	})

	Context("spyre-device-plugin", func() {
		var component *ControlledComponent
		BeforeEach(func() {
			componentName := spyreconst.DevicePluginResourceName
			componentPath := filepath.Join(AssetsPath, "state-core-components", componentName)
			var err error
			component, err = NewControlledComponent(ctx, StateClient, StateScheme, componentPath, OpNs, componentName)
			Expect(err).To(BeNil())

			// Skip SCC under envtest: remove any SecurityContextConstraints from this component
			{
				objs := component.GetObjects()
				filtered := make([]ControlledObject, 0, len(objs))
				for _, o := range objs {
					if _, isSCC := o.GetObject().(*securityv1.SecurityContextConstraints); isSCC {
						continue
					}
					filtered = append(filtered, o)
				}
				component.SetObjects(filtered)
			}

			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			err = component.Transform(cp, &ClusterState{OperatorNamespace: cp.Name})
			Expect(err).To(BeNil())
			By("creating")
			_, _, err = component.Sync(ctx)
			Expect(err).To(BeNil())
			By("expecting not ready")
		})

		AfterEach(func() {
			By("cleaning up")
			err := component.DeleteAll(ctx)
			Expect(err).To(BeNil())
		})

		It("can transform and sync", func() {
			updateNumberUnavailable(ctx, component, 1)
			ready, message, err := component.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeFalse())
			Expect(message).To(BeEquivalentTo("last step is not ready"))
			By("expecting ready")
			updateNumberUnavailable(ctx, component, 0)
			ready, message, err = component.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(message).To(BeEquivalentTo(""))
		})

		It("can clear node state on clear", func() {
			nodeName := "dummy"
			By("creating dummy nodestate")
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: v1.ObjectMeta{
					Name: nodeName,
				},
				Spec: spyrev2.SpyreNodeStateSpec{
					Pcitopo: "topo",
					SpyreInterfaces: []spyrev1alpha1.SpyreInterface{
						{PciAddress: "00"}},
				},
			}
			err := K8sClient.Create(ctx, nodeState)
			Expect(err).To(BeNil())
			component.Clear(ctx)
			By("checking dummy nodestate")
			ns := &spyrev1alpha1.SpyreNodeState{}
			err = K8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, ns)
			Expect(err).To(BeNil())
			Expect(ns.Spec.Pcitopo).To(Equal(""))
			Expect(ns.Spec.SpyreInterfaces).To(HaveLen(0))
			By("deleting dummy nodestate")
			err = K8sClient.Delete(ctx, ns)
			Expect(err).To(BeNil())
		})
	})

	Context("spyre-card-management", func() {

		AfterEach(func() {
			m, err := filepath.Glob("../../assets/state-plugin-components/spyre-card-management/*_daemonset_*.yaml")
			Expect(err).To(BeNil())
			for _, f := range m {
				err := os.Remove(f)
				Expect(err).To(BeNil())
			}
			m, err = filepath.Glob("../../assets/state-plugin-components/spyre-card-management/*.tmp")
			Expect(err).To(BeNil())
			for _, f := range m {
				err := os.Remove(f)
				Expect(err).To(BeNil())
			}
		})

		It("can delete Pods with spyrecardmanager label at disabling cardmgmt", func() {

			ctx := context.Background()
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "spyrecardmanager1",
					Namespace: OpNs,
					Labels:    map[string]string{"spyrecardmanager": "1"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c1", Image: "image"}},
				},
			}
			err := K8sClient.Create(ctx, p)
			Expect(err).To(BeNil())
			p = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "spyrecardmanager2",
					Namespace: OpNs,
					Labels:    map[string]string{"spyrecardmanager": "1"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c1", Image: "image"}},
				},
			}
			err = K8sClient.Create(ctx, p)
			Expect(err).To(BeNil())

			c := ControlledComponent{}
			c.SetDisable(true)
			c.ExportSetName(spyreconst.CardManagementResourceName)
			c.ExportSetClient(K8sClient)
			_, msg, _ := c.Sync(ctx)
			Expect(msg).Should(Equal(""))

			err = K8sClient.Get(ctx, client.ObjectKey{Name: "aiucardmanager1", Namespace: OpNs}, p)
			Expect(err.(*errors.StatusError).ErrStatus.Code).Should(Equal(int32(404)))
			err = K8sClient.Get(ctx, client.ObjectKey{Name: "aiucardmanager2", Namespace: OpNs}, p)
			Expect(err.(*errors.StatusError).ErrStatus.Code).Should(Equal(int32(404)))
		})

		It("can skip update", func() {
			serviceAccountName := "spyre-card-management"
			c := ControlledComponent{}
			c.ExportSetClient(K8sClient)
			runtimeObj := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: OpNs,
				},
			}
			defaultObj, err := NewDefaultObject(ctx, runtimeObj.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())
			controlledObj, err := NewServiceAccount(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			c.SetObjects([]ControlledObject{controlledObj})
			// object create
			_, msg, _ := c.Sync(ctx)
			Expect(msg).Should(Equal(""))
			var sa corev1.ServiceAccount
			err = K8sClient.Get(ctx, client.ObjectKey{Name: serviceAccountName, Namespace: OpNs}, &sa)
			Expect(err).To(BeNil())
			// object unsync
			runtimeObj.ImagePullSecrets = []corev1.LocalObjectReference{
				{Name: "default"},
			}
			// should be able to skip update if skipUpdate=true
			c.SetSkipUpdate(true)
			_, msg, _ = c.Sync(ctx)
			Expect(msg).Should(Equal(""))
			err = K8sClient.Get(ctx, client.ObjectKey{Name: serviceAccountName, Namespace: OpNs}, &sa)
			Expect(err).To(BeNil())
			Expect(sa.ImagePullSecrets).To(HaveLen(0))
			// should be able to update if skipUpdate=false
			c.SetSkipUpdate(false)
			_, msg, _ = c.Sync(ctx)
			Expect(msg).Should(Equal(""))
			err = K8sClient.Get(ctx, client.ObjectKey{Name: serviceAccountName, Namespace: OpNs}, &sa)
			Expect(err).To(BeNil())
			Expect(sa.ImagePullSecrets).To(HaveLen(1))
			Expect(sa.ImagePullSecrets[0].Name).To(BeEquivalentTo("default"))
			// clean up
			K8sClient.Delete(ctx, controlledObj.GetObject())
		})

		It("can apply num of spyre cards at transform", func() {

			stateName := "state-plugin-components"
			nodeNames := []string{"node1", "node2"}

			By("creating Node resources")
			for idx, n := range nodeNames {
				numCards := strconv.Itoa((idx + 1) * 2)
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: n,
						Labels: map[string]string{
							"node-role.kubernetes.io/worker": "",
							spyreconst.SpyreCountLabelKey:    numCards},
					},
				}
				err := K8sClient.Get(ctx, client.ObjectKey{Name: n}, node)
				if err != nil {
					opt := &client.CreateOptions{}
					err = K8sClient.Create(ctx, node, opt)
					Expect(err).To(BeNil())
				}
			}

			path := filepath.Join(AssetsPath, stateName)
			deployState, err := NewDeploymentState(ctx, StateClient, StateScheme, path, OpNs)
			Expect(err).To(BeNil())
			Expect(deployState).NotTo(BeNil())
			Expect(deployState.GetName()).To(BeEquivalentTo(stateName))

			By("creating component")
			componentName := spyreconst.CardManagementResourceName
			componentPath := filepath.Join(AssetsPath, stateName, componentName)
			component, err := NewControlledComponent(ctx, StateClient, StateScheme, componentPath, OpNs, componentName)
			Expect(err).To(BeNil())

			By("transforming and synchronizing component")
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			clusterState, err := state.NewClusterState(ctx, K8sClient)
			Expect(err).To(BeNil())
			err = component.Transform(cp, clusterState)
			Expect(err).To(BeNil())
			By("creating")
			_, _, err = component.Sync(ctx)
			Expect(err).To(BeNil())
		})
	})

})

func updateNumberUnavailable(ctx context.Context, component *ControlledComponent, unavailable int) {
	By("setting number of unavailable")
	objects := component.GetObjects()
	Expect(len(objects)).To(BeNumerically(">", 0))
	obj := objects[len(objects)-1]
	runtimeObj := obj.GetObject()
	deploy, ok := runtimeObj.(*appsv1.DaemonSet)
	Expect(ok).To(BeTrue())

	// expected
	deploy.Status.DesiredNumberScheduled = 1

	// actual
	deploy.Status.NumberUnavailable = int32(unavailable)
	n := int32(1 - unavailable)
	deploy.Status.NumberReady = n
	deploy.Status.CurrentNumberScheduled = n
	deploy.Status.UpdatedNumberScheduled = n
	deploy.Status.NumberAvailable = n

	err := K8sClient.Status().Update(ctx, deploy)
	Expect(err).To(BeNil())
}
