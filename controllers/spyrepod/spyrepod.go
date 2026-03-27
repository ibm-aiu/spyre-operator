/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package spyrepod

import (
	"strings"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	corev1 "k8s.io/api/core/v1"
)

// IsSpyrePod returns true if the given Pod requests Spyre.
func IsSpyrePod(p *corev1.Pod) bool {
	return GetSpyreResourceName(*p) != ""
}

// IsObsoletePod returns true if the given Pod requests Setient.
func IsObsoletePod(p *corev1.Pod) bool {
	for _, c := range p.Spec.Containers {
		for k := range c.Resources.Requests {
			if (strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.ObsoletePfResourceName)) ||
				(strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.ObsoleteVfResourceName)) {
				return true
			}
		}
		for k := range c.Resources.Limits {
			if (strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.ObsoletePfResourceName)) ||
				(strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.ObsoleteVfResourceName)) {
				return true
			}
		}
	}
	return false
}

// SafePciAddress converts raw PCI address (e.g., "0000:0a.0") to safe address (e.g., "0000_0a.0") which can
// be embedded in a Pod manifest
func SafePciAddress(resourceName, pciAddr string) string {
	return resourceName + "_" + strings.ReplaceAll(pciAddr, ":", "_")
}

func GetSpyreResourceName(p corev1.Pod) string {
	for _, c := range p.Spec.Containers {
		for k := range c.Resources.Requests {
			if (strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.PfResourceName)) ||
				(strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.VfResourceName)) {
				return string(k)
			}
		}
		for k := range c.Resources.Limits {
			if (strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.PfResourceName)) ||
				(strings.HasPrefix(string(k), spyreconst.ResourcePrefix+"/"+spyreconst.VfResourceName)) {
				return string(k)
			}
		}
	}
	return ""
}

// IsPerDevice checks whether resource name is per-device or not
func IsPerDevice(resourceName string) bool {
	return strings.Contains(resourceName, "spyre_pf_") && !strings.Contains(resourceName, "tier")
}

// IsTopologyAware checks whether resource name is topology aware or not
func IsTopologyAware(resourceName string) bool {
	return strings.Contains(resourceName, "tier")
}
