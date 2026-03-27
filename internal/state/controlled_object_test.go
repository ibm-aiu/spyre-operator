/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
		defaultRunnerImage := "docker.io/spyre-operator/x86_64/release/dd2/spyredriver-image:latest"

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
