/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */
package pcitopov2_test

import (
	"encoding/json"
	"os"

	. "github.com/ibm-aiu/spyre-operator/pkg/types/pcitopov2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	pfTopoFilepath = "../../../test/data/topo-v2/new-topo-pf.json"
	vfTopoFilepath = "../../../test/data/topo-v2/new-topo-vf.json"
)

var _ = Describe("Test Topology", func() {

	Context("topology v2", func() {

		It("can read pcitopo v2 file containing PF devices", func() {
			s, err := os.ReadFile(pfTopoFilepath)
			Expect(err).To(BeNil())
			Expect(s).NotTo(BeNil())
			var pcitopo Pcitopo
			err = json.Unmarshal(s, &pcitopo)
			Expect(err).To(BeNil())
			Expect(pcitopo.Devices).Should(HaveLen(pcitopo.NumDevices))
			for _, v := range pcitopo.Devices {
				Expect(v.Peers.Peer0).ShouldNot(BeEmpty())
			}
		})

		It("can read pcitopo v2 file containing VF devices", func() {
			s, err := os.ReadFile(vfTopoFilepath)
			Expect(err).To(BeNil())
			Expect(s).NotTo(BeNil())
			var pcitopo Pcitopo
			err = json.Unmarshal(s, &pcitopo)
			Expect(err).To(BeNil())
			Expect(pcitopo.SpyreVfDevices).Should(HaveLen(pcitopo.SpyreVfNumDevices))
			for _, v := range pcitopo.SpyreVfDevices {
				Expect(v.SpyreVfPeers.Peer0).ShouldNot(BeEmpty())
			}
		})

	})
})
