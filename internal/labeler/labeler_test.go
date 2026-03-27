/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package labeler_test

import (
	"context"
	"strconv"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	. "github.com/ibm-aiu/spyre-operator/internal/labeler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Labeler", func() {
	ctx := context.Background()

	DescribeTable("HasNFDLabels", func(labels map[string]string, expected bool) {
		has := HasNFDLabels(labels)
		Expect(has).To(Equal(expected))
	},
		Entry("empty", map[string]string{}, false),
		Entry("valid", map[string]string{NfdLabelPrefix + "aaa": "true"}, true),
		Entry("invalid", map[string]string{"something/else": "true"}, false),
	)

	DescribeTable("HasCommonSpyreLabel", func(labels map[string]string, expected bool) {
		has := HasCommonSpyreLabel(labels)
		Expect(has).To(Equal(expected))
	},
		Entry("empty", map[string]string{}, false),
		Entry("valid", map[string]string{spyreconst.CommonSpyreLabelKey: "true"}, true),
		Entry("invalid key", map[string]string{"ibm.com/spyre.present.fake": "true"}, false),
		Entry("invalid value", map[string]string{spyreconst.CommonSpyreLabelKey: "false"}, false),
	)

	DescribeTable("HasSpyreDeviceLabels", func(labels map[string]string, expected bool) {
		has := HasSpyreDeviceLabels(labels)
		Expect(has).To(Equal(expected))
	},
		Entry("empty", map[string]string{}, false),
		Entry("valid pci-1014", map[string]string{"feature.node.kubernetes.io/pci-1014.present": "true"}, true),
		Entry("valid pci-06e7_1014.present", map[string]string{"feature.node.kubernetes.io/pci-06e7_1014.present": "true"}, true),
		Entry("invalid key", map[string]string{"feature.node.kubernetes.io/fake.present": "true"}, false),
		Entry("invalid value", map[string]string{"feature.node.kubernetes.io/pci-1014.present": "false"}, false),
	)

	DescribeTable("UpdateCommonSpyreLabel", func(pseudoMode bool, hasNFD bool, hasSpyre bool, hasSpyreCommonLabel bool,
		expectedChange bool, expectedLabel bool) {
		// generate node labels
		nodeLabels := make(map[string]string)
		if hasNFD {
			nodeLabels["feature.node.kubernetes.io/any"] = "true"
		}
		if hasSpyre {
			nodeLabels["feature.node.kubernetes.io/pci-1014.present"] = "true"
		}
		if hasSpyreCommonLabel {
			nodeLabels[spyreconst.CommonSpyreLabelKey] = "true"
		}
		changed := UpdateCommonSpyreLabel(pseudoMode, nodeLabels)
		Expect(changed).To(Equal(expectedChange))
		val, ok := nodeLabels[spyreconst.CommonSpyreLabelKey]
		if expectedLabel {
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(CommonSpyreLabelValue))
		} else if ok {
			Expect(val).ShouldNot(Equal(CommonSpyreLabelValue))
		}
	},
		Entry("New Spyre node: hasNFD/hasSpyre", false, true, true, false, true, true),
		Entry("Existing Spyre node: hasNFD/hasSpyre/hasSpyreCommonLabel", false, true, true, true, false, true),
		Entry("New pseudo-Spyre: hasNFD", true, true, false, false, true, true),
		Entry("Existing pseudo-Spyre: hasNFD/hasSpyreCommonLabel", true, true, false, true, false, true),
		Entry("Non-Spyre node: hasNFD", false, true, false, false, false, false),
		Entry("Non-Spyre node with previous set: hasNFD/hasSpyreCommonLabel", false, true, false, true, true, false),
	)

	DescribeTable("UpdateDeviceCountProductName", func(capacityCount int, labels map[string]string,
		expectedChange bool, expectedCount int) {
		capacity := make(corev1.ResourceList)
		if capacityCount >= 0 {
			capacity[corev1.ResourceName("ibm.com/spyre_pf")] = *resource.NewQuantity(int64(capacityCount), resource.DecimalSI)
		}
		changed := UpdateDeviceCountProductName(capacity, labels)
		Expect(changed).To(Equal(expectedChange))
		count, countFound := labels[spyreconst.SpyreCountLabelKey]
		product, productFound := labels[spyreconst.SpyreProductLabelKey]
		if expectedCount < 0 {
			Expect(countFound).To(BeFalse())
			Expect(productFound).To(BeFalse())
		} else {
			Expect(countFound).To(BeTrue())
			Expect(count).To(BeEquivalentTo(strconv.Itoa(expectedCount)))
			Expect(productFound).To(BeTrue())
			Expect(product).To(BeEquivalentTo(ProductName))
		}

	},
		Entry("no labels: no capacity", -1, make(map[string]string), false, -1),
		Entry("no labels: has zero capacity", 0, make(map[string]string), true, -1),
		Entry("no labels: has one capacity", 1, make(map[string]string), true, 1),
		Entry("existing match count labels: one capacity", 1,
			map[string]string{spyreconst.SpyreCountLabelKey: "1", spyreconst.SpyreProductLabelKey: ProductName}, false, 1),
		Entry("existing unmatch count labels: one capacity", 1,
			map[string]string{spyreconst.SpyreCountLabelKey: "2", spyreconst.SpyreProductLabelKey: ProductName}, true, 1),
		Entry("existing count labels: no capacity", -1,
			map[string]string{spyreconst.SpyreCountLabelKey: "1", spyreconst.SpyreProductLabelKey: ProductName}, true, -1),
	)

	DescribeTable("LabelSpyreNodes", Ordered, func(pseudoMode bool, labels map[string]string, expectedLabels map[string]string, expectedHasNFD bool, expectedNodeArchitecture string, expectedError error) {
		labeler := Labeler{}
		By("preparing node")
		nodeName := "labeler-test-node"
		_, err := K8sClientset.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nodeName,
				Labels: labels,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		By("labeling node")
		hasNFD, nodeArchitecture, err := labeler.LabelSpyreNodes(ctx, K8sClient, pseudoMode)
		Expect(nodeArchitecture).To(BeEquivalentTo(expectedNodeArchitecture))
		Expect(hasNFD).To(BeEquivalentTo(expectedHasNFD))
		if expectedError != nil {
			Expect(err.Error()).To(Equal(expectedError.Error()))
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
		if nodeArchitecture != "" {
			node, err := K8sClientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			for k, expected := range expectedLabels {
				v, found := node.Labels[k]
				Expect(found).To(BeTrue())
				Expect(v).To(BeEquivalentTo(expected))
			}
		}
		By("deleting node")
		err = K8sClientset.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	},
		Entry("pseudoMode", true, map[string]string{"kubernetes.io/arch": "amd64"},
			map[string]string{spyreconst.CommonSpyreLabelKey: "true"}, false, "amd64", nil),
		Entry("pseudoMode with an NFD label", true, map[string]string{"kubernetes.io/arch": "amd64", "feature.node.kubernetes.io/pci-xxx.present": "true"},
			map[string]string{spyreconst.CommonSpyreLabelKey: "true"}, true, "amd64", nil),
		Entry("amd64", false, map[string]string{"kubernetes.io/arch": "amd64", "feature.node.kubernetes.io/pci-06e7_1014.present": "true"},
			map[string]string{spyreconst.CommonSpyreLabelKey: "true"}, true, "amd64", nil),
		Entry("non-amd64", false, map[string]string{"kubernetes.io/arch": "s390x", "feature.node.kubernetes.io/pci-06e7_1014.present": "true"},
			map[string]string{spyreconst.CommonSpyreLabelKey: "true"}, true, "s390x", nil),
	)

	DescribeTable("GetClusterSpyreLabelInfo", Ordered, func(labels map[string]string, expectedHasNFD bool, expectedArch string) {
		l := Labeler{}
		nodeName := "get-info-test-node"
		_, err := K8sClientset.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: labels},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		hasNFD, arch, err := l.GetClusterSpyreLabelInfo(ctx, K8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasNFD).To(Equal(expectedHasNFD))
		Expect(arch).To(Equal(expectedArch))
		Expect(K8sClientset.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})).To(Succeed())
	},
		Entry("no NFD labels", map[string]string{"kubernetes.io/arch": "amd64"}, false, ""),
		Entry("has NFD + spyre label → returns arch", map[string]string{
			"kubernetes.io/arch":                          "amd64",
			"feature.node.kubernetes.io/pci-1014.present": "true",
			spyreconst.CommonSpyreLabelKey:                "true",
		}, true, "amd64"),
		Entry("has NFD but no spyre common label → no arch", map[string]string{
			"feature.node.kubernetes.io/pci-1014.present": "true",
		}, true, ""),
	)
})
