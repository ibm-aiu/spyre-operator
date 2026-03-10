/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	imagev1 "github.com/openshift/api/image/v1"
	secv1 "github.com/openshift/api/security/v1"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	secondschedv1 "github.com/openshift/secondary-scheduler-operator/pkg/apis/secondaryscheduler/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

var nfdSchemeGroupVersion = schema.GroupVersion{
	Group:   nfdv1alpha1.GroupVersion.Group,
	Version: nfdv1alpha1.GroupVersion.Version,
}

// filePathWalkDir returns all files which satisfy following conditions:
// 1. under root directory.
// 2. has suffix specified in suffixes
func filePathWalkDir(root string, suffixes []string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			for _, s := range suffixes {
				if strings.HasSuffix(path, s) {
					files = append(files, path)
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		return files, fmt.Errorf("failed to walk file path: %w", err)
	}
	return files, nil
}

func decodeFromFile(scheme *runtime.Scheme, file string) (runtime.Object, *schema.GroupVersionKind, error) {
	decode := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode
	stream, _ := os.ReadFile(file)
	return decode(stream, nil, nil)
}

func encodeToFile(obj runtime.Object, file string) (err error) {
	serializer := json.NewYAMLSerializer(json.DefaultMetaFactory, nil, nil)
	f, err := os.Create(file)
	if err != nil {
		return fmt.Errorf("failed to encode object to file: %w", err)
	}
	defer func() {
		err = f.Close()
	}()
	if err = serializer.Encode(obj, f); err != nil {
		return fmt.Errorf("failed to encode object to file: %w", err)
	}
	return nil
}

func metricsExporterEnabled(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) bool {
	return clusterPolicy.Spec.MetricsExporter.Enabled
}

func InitializeScheme(scheme *runtime.Scheme) error {
	if err := promv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add prometheus operator scheme: %w", err)
	}
	if err := certmanagerv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add cert-manager scheme: %w", err)
	}
	if err := secv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add openshift security scheme: %w", err)
	}
	if err := configv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add openshift config api scheme: %w", err)
	}
	if err := imagev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add openshift api scheme: %w", err)
	}
	if err := nfdv1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add node feature discovery scheme: %w", err)
	}
	if err := secondschedv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add second scheduler operator scheme: %w", err)
	}
	// openshift NFD's addKnownTypes has no NodeFeatureRuleList, need to add this line
	scheme.AddKnownTypes(nfdSchemeGroupVersion, &nfdv1alpha1.NodeFeatureRuleList{})
	return nil
}
