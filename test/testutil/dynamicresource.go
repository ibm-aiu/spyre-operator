/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"fmt"
	"os"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
)

func CreateResource(ctx context.Context, dynClient dynamic.Interface, namespace string, mapObj map[string]interface{}, resourceName string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{Object: mapObj}
	gvr, _ := schema.ParseResourceArg(resourceName)
	object, err := dynClient.Resource(*gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create resource %s in namespace %s: %w", resourceName, namespace, err)
	}
	return object, nil
}

func DeleteResource(ctx context.Context, dynClient dynamic.Interface, namespace, name string, resourceName string) error {
	gvr, _ := schema.ParseResourceArg(resourceName)
	err := dynClient.Resource(*gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete resource %s in namespace %s: %w", resourceName, namespace, err)
	}

	// Wait for resource to be deleted
	Eventually(func(g Gomega) {
		_, err := dynClient.Resource(*gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		g.Expect(errors.IsNotFound(err)).To(BeTrue())
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

	return nil
}

func GetResource(ctx context.Context, dynClient dynamic.Interface, namespace, name string, resourceName string) (*unstructured.Unstructured, error) {
	gvr, _ := schema.ParseResourceArg(resourceName)
	var object *unstructured.Unstructured
	var err error
	if namespace != "" {
		object, err = dynClient.Resource(*gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get resource %s in namespace %s: %w", resourceName, namespace, err)
		}
	} else {
		object, err = dynClient.Resource(*gvr).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get resource %s: %w", resourceName, err)
		}
	}
	return object, nil
}

func ListCustomResource(ctx context.Context, dynClient dynamic.Interface, namespace, resourceName string) ([]string, error) {
	gvr, _ := schema.ParseResourceArg(resourceName)
	list, err := dynClient.Resource(*gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", resourceName, err)
	}
	names := []string{}
	for _, item := range list.Items {
		name := item.GetName()
		names = append(names, name)
	}
	return names, nil
}

func DecodeYAML(ymlfile string) (*unstructured.Unstructured, *schema.GroupVersionKind, error) {
	var decoder = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	yml, err := os.ReadFile(ymlfile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read the yml file %s: %w ", ymlfile, err)
	}
	_, gvk, err := decoder.Decode(yml, nil, obj)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode the content into an object: %w", err)
	}
	return obj, gvk, nil
}

func GvrMap(discoClient *discovery.DiscoveryClient, gvk *schema.GroupVersionKind) (*meta.RESTMapping, error) {
	// Check server has a matching gvr given a gvk
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoClient))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find a gvr for the gvk %v: %w", gvk, err)
	}
	return mapping, nil
}

func CreateResourceFromYaml(ctx context.Context, dynClient *dynamic.DynamicClient, discoClient *discovery.DiscoveryClient, ns string, ymlfile string) (*unstructured.Unstructured, error) {
	obj, gvk, err := DecodeYAML(ymlfile)
	if err != nil {
		return nil, err
	}
	rm, err := GvrMap(discoClient, gvk)
	if err != nil {
		return nil, err
	}
	object, err := dynClient.Resource(rm.Resource).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create resource %v in namespace %s: %w", rm.Resource, ns, err)
	}
	return object, nil
}

func DeleteNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespaceName string) error {
	err := clientset.CoreV1().Namespaces().Delete(ctx, namespaceName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete the project %s: %w", namespaceName, err)
	}
	return nil
}

func CreateNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespaceName string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				// Otherwise when creating Pod w/o specify SCC would warn
				// violate PodSecurity "restricted:v1.24": allowPrivilegeEscalation != false (container "app" must set securityContext.allowPrivilegeEscalation=false),
				// unrestricted capabilities (container "app" must set securityContext.capabilities.drop=["ALL"]),
				// runAsNonRoot != true (pod or container "app" must set securityContext.runAsNonRoot=true),
				// seccompProfile (pod or container "app" must set securityContext.seccompProfile.type to "RuntimeDefault" or "Localhost")
				"pod-security.kubernetes.io/enforce": "privileged",
				"pod-security.kubernetes.io/audit":   "privileged",
				"pod-security.kubernetes.io/warn":    "privileged",
			},
		},
	}
	_, err := clientset.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create the namespace %s: %w", namespace, err)
	}
	return nil
}

// Return the number of spyre devices allocated to a namespaced pod in an SpyreNodeState or 0 if the Pod is not found
func NumDeviceSpyrensForPod(podname string, ns string, spyrensstatus spyrev1alpha1.SpyreNodeStateStatus) int {
	for _, i := range spyrensstatus.AllocationList {
		if i.Pod.Name == podname && i.Pod.Namespace == ns {
			return len(i.DeviceList)
		}
	}
	return 0
}
