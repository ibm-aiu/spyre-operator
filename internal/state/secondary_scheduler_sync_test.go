/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2026 IBM Corp.                                      |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1 "github.com/openshift/api/operator/v1"
	secondschedv1 "github.com/openshift/secondary-scheduler-operator/pkg/apis/secondaryscheduler/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// The SecondaryScheduler is synced against a real API server whose CRD is that of
// Secondary Scheduler Operator 1.6 (with spec.topology, unknown to the vendored types)
// and that of 1.5 or earlier (without it), so that dropping fields on write-back is detected.
var _ = Describe("SecondaryScheduler sync", Ordered, func() {
	const ns = "openshift-secondary-scheduler-operator"
	gvk := secondschedv1.SchemeGroupVersion.WithKind("SecondaryScheduler")

	for _, version := range []string{"v1.6", "v1.5"} {
		hasTopology := version == "v1.6"
		Context("with the CRD of Secondary Scheduler Operator "+version, Ordered, func() {
			var env *envtest.Environment
			var rawClient, stateClient client.Client

			BeforeAll(func() {
				env = &envtest.Environment{
					CRDDirectoryPaths:     []string{filepath.Join("..", "..", "test", "crd", "secsched", version)},
					ErrorIfCRDPathMissing: true,
				}
				cfg, err := env.Start()
				Expect(err).NotTo(HaveOccurred())
				// rawClient knows no scheme, so it reads and writes whole objects as unstructured.
				rawClient, err = client.New(cfg, client.Options{Scheme: runtime.NewScheme()})
				Expect(err).NotTo(HaveOccurred())
				scheme := runtime.NewScheme()
				Expect(secondschedv1.AddToScheme(scheme)).To(Succeed())
				Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
				stateClient, err = client.New(cfg, client.Options{Scheme: scheme})
				Expect(err).NotTo(HaveOccurred())
				nsObj := &unstructured.Unstructured{}
				nsObj.SetAPIVersion("v1")
				nsObj.SetKind("Namespace")
				nsObj.SetName(ns)
				Expect(rawClient.Create(context.Background(), nsObj)).To(Succeed())
			})
			AfterAll(func() { Expect(env.Stop()).To(Succeed()) })

			It("overwrites the managed fields and keeps every other field", func() {
				ctx := context.Background()
				spec := map[string]interface{}{
					"managementState":  "Managed",
					"logLevel":         "Normal",
					"operatorLogLevel": "Normal",
					"schedulerConfig":  "secondary-scheduler-config",
					"schedulerImage":   "quay.io/ibm-aiu/spyre-scheduler:1.4.0",
					"observedConfig": map[string]interface{}{
						"servingInfo": map[string]interface{}{"minTLSVersion": "VersionTLS12"},
					},
				}
				if hasTopology {
					spec["topology"] = map[string]interface{}{"mode": "SingleReplica"}
				}
				live := &unstructured.Unstructured{Object: map[string]interface{}{"spec": spec}}
				live.SetGroupVersionKind(gvk)
				live.SetName("cluster")
				live.SetNamespace(ns)
				Expect(rawClient.Create(ctx, live)).To(Succeed())

				desired := &secondschedv1.SecondaryScheduler{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: ns},
					Spec: secondschedv1.SecondarySchedulerSpec{
						OperatorSpec: operatorv1.OperatorSpec{
							ManagementState: operatorv1.Managed,
							LogLevel:        operatorv1.Debug,
						},
						SchedulerConfig: "secondary-scheduler-config",
						SchedulerImage:  "quay.io/ibm-aiu/spyre-scheduler:1.5.0",
					},
				}
				obj, err := NewControlledObject(ctx, "SecondaryScheduler", ns, desired)
				Expect(err).NotTo(HaveOccurred())
				component := &ControlledComponent{client: stateClient, scheme: stateClient.Scheme()}
				ok, msg, err := component.syncObject(ctx, logr.Discard(), obj)
				Expect(err).NotTo(HaveOccurred(), msg)
				Expect(ok).To(BeTrue())

				got := &unstructured.Unstructured{}
				got.SetGroupVersionKind(gvk)
				Expect(rawClient.Get(ctx, client.ObjectKey{Name: "cluster", Namespace: ns}, got)).To(Succeed())
				str := func(path ...string) string {
					v, _, _ := unstructured.NestedString(got.Object, append([]string{"spec"}, path...)...)
					return v
				}
				Expect(str("schedulerImage")).To(Equal("quay.io/ibm-aiu/spyre-scheduler:1.5.0"))
				Expect(str("logLevel")).To(Equal("Debug"))
				Expect(str("operatorLogLevel")).To(Equal("Normal"))
				Expect(str("observedConfig", "servingInfo", "minTLSVersion")).To(Equal("VersionTLS12"))
				if hasTopology {
					Expect(str("topology", "mode")).To(Equal("SingleReplica"))
				}
			})

			It("is idempotent: syncing twice does not change the object", func() {
				ctx := context.Background()
				key := client.ObjectKey{Name: "cluster", Namespace: ns}
				before := &unstructured.Unstructured{}
				before.SetGroupVersionKind(gvk)
				Expect(rawClient.Get(ctx, key, before)).To(Succeed())

				desired := &secondschedv1.SecondaryScheduler{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: ns},
					Spec: secondschedv1.SecondarySchedulerSpec{
						OperatorSpec:    operatorv1.OperatorSpec{ManagementState: operatorv1.Managed, LogLevel: operatorv1.Debug},
						SchedulerConfig: "secondary-scheduler-config",
						SchedulerImage:  "quay.io/ibm-aiu/spyre-scheduler:1.5.0",
					},
				}
				obj, err := NewControlledObject(ctx, "SecondaryScheduler", ns, desired)
				Expect(err).NotTo(HaveOccurred())
				component := &ControlledComponent{client: stateClient, scheme: stateClient.Scheme()}
				_, _, err = component.syncObject(ctx, logr.Discard(), obj)
				Expect(err).NotTo(HaveOccurred())

				after := &unstructured.Unstructured{}
				after.SetGroupVersionKind(gvk)
				Expect(rawClient.Get(ctx, key, after)).To(Succeed())
				Expect(after.GetResourceVersion()).To(Equal(before.GetResourceVersion()))
			})
		})
	}
})
