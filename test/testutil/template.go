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

const PrintSenlibConfig = "cat /etc/aiu/senlib_config.json;tail -f /dev/null"

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

// SmallToyJobTemplate runs the small-toy workload on a Spyre PF until it exits.
// Keep the command literal: html/template escapes interpolated values.
const SmallToyJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{.Name}}
  labels:
    app: {{.Name}}
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: {{.Name}}
    spec:
      restartPolicy: Never
      schedulerName: spyre-scheduler
      containers:
      - name: small-toy
        image: {{.Image}}
        imagePullPolicy: Always
        # Need to set workingDir to writable path
        workingDir: /tmp
        # 1. Quick generate topo.json
        # 2. Assign rank from topo file to generate env.sh
        # 3. Apply env.sh to set env values
        # 4. Run small-toy
        command:
        - /bin/bash
        - -c
        - |
          /opt/ibm/spyre/bin/aiu-discover-topo -j /tmp/topo.json &&
          python3 /opt/ibm/spyre/bin/aiu-assign-ranks.py \
          -t /tmp/topo.json \
          -i /etc/aiu/senlib_config.json \
          -o /tmp/senlib_config.json.new \
          -e /tmp/env.sh &&
          source /tmp/env.sh &&
          export PATH=/opt/ibm/spyre/runtime/bin:$PATH &&
          python3 -m torch.distributed.run \
              --nproc-per-node ${AIU_WORLD_SIZE} \
              /usr/local/lib/python3.12/site-packages/aiu_fms_testing_utils/scripts/small-toy.py \
              --backend aiu
        resources:
          limits:
            {{.ResourceName}}: {{.ResourceQuantity}}
        env:
        - name: FLEX_COMPUTE
          value: SENTIENT
        - name: FLEX_DEVICE
          value: PF
        - name: FLEX_HDMA_MODE_FULL
          value: "1"
        - name: DEEPTOOLS_PATH
          value: /opt/ibm/spyre/deeptools/share
        - name: SENLIB_INSTALL_DIR
          value: /opt/ibm/spyre/senlib
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

const PodWithResourceClaimTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
spec:
  restartPolicy: Never
  containers:
  - name: app
    image: {{ .Image }}
    imagePullPolicy: IfNotPresent
    {{- if .Arg0 }}
    command: ["/bin/bash", "-c"]
    args:
    - "{{ .Arg0 }}"
    {{- else }}
    command: ["tail", "-f", "/dev/null"]
    {{- end }}
    resources:
      claims:
      - name: spyre
  resourceClaims:
  - name: spyre
    resourceClaimTemplateName: {{ .ResourceClaimTemplateName }}
  terminationGracePeriodSeconds: 0
  {{- if .NodeSelectorNode }}
  nodeSelector:
    "kubernetes.io/hostname": {{ .NodeSelectorNode }}
  {{- end }}
`

type PodTemplateData struct {
	Name             string
	Namespace        string
	Image            string
	ResourceName     string
	ResourceQuantity string
	NodeSelectorNode string
	FlexDevice       string
	SidecarName      string
	Arg0             string
}

// BasicPodTemplateData sets default ubi image without node or arg0
func BasicPodTemplateData(name, namespace string) *PodTemplateData {
	return &PodTemplateData{
		Name:      name,
		Namespace: namespace,
		Image:     Ubi9MicroTestImage,
	}
}

func (p *PodTemplateData) SetImage(image string) *PodTemplateData {
	p.Image = image
	return p
}

func (p *PodTemplateData) SetNode(node string) *PodTemplateData {
	p.NodeSelectorNode = node
	return p
}

func (p *PodTemplateData) SetArg0(arg0 string) *PodTemplateData {
	p.Arg0 = arg0
	return p
}

// PodWithResourceClaimTemplateData holds basic PodTemplateData and ResourceClaimTemplateName
type PodWithResourceClaimTemplateData struct {
	PodTemplateData
	ResourceClaimTemplateName string
}

func YamlFromTemplate(tmpl string, data any) (yamlPathName string) {
	manifestTmpl, err := template.New("pod11").Parse(tmpl)
	Expect(err).To(BeNil(), "Error parsing template: %v", err)

	file, err := os.CreateTemp("", "manifest-*.yaml")
	Expect(err).To(BeNil(), "Error creating template file: %v", err)

	err = manifestTmpl.Execute(file, data)
	Expect(err).To(BeNil(), "Error executing template: %v", err)

	return file.Name()
}
