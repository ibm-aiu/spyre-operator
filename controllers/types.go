/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package controllers

type SenlibConfigGeneral struct {
	PciAddresses       []string `json:"sen_bus_id"`
	Target             string   `json:"target"`
	MultiAiuConfigPath string   `json:"multi_aiu_config_path"`
	Doom               bool     `json:"doom"`
}

type SenlibConfigMetricGeneral struct {
	General SenlibConfigMetric `json:"general"`
}

type SenlibConfigMetric struct {
	Enable     bool             `json:"enable"`
	Path       string           `json:"path"`
	Port       int              `json:"port"`
	PromClient PromClientConfig `json:"promclient"`
}

type PromClientConfig struct {
	WakeUpTimeInSec int `json:"wakeup_interval_in_seconds"`
}

type SenlibConfig struct {
	General SenlibConfigGeneral       `json:"GENERAL"`
	Metric  SenlibConfigMetricGeneral `json:"METRICS"`
}
