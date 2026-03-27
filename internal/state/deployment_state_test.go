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

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	. "github.com/ibm-aiu/spyre-operator/internal/state"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	secondschedv1 "github.com/openshift/secondary-scheduler-operator/pkg/apis/secondaryscheduler/v1"
)

var _ = Describe("DeploymentState", Ordered, func() {
	ctx := context.Background()
	cpName := "deployment-state-test-policy"
	BeforeEach(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName}}
		err := K8sClient.Create(ctx, cp)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName}}
		err := K8sClient.Delete(ctx, cp)
		Expect(err).To(BeNil())
	})

	Context("state-init", func() {
		var deploymentState *DeploymentState
		ctx := context.Background()

		BeforeEach(func() {
			deploymentState = initDeploymentState(ctx, "state-init",
				[]string{"common", "spyre-webhook-validator", "spyre-health-checker"})
		})

		DescribeTable("Transform", func(enabledPodValidator bool, enabledHealthChecker bool) {
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err := K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			deploymentConfig := ValidDeploymentConfig("validator")
			healthCheckerConfig := ValidDeploymentConfig("health-checker")
			cp.Spec = spyrev1alpha1.SpyreClusterPolicySpec{
				PodValidator: spyrev1alpha1.PodValidatorSpec{
					DeploymentConfig: deploymentConfig,
				},
				HealthChecker: spyrev1alpha1.HealthCheckerSpec{
					DeploymentConfig: healthCheckerConfig,
				},
			}
			Expect(err).To(BeNil())
			cp.Spec.PodValidator.Enabled = enabledPodValidator
			cp.Spec.HealthChecker.Enabled = enabledHealthChecker
			err = deploymentState.Transform(cp, Cluster)
			Expect(err).To(BeNil())

			By("checking pod validator")
			found, disabled := deploymentState.IsDisabled(spyreconst.ValidatorResourceName)
			Expect(found).To(BeTrue())
			Expect(disabled).To(Equal(!enabledPodValidator))

			podValidatorDeploymentID := ControlledID{
				Name:      spyreconst.ValidatorResourceName,
				Namespace: OpNs,
				Kind:      "Deployment",
			}
			obj := deploymentState.GetObject(spyreconst.ValidatorResourceName, podValidatorDeploymentID)
			runtimeObj := obj.GetObject()
			deploy, ok := runtimeObj.(*appsv1.Deployment)
			Expect(ok).To(BeTrue())
			Expect(deploy.Namespace).To(Equal(OpNs))
			if enabledPodValidator {
				checkValidImage(deploy.Spec.Template.Spec, deploymentConfig)
				checkOwner(deploy.ObjectMeta, cpName)
			} else {
				Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo(spyreconst.OPERATOR_FILLED))
			}

			By("checking health checker")
			found, disabled = deploymentState.IsDisabled(spyreconst.HealthCheckerResourceName)
			Expect(found).To(BeTrue())
			Expect(disabled).To(Equal(!enabledHealthChecker))
			checkerDeploymentID := ControlledID{
				Name:      spyreconst.HealthCheckerResourceName,
				Namespace: OpNs,
				Kind:      "DaemonSet",
			}
			obj = deploymentState.GetObject(spyreconst.HealthCheckerResourceName, checkerDeploymentID)
			dsRuntimeObj := obj.GetObject()
			ds, ok := dsRuntimeObj.(*appsv1.DaemonSet)
			Expect(ok).To(BeTrue())
			Expect(ds.Namespace).To(Equal(OpNs))
			if enabledHealthChecker {
				checkValidImage(ds.Spec.Template.Spec, healthCheckerConfig)
				checkOwner(ds.ObjectMeta, cpName)
			} else {
				Expect(ds.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo(spyreconst.OPERATOR_FILLED))
			}
		},
			Entry("validator/health checker both disabled", false, false),
			Entry("only validator enabled", true, false),
			Entry("only health checker enabled", false, true),
			Entry("validator/health checker both enabled", true, true),
		)
	})

	Context("state-core-components", func() {
		var deploymentState *DeploymentState
		ctx := context.Background()

		BeforeEach(func() {
			deploymentState = initDeploymentState(ctx, "state-core-components",
				[]string{"spyre-device-plugin"})
		})

		It("transforms device plugin deployments by SpyreClusterPolicy", func() {
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err := K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			devicePluginDeploymentConfig := ValidDeploymentConfig("device-plugin")
			cp.Spec = spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					DeploymentConfig: devicePluginDeploymentConfig,
				},
			}
			Expect(err).To(BeNil())
			err = deploymentState.Transform(cp, Cluster)
			Expect(err).To(BeNil())
			By("checking device plugin")
			devicePluginDaemonSetID := ControlledID{
				Name:      spyreconst.DevicePluginResourceName,
				Namespace: OpNs,
				Kind:      "DaemonSet",
			}
			obj := deploymentState.GetObject(spyreconst.DevicePluginResourceName, devicePluginDaemonSetID)
			runtimeObj := obj.GetObject()
			deploy, ok := runtimeObj.(*appsv1.DaemonSet)
			Expect(ok).To(BeTrue())
			Expect(deploy.Namespace).To(Equal(OpNs))
			checkValidImage(deploy.Spec.Template.Spec, devicePluginDeploymentConfig)
			checkOwner(deploy.ObjectMeta, cpName)
		})
	})

	Context("state-plugin-components", func() {
		var deploymentState *DeploymentState
		ctx := context.Background()

		BeforeEach(func() {
			deploymentState = initDeploymentState(ctx, "state-plugin-components",
				[]string{"spyre-card-management", "spyre-metrics-exporter", "secondary-scheduler"})
		})

		DescribeTable("Transform", func(enabledMetricsExporter, enabledScheduler bool) {
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err := K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			deploymentConfig := ValidDeploymentConfig("metrics-exporter")
			schedulerConfig := ValidDeploymentConfig("scheduler")
			cp.Spec = spyrev1alpha1.SpyreClusterPolicySpec{
				MetricsExporter: spyrev1alpha1.MetricsExporterSpec{
					DeploymentConfig: deploymentConfig,
				},
				Scheduler: spyrev1alpha1.SchedulerSpec{
					DeploymentConfig: schedulerConfig,
				},
			}
			if enabledScheduler {
				cp.Spec.ExperimentalMode = append(cp.Spec.ExperimentalMode, spyrev1alpha1.ReservationMode)
			}
			Expect(err).To(BeNil())
			cp.Spec.MetricsExporter.Enabled = enabledMetricsExporter
			err = deploymentState.Transform(cp, Cluster)
			Expect(err).To(BeNil())
			By("checking metrics exporter")
			found, disabled := deploymentState.IsDisabled(spyreconst.MonitorResourceName)
			Expect(found).To(BeTrue())
			Expect(disabled).To(Equal(!enabledMetricsExporter))
			monitorDeploymentID := ControlledID{
				Name:      spyreconst.MonitorResourceName,
				Namespace: OpNs,
				Kind:      "DaemonSet",
			}
			obj := deploymentState.GetObject(spyreconst.MonitorResourceName, monitorDeploymentID)
			runtimeObj := obj.GetObject()
			deploy, ok := runtimeObj.(*appsv1.DaemonSet)
			Expect(ok).To(BeTrue())
			Expect(deploy.Namespace).To(Equal(OpNs))
			if enabledMetricsExporter {
				checkValidImage(deploy.Spec.Template.Spec, deploymentConfig)
				checkOwner(deploy.ObjectMeta, cpName)
			} else {
				Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(BeEquivalentTo(spyreconst.OPERATOR_FILLED))
			}
			By("checking scheduler")
			found, disabled = deploymentState.IsDisabled(spyreconst.SchedulerResourceName)
			Expect(found).To(BeTrue())
			Expect(disabled).To(Equal(!enabledScheduler))
			schedulerID := ControlledID{
				Name:      "cluster",
				Namespace: "openshift-secondary-scheduler-operator",
				Kind:      "SecondaryScheduler",
			}
			obj = deploymentState.GetObject(spyreconst.SchedulerResourceName, schedulerID)
			runtimeObj = obj.GetObject()
			scheduler, ok := runtimeObj.(*secondschedv1.SecondaryScheduler)
			Expect(ok).To(BeTrue())
			if enabledScheduler {
				imagePath, err := spyrev1alpha1.ImagePath(schedulerConfig.Repository,
					schedulerConfig.Image, schedulerConfig.Version)
				Expect(err).To(BeNil())
				Expect(scheduler.Spec.SchedulerImage).To(BeEquivalentTo(imagePath))
				checkOwner(scheduler.ObjectMeta, cpName)
			} else {
				Expect(scheduler.Spec.SchedulerImage).To(BeEquivalentTo(spyreconst.OPERATOR_FILLED))
			}
		},
			Entry("exporter disabled/scheduler disabled", false, false),
			Entry("exporter disabled/scheduler enabled", false, true),
			Entry("exporter enabled/scheduler disabled", true, false),
			Entry("exporter enabled/scheduler enabled", true, true),
		)
	})

	Context("all states", func() {
		var deploymentStates []*DeploymentState
		ctx := context.Background()

		BeforeEach(func() {
			deploymentStates = []*DeploymentState{
				initDeploymentState(ctx, "state-init",
					[]string{"common", "spyre-webhook-validator", "spyre-health-checker"}),
				initDeploymentState(ctx, "state-core-components",
					[]string{"spyre-device-plugin"}),
				initDeploymentState(ctx, "state-plugin-components",
					[]string{"spyre-card-management", "spyre-metrics-exporter", "secondary-scheduler"}),
			}
		})

		It("can set skipUpdate to components", func() {
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err := K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			Expect(err).To(BeNil())
			By("checking initial values")
			for _, state := range deploymentStates {
				for _, component := range state.GetComponents() {
					Expect(component.GetSkipUpdate()).To(BeFalse())
				}
			}
			cp.Spec.SkipUpdateComponents = []spyrev1alpha1.Component{
				// all components
				spyrev1alpha1.ComponentCommonInit,
				spyrev1alpha1.ComponentDevicePlugin,
				spyrev1alpha1.ComponentCardManagement,
				spyrev1alpha1.ComponentMetricsExporter,
				spyrev1alpha1.ComponentScheduler,
				spyrev1alpha1.ComponentPodValidator,
				spyrev1alpha1.ComponentHealthChecker,
				// Add new component mapping here
			}
			for _, state := range deploymentStates {
				err = state.Transform(cp, Cluster)
				Expect(err).NotTo(HaveOccurred())
				for _, component := range state.GetComponents() {
					Expect(component.GetSkipUpdate()).To(BeTrue())
				}
			}
		})
	})
})

