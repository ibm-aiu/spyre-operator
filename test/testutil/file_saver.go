/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

const (
	devicePluginLogFile = "device-plugin.log"
	operatorLogFile     = "operator-manager.log"
)

var (
	LogFolder = filepath.Join("..", "..", "e2e-test-log")
)

func PrepareLogFolder(g Gomega) {
	if _, err := os.Stat(LogFolder); os.IsNotExist(err) {
		fmt.Println("Create new log folder")
		err = os.Mkdir(LogFolder, 0755)
		g.Expect(err).To(BeNil())
	}
}

func WriteDevicePluginLog(g Gomega, content string) {
	logFile := filepath.Join(LogFolder, devicePluginLogFile)
	writeLogFile(g, logFile, content)
}

func WriteOperatorLog(g Gomega, content string) {
	logFile := filepath.Join(LogFolder, operatorLogFile)
	writeLogFile(g, logFile, content)
}

func WriteSpyreNodeState(g Gomega, yamlObj interface{}, nodeName string) {
	spyreNodeStatYamlFile := nodeName + "-spyre-node-state.yaml"
	logFile := filepath.Join(LogFolder, spyreNodeStatYamlFile)
	writeYamlFile(g, logFile, yamlObj)
}

func WriteNode(g Gomega, yamlObj interface{}, nodeName string) {
	nodeYamlFile := nodeName + "-node.yaml"
	logFile := filepath.Join(LogFolder, nodeYamlFile)
	writeYamlFile(g, logFile, yamlObj)
}

func writeYamlFile(g Gomega, logFile string, yamlObj interface{}) {
	yamlData, err := yaml.Marshal(yamlObj)
	g.Expect(err).To(BeNil())
	writeLogFile(g, logFile, string(yamlData))
}

func writeLogFile(g Gomega, logFile string, content string) {
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	g.Expect(err).To(BeNil())
	defer func() {
		_ = file.Close()
	}()
	_, err = io.WriteString(file, content)
	g.Expect(err).To(BeNil())
}
