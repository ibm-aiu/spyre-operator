// Copyright 2026 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// resourceclaim.go defines functions to handle ResourceClaim and ResourceClaimTemplate resource
// and their corresponding variables and constants

package testutil

import (
	"context"

	. "github.com/onsi/gomega"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	numaAttribute = "spyre.ibm.com/numaInfo"
	PfProductId   = "06a7"
	VfProductId   = "06a8"

	pfDeviceClassName           = "spyre-pf"
	vfDeviceClassName           = "spyre-standard-vf"
	vfPrivilegedDeviceClassName = "spyre-privileged-vf"
)

const ResourceClaimTemplate = `
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
spec:
  spec:
    devices:
      requests:
      - name: spyre
        exactly:
          deviceClassName: {{ .DeviceClassName }}
          {{- if gt .Count 0 }}
          count: {{ .Count }}
          {{- end}}
          {{- if .PCIAddress }}
          selectors:
          - cel:
              expression: |-
                device.attributes["spyre.ibm.com"].pciAddress == "{{ .PCIAddress }}"
          {{- else if .ProductId }}
          selectors:
          - cel:
              expression: |-
                device.attributes["spyre.ibm.com"].productId == "{{ .ProductId }}"
          {{- end}}
      {{- if .MatchAttribute }}
      constraints:
      - requests: ["spyre"]
        matchAttribute: {{ .MatchAttribute }}
      {{- end}}
`

// ResourceClaimTemplateData holds data used to populate a Kubernetes ResourceClaimTemplate template.
//
// Example usage:
//
//	data := BasicResourceClaimTemplateData(name, namespace)
//	data = data.SetCount(image) // default: not set (DRA default: 1)
//	data = data.SetPCIAddressSelector(pciAddress) // default: not set
//	data = data.SetProductId(productId)   // default: not set
//	data = data.SetMatchAttribute(matchAttribute) // default: not set
//
// pciAddress and productId cannot be applied at the same time.
// If both specified, pciAddress will be used.
type ResourceClaimTemplateData struct {
	Name            string
	Namespace       string
	Count           int
	PCIAddress      string
	ProductId       string
	MatchAttribute  string
	DeviceClassName string
}

// BasicResourceClaimTemplateData init data
func BasicResourceClaimTemplateData(name, namespace string) *ResourceClaimTemplateData {
	return &ResourceClaimTemplateData{
		Name:      name,
		Namespace: namespace,
	}
}

// SetCount sets count for ExactCount which is default allocation mode
func (c *ResourceClaimTemplateData) SetCount(count int) *ResourceClaimTemplateData {
	c.Count = count
	return c
}

// SetPCIAddressSelector sets PCI address in selector
func (c *ResourceClaimTemplateData) SetPCIAddressSelector(pciAddress string) *ResourceClaimTemplateData {
	c.PCIAddress = pciAddress
	return c
}

// SetProductId sets productId in selector
func (c *ResourceClaimTemplateData) SetProductId(productId string) *ResourceClaimTemplateData {
	c.ProductId = productId
	return c
}

// DeviceClassNameForDevice returns the DeviceClass name (spyre-pf,
// spyre-privileged-vf, or spyre-standard-vf) matching the given productId
// and pciFunctionIndex. pciFunctionIndex is only consulted for VF devices,
// since privileged VFs (pciFunctionIndex == 1) require spyre-privileged-vf
// rather than spyre-standard-vf.
func DeviceClassNameForDevice(productId string, pciFunctionIndex int64) string {
	if productId != VfProductId {
		return pfDeviceClassName
	}
	if pciFunctionIndex == 1 {
		return vfPrivilegedDeviceClassName
	}
	return vfDeviceClassName
}

// SetMatchAttribute sets an attribute to match in constraints
func (c *ResourceClaimTemplateData) SetMatchAttribute(matchAttribute string) *ResourceClaimTemplateData {
	c.MatchAttribute = matchAttribute
	return c
}

// BuildResourceClaimTemplate creates ResourceClaimTemplate according to template data
// Always check nil error.
func BuildResourceClaimTemplate(ctx context.Context, dynClient *dynamic.DynamicClient, discoClient *discovery.DiscoveryClient,
	data *ResourceClaimTemplateData) {
	yamlData := YamlFromTemplate(ResourceClaimTemplate, *data)
	_, err := CreateResourceFromYaml(ctx, dynClient, discoClient, data.Namespace, yamlData)
	Expect(err).To(BeNil())
}

// BuildTopologyAwareResourceClaimTemplate creates ResourceClaimTemplate
// with numa match for specific number of devices with specific productId (PF or VF)
func BuildTopologyAwareResourceClaimTemplate(ctx context.Context, dynClient *dynamic.DynamicClient, discoClient *discovery.DiscoveryClient,
	name, namespace string, count int, productId string) {
	data := BasicResourceClaimTemplateData(name, namespace).SetCount(count).SetMatchAttribute(numaAttribute).SetProductId(productId)
	BuildResourceClaimTemplate(ctx, dynClient, discoClient, data)
}

// BuildPCISpecificResourceClaimTemplate creates ResourceClaimTemplate with specific pci address
func BuildPCISpecificResourceClaimTemplate(ctx context.Context, dynClient *dynamic.DynamicClient, discoClient *discovery.DiscoveryClient,
	name, namespace string, pciAddress string) {
	data := BasicResourceClaimTemplateData(name, namespace).SetPCIAddressSelector(pciAddress)
	BuildResourceClaimTemplate(ctx, dynClient, discoClient, data)
}

func ListResourceSlices(ctx context.Context, k8sClientset *kubernetes.Clientset) []resourcev1.ResourceSlice {
	slices, err := k8sClientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())
	return slices.Items
}

func ListResourceClaims(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) []resourcev1.ResourceClaim {
	claims, err := k8sClientset.ResourceV1().ResourceClaims(namespace).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())
	return claims.Items
}

// BuildPodWithClaim creates a pod with common command to print senlib config
// Always check nil error.
func BuildPodWithClaim(ctx context.Context, dynClient *dynamic.DynamicClient, discoClient *discovery.DiscoveryClient,
	data *PodTemplateData, claimName string) {
	buildPodWithClaim(ctx, dynClient, discoClient, PodWithResourceClaimTemplate, data, claimName)
}

// buildPodWithClaim creates a pod with the defined Arg0.
// If Arg0 is "", it sets the command to ["tail", "-f", "/dev/null"].
// Always check nil error.
func buildPodWithClaim(ctx context.Context, dynClient *dynamic.DynamicClient, discoClient *discovery.DiscoveryClient,
	template string, data *PodTemplateData, claimName string) {
	var yamlData string
	if claimName == "" {
		yamlData = YamlFromTemplate(template, *data)
	} else {
		dataWithClaim := PodWithResourceClaimTemplateData{
			PodTemplateData:           *data,
			ResourceClaimTemplateName: claimName,
		}
		yamlData = YamlFromTemplate(template, dataWithClaim)
	}
	_, err := CreateResourceFromYaml(ctx, dynClient, discoClient, data.Namespace, yamlData)
	Expect(err).To(BeNil())
}

func DeleteResourceClaimTemplate(ctx context.Context, k8sClientset *kubernetes.Clientset, claimName, namespace string) {
	err := k8sClientset.ResourceV1().ResourceClaimTemplates(namespace).Delete(ctx, claimName, metav1.DeleteOptions{})
	if err != nil {
		Expect(errors.IsNotFound(err)).To(BeTrue())
	} else {
		Expect(err).To(BeNil())
	}
}
