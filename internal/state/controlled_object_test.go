/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"
	"path/filepath"
	"strings"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	. "github.com/ibm-aiu/spyre-operator/internal/state"
	spyreerr "github.com/ibm-aiu/spyre-operator/pkg/errors"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ControlledObject", Ordered, func() {
	ctx := context.Background()
	cpName := "controlled-object-test-policy"
	devicePluginDaemonSetPath := filepath.Join(AssetsPath, "state-core-components", spyreconst.DevicePluginResourceName, "0500_daemonset.yaml")

	BeforeEach(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName},
			Spec: spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					Mode:             spyrev1alpha1.DevicePluginModeClassic,
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

	It("can apply and revert experimental modes", func() {
		runtimeObj, gvk, err := DecodeFromFile(StateScheme, devicePluginDaemonSetPath)
		Expect(err).To(BeNil())
		defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
		Expect(err).To(BeNil())
		controlledObj, err := NewDaemonSet(defaultObj, runtimeObj, OpNs)
		Expect(err).To(BeNil())
		cp := &spyrev1alpha1.SpyreClusterPolicy{}
		err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
		Expect(err).To(BeNil())
		By("adding mode")
		cp.Spec.ExperimentalMode = []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.ReservationMode}
		err = controlledObj.TransformByPolicy(StateScheme, cp, &ClusterState{OperatorNamespace: OpNs})
		Expect(err).To(BeNil())
		transformedObj := controlledObj.GetObject()
		ds, ok := transformedObj.(*appsv1.DaemonSet)
		Expect(ok).To(BeTrue())
		checkModeEnable(ds, spyrev1alpha1.ReservationMode, true, spyreconst.ModeEnabledValue)
		By("removing mode")
		cp.Spec.ExperimentalMode = []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{}
		err = controlledObj.TransformByPolicy(StateScheme, cp, &ClusterState{OperatorNamespace: OpNs})
		Expect(err).To(BeNil())
		transformedObj = controlledObj.GetObject()
		ds, ok = transformedObj.(*appsv1.DaemonSet)
		Expect(ok).To(BeTrue())
		checkModeEnable(ds, spyrev1alpha1.ReservationMode, false, "")
	})

	Context("card management", func() {
		cardManagementPath := filepath.Join(AssetsPath, "state-plugin-components", spyreconst.CardManagementResourceName)

		emptyStr := ""
		anSpyreFilter := "myfilter"
		pfImage := "pfimage"
		vfImage := "vfimage"
		defaultRunnerImage := ""

		DescribeTable("aiucardmgmt.ini", func(disableVfMode bool, cardMgmtConfig *spyrev1alpha1.CardManagementConfig,
			expectedSpyreFilter, expectedPfImage, expectedVfImage string) {
			configPath := filepath.Join(cardManagementPath, "0500_configmap.yaml")
			runtimeObj, gvk, err := DecodeFromFile(StateScheme, configPath)
			Expect(err).To(BeNil())
			defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())
			controlledObj, err := NewConfigMap(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			cp.Spec.CardManagement.Enabled = true
			if disableVfMode {
				cp.Spec.ExperimentalMode = append(cp.Spec.ExperimentalMode, spyrev1alpha1.DisableVfMode)
			}
			cp.Spec.CardManagement.Config = cardMgmtConfig
			By("updating cluster policy to apply default values")
			err = K8sClient.Update(ctx, cp, &client.UpdateOptions{})
			Expect(err).To(BeNil())
			err = controlledObj.TransformByPolicy(StateScheme, cp, &ClusterState{OperatorNamespace: OpNs})
			Expect(err).To(BeNil())
			transformedObj := controlledObj.GetObject()
			config, ok := transformedObj.(*corev1.ConfigMap)
			Expect(ok).To(BeTrue())
			content, ok := config.Data[spyreconst.CardManagementConfigFile]
			Expect(ok).To(BeTrue())
			Expect(content).To(ContainSubstring(OpNs))
			if disableVfMode {
				Expect(strings.Contains(content, "False")).To(BeTrue())
				Expect(strings.Contains(content, "True")).To(BeFalse())
			} else {
				Expect(strings.Contains(content, "True")).To(BeTrue())
				Expect(strings.Contains(content, "False")).To(BeFalse())
			}
			Expect(content).Should(MatchRegexp("aiufilter\\s*=\\s*" + expectedSpyreFilter))
			Expect(content).Should(MatchRegexp("pfimage_URL\\s*=\\s*" + expectedPfImage))
			Expect(content).Should(MatchRegexp("vfimage_URL\\s*=\\s*" + expectedVfImage))
		},
			Entry("disableVF-nilSpyreFilter", true, nil, ".", defaultRunnerImage, defaultRunnerImage),
			Entry("enableVF-nilSpyreFilter", false, nil, ".", defaultRunnerImage, defaultRunnerImage),
			Entry("disableVF-emptySpyreFilter", true,
				&spyrev1alpha1.CardManagementConfig{
					SpyreFilter: &emptyStr,
				}, ".", defaultRunnerImage, defaultRunnerImage),
			Entry("enableVF-emptySpyreFilter", false,
				&spyrev1alpha1.CardManagementConfig{
					SpyreFilter: &emptyStr,
				}, ".", defaultRunnerImage, defaultRunnerImage),
			Entry("disableVF-customSpyreFilter", true,
				&spyrev1alpha1.CardManagementConfig{
					SpyreFilter: &anSpyreFilter,
				}, anSpyreFilter, defaultRunnerImage, defaultRunnerImage),
			Entry("enableVF-customSpyreFilter", false,
				&spyrev1alpha1.CardManagementConfig{
					SpyreFilter: &anSpyreFilter,
				}, anSpyreFilter, defaultRunnerImage, defaultRunnerImage),
			Entry("set PF runner image", false,
				&spyrev1alpha1.CardManagementConfig{
					PfRunnerImage: &pfImage,
				}, ".", pfImage, defaultRunnerImage),
			Entry("set VF runner image", false,
				&spyrev1alpha1.CardManagementConfig{
					VfRunnerImage: &vfImage,
				}, ".", defaultRunnerImage, vfImage),
			Entry("set both runner image with filter", false,
				&spyrev1alpha1.CardManagementConfig{
					SpyreFilter:   &anSpyreFilter,
					PfRunnerImage: &pfImage,
					VfRunnerImage: &vfImage,
				}, anSpyreFilter, pfImage, vfImage),
		)

		It("can handle DeviceClass object", func() {
			deviceClassPath := filepath.Join(AssetsPath, "state-core-components", "dra-driver-spyre", "0500_deviceclass_pf.yaml")
			runtimeObj, gvk, err := DecodeFromFile(StateScheme, deviceClassPath)
			Expect(err).To(BeNil())
			Expect(gvk.Kind).To(Equal("DeviceClass"))

			defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())

			controlledObj, err := NewDeviceClass(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			Expect(controlledObj).NotTo(BeNil())

			// Test GetID
			objID := controlledObj.GetID()
			Expect(objID.Kind).To(Equal("DeviceClass"))

			// Test GetObject
			obj := controlledObj.GetObject()
			Expect(obj).NotTo(BeNil())
		})
	})

	Context("ResourceClaimTemplate", func() {
		cardmgmtDir := filepath.Join(AssetsPath, "state-plugin-components", "spyre-card-management")
		coreDir := filepath.Join(AssetsPath, "state-core-components", "dra-driver-spyre")

		// deviceClassNames returns the names of all DeviceClass assets of the DRA driver.
		deviceClassNames := func() []string {
			files, err := filepath.Glob(filepath.Join(coreDir, "*deviceclass*.yaml"))
			Expect(err).To(BeNil())
			Expect(files).NotTo(BeEmpty())
			names := make([]string, 0, len(files))
			for _, f := range files {
				runtimeObj, gvk, err := DecodeFromFile(StateScheme, f)
				Expect(err).To(BeNil())
				Expect(gvk.Kind).To(Equal("DeviceClass"))
				dc, ok := runtimeObj.(*resourcev1.DeviceClass)
				Expect(ok).To(BeTrue())
				names = append(names, dc.Name)
			}
			return names
		}

		It("can handle ResourceClaimTemplate object", func() {
			runtimeObj, gvk, err := DecodeFromFile(StateScheme,
				filepath.Join(cardmgmtDir, "0550_resourceclaimtemplate_managed.yaml"))
			Expect(err).To(BeNil())
			Expect(gvk.Kind).To(Equal("ResourceClaimTemplate"))

			defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())
			controlledObj, err := NewResourceClaimTemplate(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			Expect(controlledObj).NotTo(BeNil())
			Expect(controlledObj.GetID().Kind).To(Equal("ResourceClaimTemplate"))
			Expect(controlledObj.GetID().Name).To(Equal(spyreconst.CardManagementResourceClaimTemplateName))

			// the placeholder namespace in the asset must be replaced by the operator namespace
			obj, ok := controlledObj.GetObject().(*resourcev1.ResourceClaimTemplate)
			Expect(ok).To(BeTrue())
			Expect(obj.Namespace).To(Equal(OpNs))

			By("checking the PF and VF device requests in the single claim")
			requests := obj.Spec.Spec.Devices.Requests
			Expect(requests).To(HaveLen(2))
			expected := []struct{ request, deviceClass string }{
				{spyreconst.DRADeviceRequestManagedPf, "spyre-managed-pf"},
				{spyreconst.DRADeviceRequestManagedVf, "spyre-managed-vf"},
			}
			for i, e := range expected {
				Expect(requests[i].Name).To(Equal(e.request))
				Expect(requests[i].Exactly).NotTo(BeNil())
				Expect(requests[i].Exactly.DeviceClassName).To(Equal(e.deviceClass))
				// cardmgmt must hold every device of the class, not a single one
				Expect(requests[i].Exactly.AllocationMode).To(Equal(resourcev1.DeviceAllocationModeAll))

				By("checking that the DeviceClass is defined by the DRA driver assets")
				Expect(deviceClassNames()).To(ContainElement(e.deviceClass))
			}
		})

		It("rejects a non-ResourceClaimTemplate object", func() {
			runtimeObj := &resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: "not-a-template"}}
			defaultObj, err := NewDefaultObject(ctx, "DeviceClass", OpNs, runtimeObj)
			Expect(err).To(BeNil())
			_, err = NewResourceClaimTemplate(defaultObj, runtimeObj, OpNs)
			Expect(err).To(MatchError(spyreerr.ErrParseFile))
		})

		It("keeps the PF and VF request names distinct", func() {
			Expect(spyreconst.DRADeviceRequestManagedPf).NotTo(Equal(spyreconst.DRADeviceRequestManagedVf))
		})
	})

	Context("DaemonSet.Ready", func() {
		var controlledObj ControlledObject
		var ds *appsv1.DaemonSet

		BeforeEach(func() {
			runtimeObj, gvk, err := DecodeFromFile(StateScheme, devicePluginDaemonSetPath)
			Expect(err).To(BeNil())
			defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())
			controlledObj, err = NewDaemonSet(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			ds = controlledObj.GetObject().(*appsv1.DaemonSet)
			err = K8sClient.Create(ctx, ds)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			err := K8sClient.Delete(ctx, ds)
			Expect(err).To(BeNil())
		})

		DescribeTable("returns correct readiness",
			func(observedGeneration int64, desired, ready, unavailable, updated int32, expectedReady bool) {
				By("setting daemonset status")
				ds.Status.ObservedGeneration = observedGeneration
				ds.Status.DesiredNumberScheduled = desired
				ds.Status.NumberReady = ready
				ds.Status.NumberUnavailable = unavailable
				ds.Status.UpdatedNumberScheduled = updated
				err := K8sClient.Status().Update(ctx, ds)
				Expect(err).To(BeNil())
				Expect(controlledObj.Ready(ctx, K8sClient)).To(Equal(expectedReady))
			},
			Entry("not ready when ObservedGeneration lags Generation",
				int64(0), int32(2), int32(2), int32(0), int32(2), false),
			Entry("not ready when NumberReady is zero",
				int64(1), int32(2), int32(0), int32(0), int32(2), false),
			Entry("not ready when NumberUnavailable is greater than zero",
				int64(1), int32(2), int32(2), int32(1), int32(2), false),
			Entry("not ready when UpdatedNumberScheduled does not equal DesiredNumberScheduled",
				int64(1), int32(2), int32(2), int32(0), int32(1), false),
			Entry("ready when all conditions are satisfied",
				int64(1), int32(2), int32(2), int32(0), int32(2), true),
		)
	})

	Context("ValidatingAdmissionPolicy and Binding", func() {
		coreDir := filepath.Join(AssetsPath, "state-core-components", "dra-driver-spyre")

		DescribeTable("can handle admission registration objects",
			func(fileName, expectedKind, expectedName string) {
				objPath := filepath.Join(coreDir, fileName)
				runtimeObj, gvk, err := DecodeFromFile(StateScheme, objPath)
				Expect(err).To(BeNil())
				Expect(gvk.Kind).To(Equal(expectedKind))

				controlledObj, err := NewControlledObject(ctx, gvk.Kind, OpNs, runtimeObj)
				Expect(err).To(BeNil())
				Expect(controlledObj).NotTo(BeNil())
				Expect(controlledObj.GetID().Kind).To(Equal(expectedKind))
				Expect(controlledObj.GetID().Name).To(Equal(expectedName))
				Expect(controlledObj.Ready(ctx, K8sClient)).To(BeTrue())

				switch expectedKind {
				case "ValidatingAdmissionPolicy":
					policy, ok := controlledObj.GetObject().(*admissionv1.ValidatingAdmissionPolicy)
					Expect(ok).To(BeTrue())
					Expect(policy.Spec.MatchConstraints.ResourceRules).NotTo(BeEmpty())
					Expect(policy.Spec.Validations).NotTo(BeEmpty())
				case "ValidatingAdmissionPolicyBinding":
					binding, ok := controlledObj.GetObject().(*admissionv1.ValidatingAdmissionPolicyBinding)
					Expect(ok).To(BeTrue())
					Expect(binding.Spec.PolicyName).To(Equal("block-restricted-device-classes"))
					Expect(binding.Spec.ValidationActions).To(ContainElement(admissionv1.Deny))
					Expect(binding.Spec.MatchResources.NamespaceSelector.MatchExpressions[0].Values).To(ContainElement(OpNs))
				}
			},
			Entry("ValidatingAdmissionPolicy", "0530_validatingadmissionpolicy.yaml",
				"ValidatingAdmissionPolicy", "block-restricted-device-classes"),
			Entry("ValidatingAdmissionPolicyBinding", "0540_validatingadmissionpolicybinding.yaml",
				"ValidatingAdmissionPolicyBinding", "block-restricted-device-classes-binding"),
		)

	})

})

func checkModeEnable(ds *appsv1.DaemonSet, mode spyrev1alpha1.SpyreClusterPolicyExperimentalMode, expectedFound bool, expectedValue string) {
	expectedKey := mode.EnvKey()
	found := false
	var value string
	for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
		if env.Name == expectedKey {
			found = true
			value = env.Value
			break
		}
	}
	Expect(found).To(Equal(expectedFound))
	Expect(value).To(BeEquivalentTo(expectedValue))
}
