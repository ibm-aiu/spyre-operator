/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */
package testutil

var (
	spyreCrds = []string{
		"spyreclusterpolicies.spyre.ibm.com",
		"spyrenodestates.spyre.ibm.com",
	}
	ExpectedNodeLabelsWithPseudoDevice = map[string]string{
		"ibm.com/spyre.deploy.device-plugin":     "true",
		"ibm.com/spyre.deploy.feature-discovery": "true",
		"ibm.com/spyre.deploy.metrics-exporter":  "true",
		"ibm.com/spyre.present":                  "true",
		"ibm.com/spyre.count":                    "2",
		"ibm.com/spyre.product":                  "Spyre",
	}
	monitoringDisableAnnotations = map[string]string{
		"inject-spyre-monitoring-sidecar":        "false",
		"inject-spyre-monitoring-service":        "false",
		"inject-spyre-monitoring-servicemonitor": "false",
	}
)
