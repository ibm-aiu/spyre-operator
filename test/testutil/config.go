/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"fmt"
	"log"
	"os"
	"strings"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"gopkg.in/yaml.v2"
)

type TestConfig struct {
	Repository       string                 `yaml:"repository"`
	Channel          string                 `yaml:"defaultChannel"`
	Operator         ImageVersion           `yaml:"operator"`
	DevicePlugin     ImageVersion           `yaml:"devicePlugin"`
	DevicePluginInit DevicePluginInitConfig `yaml:"devicePluginInit"`
	Scheduler        ImageVersion           `yaml:"scheduler"`
	PodValidator     OptionalComponent      `yaml:"podValidator"`
	Exporter         OptionalComponent      `yaml:"exporter"`
	HealthChecker    OptionalComponent      `yaml:"healthChecker"`
	CatalogSource    ImageVersion           `yaml:"catalog"`
	ExporterMockUser ImageVersion           `yaml:"mockUser"`
	CardManagement   CardManagementConfig   `yaml:"cardManagement"`
	HasDevice        bool                   `yaml:"hasDevice"`
	PseudoDeviceMode bool                   `yaml:"pseudoDeviceMode"`
	NodeName         string                 `yaml:"nodeName"`
	WorkloadImage    string                 `yaml:"workloadImage"`
}

func (config *TestConfig) SetRepositories() {
	config.CatalogSource.Repository = config.Repository
	config.DevicePlugin.Repository = config.Repository
	config.DevicePluginInit.SetConfigOptions(config.Repository, config.DevicePluginInit.ExecutePolicy)
	config.Exporter.Repository = config.Repository
	config.ExporterMockUser.Repository = config.Repository
	config.Operator.Repository = config.Repository
	config.Scheduler.Repository = config.Repository
	config.PodValidator.Repository = config.Repository
	config.HealthChecker.Repository = config.Repository
	config.CardManagement.SetConfigOptions(config.CardManagement.Repository, config.CardManagement.Enabled, *config.CardManagement.Config.SpyreFilter, config.CardManagement.ImagePullPolicy)
}

type ImageVersion struct {
	Image           string `yaml:"image"`
	Version         string `yaml:"version"`
	Repository      string `yaml:"repository"`
	ImagePullPolicy string `yaml:"imagePullPolicy"`
}

type OptionalComponent struct {
	ImageVersion `yaml:",inline"`
	Enabled      bool `yaml:"enabled,omitempty"`
}

type DevicePluginInitConfig struct {
	OptionalComponent `yaml:",inline"`
	ExecutePolicy     spyrev1alpha1.ExecutePolicy `yaml:"executePolicy,omitempty"`
}

func (dpi *DevicePluginInitConfig) SetConfigOptions(repository string, executePolicy spyrev1alpha1.ExecutePolicy) {
	dpi.Repository = repository
	dpi.ExecutePolicy = executePolicy
}

type CardManagementConfig struct {
	OptionalComponent `yaml:",inline"`
	Config            spyrev1alpha1.CardManagementConfig `yaml:"config"`
}

func (cm *CardManagementConfig) SetConfigOptions(repository string, enabled bool, spyreFilter string, imgPullPol string) {
	cm.Repository = repository
	cm.ImagePullPolicy = imgPullPol
	cm.Enabled = enabled
	cm.Config.SpyreFilter = &spyreFilter
}

// GetImage returns the full image.
// If the repository is set, it will be inserted as a prefix.
// If the version is a sha256 digest, it will be appended with an @.
// If the version is not a sha256 digest, it will be appended with a :.
func (iv *ImageVersion) GetImage() string {
	image := iv.Image
	if iv.Repository != "" {
		image = fmt.Sprintf("%s/%s", iv.Repository, iv.Image)
	}
	if iv.Version != "" {
		if strings.HasPrefix(iv.Version, "sha256:") {
			return image + "@" + iv.Version
		} else {
			return image + ":" + iv.Version
		}
	}
	return image
}

func (iv *ImageVersion) GetImageName() string {
	return fmt.Sprintf("%s/%s", iv.Repository, iv.Image)
}

// LoadTestConfig reads config from TestConfigFilePathKey
func LoadTestConfig() TestConfig {
	filename, exists := os.LookupEnv(TestConfigFilePathKey)
	if !exists {
		log.Printf("%s is not specified; use default.", TestConfigFilePathKey)
		filename = "config.yaml"
	}
	var config TestConfig
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading YAML file: %v", err)
	}
	// Unmarshal YAML content into TestConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Error un-marshalling YAML content: %v", err)
	}
	config.SetRepositories()
	return config
}
