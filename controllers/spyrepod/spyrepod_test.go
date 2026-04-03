/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */
package spyrepod_test

import (
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/controllers/spyrepod"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func res(m map[string]string) map[corev1.ResourceName]resource.Quantity {
	res := map[corev1.ResourceName]resource.Quantity{}
	for k, v := range m {
		q, err := resource.ParseQuantity(v)
		if err != nil {
			panic("failed to parse quantity")
		}
		res[corev1.ResourceName(k)] = q
	}
	return res
}

var _ = Describe("Spyrepod", func() {

	pf := spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName
	vf := spyreconst.ResourcePrefix + "/" + spyreconst.VfResourceName
	vfDevId := vf + "_0000_0a"

	DescribeTable("GetSpyreResourceName and IsSpyrePod", func(ann []string, req map[corev1.ResourceName]resource.Quantity, lim map[corev1.ResourceName]resource.Quantity, expectedResourceName string, expectedSpyrePod bool) {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:    "container",
						Image:   "image",
						Command: []string{"run.sh"},
						Resources: corev1.ResourceRequirements{
							Limits:   lim,
							Requests: req,
						},
					},
				},
			},
		}
		resourceName := spyrepod.GetSpyreResourceName(*p)
		Expect(resourceName).To(Equal(expectedResourceName))
		Expect(spyrepod.IsSpyrePod(p)).Should(Equal(expectedSpyrePod))
	},
		Entry("nothing", nil, nil, nil, "", false),
		Entry("non-spyre in lim", nil, nil, res(map[string]string{"foo": "1"}), "", false),
		Entry("non-spyre in req", nil, res(map[string]string{"foo": "1"}), nil, "", false),
		Entry("non-spyre in both", nil, res(map[string]string{"foo": "1"}), res(map[string]string{"foo": "1"}), "", false),
		Entry("spyre in lim", nil, nil, res(map[string]string{pf: "1"}), pf, true),
		Entry("spyre in req", nil, res(map[string]string{pf: "1"}), nil, pf, true),
		Entry("spyre (vf) in req", nil, res(map[string]string{vf: "1"}), nil, vf, true),
		Entry("spyre (pf + devId) in req", nil, res(map[string]string{vfDevId: "1"}), nil, vfDevId, true),
		Entry("spyre in both", nil, res(map[string]string{pf: "1"}), res(map[string]string{pf: "1"}), pf, true),
		Entry("spyre in req and non-spyre in lim", nil, res(map[string]string{pf: "1"}), res(map[string]string{"foo": "1"}), pf, true),
		Entry("spyre in lim and non-spyre in req", nil, res(map[string]string{"foo": "1"}), res(map[string]string{pf: "1"}), pf, true),
		Entry("both in both", nil, res(map[string]string{"foo": "1", pf: "1"}), res(map[string]string{"foo": "1", pf: "1"}), pf, true),
	)

	DescribeTable("PCI address handling", func(s, addr, safeAddr string) {
		Expect(spyrepod.SafePciAddress(s, addr)).Should(Equal(safeAddr))
	},
		Entry("numeric address", "prefix", "0000:39:00.0", "prefix_0000_39_00.0"),
		Entry("numeric + char address", "prefix", "0000:3d:00.0", "prefix_0000_3d_00.0"),
	)

	DescribeTable("IsPerDevice", func(resourceName string, expectedResult bool) {
		Expect(spyrepod.IsPerDevice(resourceName)).To(Equal(expectedResult))
	},
		Entry("per-device", "ibm.com/spyre_pf_0000_1a_00.0", true),
		Entry("pf", pf, false),
		Entry("tier", "ibm.com/spyre_pf_tier0", false),
		Entry("irrelevant", "cpu", false),
	)

	DescribeTable("IsTopologyAware", func(resourceName string, expectedResult bool) {
		Expect(spyrepod.IsTopologyAware(resourceName)).To(Equal(expectedResult))
	},
		Entry("per-device", "ibm.com/spyre_pf_0000_1a_00.0", false),
		Entry("pf", pf, false),
		Entry("tier0", "ibm.com/spyre_pf_tier0", true),
		Entry("tier1", "ibm.com/spyre_pf_tier1", true),
		Entry("tier2", "ibm.com/spyre_pf_tier2", true),
		Entry("irrelevant", "cpu", false),
	)
})