func initDeploymentState(ctx context.Context, stateName string, expectedComponents []string) *DeploymentState {
	path := filepath.Join(AssetsPath, stateName)
	state, err := NewDeploymentState(ctx, StateClient, StateScheme, path, OpNs)
	Expect(err).To(BeNil())
	Expect(state).NotTo(BeNil())
	Expect(state.GetName()).To(BeEquivalentTo(stateName))
	components := state.GetComponents()
	Expect(components).To(HaveLen(len(expectedComponents)))
	for _, comp := range components {
		Expect(comp.GetName()).To(BeElementOf(expectedComponents))
	}
	return state
}

func checkValidImage(podSpec corev1.PodSpec, deploymentConfig spyrev1alpha1.DeploymentConfig) {
	imagePath, err := spyrev1alpha1.ImagePath(deploymentConfig.Repository,
		deploymentConfig.Image, deploymentConfig.Version)
	Expect(err).To(BeNil())
	Expect(podSpec.Containers[0].Image).To(BeEquivalentTo(imagePath))
}

func checkOwner(meta metav1.ObjectMeta, cpName string) {
	references := meta.GetOwnerReferences()
	Expect(references).To(HaveLen(1))
	Expect(references[0].Name).To(BeEquivalentTo(cpName))
	Expect(references[0].Kind).To(BeEquivalentTo("SpyreClusterPolicy"))
}
