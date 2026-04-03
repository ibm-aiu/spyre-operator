/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"html/template"
	"os"

	. "github.com/onsi/gomega"
)

const PodTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{.Name}}
  labels:
    app: mypod
spec:
  schedulerName: spyre-scheduler
  containers:
  - name: app
    image: registry.access.redhat.com/ubi9-minimal:9.5
    imagePullPolicy: IfNotPresent
    command: ["tail", "-f", "/dev/null"]
    resources:
      requests:
       {{.ResourceName}}: {{.ResourceQuantity}}
      limits:
       {{.ResourceName}}: {{.ResourceQuantity}}
  terminationGracePeriodSeconds: 0
  {{if .NodeSelectorNode}}nodeSelector:
    "kubernetes.io/hostname": {{.NodeSelectorNode}}
  {{end}}
`
const WorkloadPodTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{.Name}}
spec:
  schedulerName: spyre-scheduler
  restartPolicy: Never
  containers:
  - name: app
    imagePullPolicy: Always
    image: {{.Image}}
    command:
    - /usr/bin/bash
    - -lc
    - /home/senuser/entry.sh
    resources:
      requests:
       {{.ResourceName}}: {{.ResourceQuantity}}
      limits:
       {{.ResourceName}}: {{.ResourceQuantity}}
    env:
    - name: HOME
      value: /home/senuser
    - name: FLEX_COMPUTE
      value: SENTIENT
    - name: FLEX_DEVICE
      value: {{.FlexDevice}}
    - name: DTLOG_LEVEL
      value: "info"
    - name: TORCH_SENDNN_LOG
      value: "CRITICAL"
    - name: DT_DEEPRT_VERBOSE
      value: "-1"
    workingDir: /home/senuser
    volumeMounts:
    - mountPath: /dev/shm
      name: dev-shm
    - name: config-volume
      mountPath: /home/senuser/entry.sh
      subPath: entry.sh
    - name: config-volume
      mountPath: /tmp/dd2.json
      subPath: dd2.json
    terminationMessagePolicy: FallbackToLogsOnError
  volumes:
  - name: config-volume
    configMap:
      name: small-toy-config
      defaultMode: 0777
  - name: dev-shm
    emptyDir:
      medium: Memory
      sizeLimit: 8G
  {{if .NodeSelectorNode}}nodeSelector:
    "kubernetes.io/hostname": {{.NodeSelectorNode}}
  {{end}}
`
const WorkloadConfigMapTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: small-toy-config
data:
  entry.sh: |
    #!/bin/bash
    cd /home/senuser
    # Using default packages on the workload packages
    pip install ibm-fms aiu-fms-testing-utils --no-deps
    git clone https://github.com/foundation-model-stack/aiu-fms-testing-utils.git
    jq -s '.[0] * .[1]' /etc/aiu/senlib_config.json /tmp/dd2.json > /tmp/senlib_config.json
    export SENLIB_DEVEL_CONFIG_FILE=/tmp/senlib_config.json
    export FLEX_HDMA_MODE_FULL=1
    [ ! -z ${AIU_WORLD_SIZE} ] && torchrun --nproc-per-node ${AIU_WORLD_SIZE} aiu-fms-testing-utils/scripts/{{.ScriptName}} --backend aiu
  dd2.json: |
    {
    "SNT_MCI": {
      "DCR": {
        "MCI_CTRL": {
          "ENABLE_RISCV": "0x0"
        }
      }
    },
    "GENERAL": {
      "doom": {{.Doom}}
    },
    "METRICS": {
      "general": {
        "enable": true
        }
      }
    }
`
const CurlPodTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{.Name}}
  namespace: "spyre-apps"
  labels:
    app: curl
spec:
  containers:
  - name: curl
    image: {{.Image}}
    command: ["/bin/sh"]
    args:
    - "-c"
    - "sleep infinity"
    resources:
      limits:
        cpu: "500m"
        memory: "512Mi"
      requests:
        cpu: "200m"
        memory: "256Mi"
`

const CardmgmtTestPodTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{.Name}}
spec:
  schedulerName: spyre-scheduler
  containers:
  - name: app
    image: {{.Image}}
    imagePullPolicy: IfNotPresent
    command: ["tail", "-f", "/dev/null"]
    {{if .ResourceName}}resources:
      limits:
       {{.ResourceName}}: {{.ResourceQuantity}}
    {{end}}
  {{if .SidecarName}}- name: {{.SidecarName}}
    image: {{.Image}}
    imagePullPolicy: IfNotPresent
    command: ["tail", "-f", "/dev/null"]
  {{end}}
  {{if .NodeSelectorNode}}nodeSelector:
    "kubernetes.io/hostname": {{.NodeSelectorNode}}
  {{end}}
`

func YamlFromTemplate(tmpl string, data any) (yamlPathName string) {
	manifestTmpl, err := template.New("pod11").Parse(tmpl)
	Expect(err).To(BeNil(), "Error parsing template: %v", err)

	file, err := os.CreateTemp("", "manifest-*.yaml")
	Expect(err).To(BeNil(), "Error creating template file: %v", err)

	err = manifestTmpl.Execute(file, data)
	Expect(err).To(BeNil(), "Error executing template: %v", err)

	return file.Name()
}
