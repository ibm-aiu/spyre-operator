/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package pcitopov2

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
)

var (
	OutOfExpectedTierErr = fmt.Errorf("some device is out of expected tier")
	DeviceNotFoundErr    = fmt.Errorf("device not found in topology")
)

type Pcitopo struct {
	Timestamp         string            `json:"timestamp,omitempty"`
	Version           float32           `json:"version,omitempty"` // this must be string!
	Server            string            `json:"server,omitempty"`
	NumDevices        int               `json:"num_devices,omitempty"`
	Devices           map[string]Device `json:"devices,omitempty"`
	SpyreVfNumDevices int               `json:"spyre_vf_num_devices,omitempty"`
	SpyreVfDevices    map[string]Device `json:"spyre_vf_devices,omitempty"`
}

type Device struct {
	Name         string `json:"name,omitempty"`
	NumaNode     int    `json:"numanode,omitempty"`
	Linkspeed    string `json:"linkspeed,omitempty"`
	Peers        Peers  `json:"peers,omitempty"`
	SpyreVfPeers Peers  `json:"spyre_vf_peers,omitempty"`
	DeviceId     string `json:"device_id,omitempty"`
	IsPf         bool   `json:"is_pf,omitempty"`
}

type Peers struct {
	Peer0 map[string]int `json:"peers_0,omitempty"`
	Peer1 map[string]int `json:"peers_1,omitempty"`
	Peer2 map[string]int `json:"peers_2,omitempty"`
}

func (t Pcitopo) Write(filepath string) error {
	outputFile, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create pcitopo file: %s: %w", filepath, err)
	}
	defer outputFile.Close() //nolint:errcheck
	jsonData, err := json.MarshalIndent(t, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal pcitopo data: %w", err)
	}
	if _, err = outputFile.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write pcitopo file: %s: %w", filepath, err)
	}
	return nil
}

func (t Pcitopo) String() string {
	if jsonData, err := json.Marshal(t); err != nil {
		return ""
	} else {
		return string(jsonData)
	}
}

// ValidateTier returns error if not valid or cannot check validity
func (t Pcitopo) ValidateTier(resourceName string, deviceIDs []string) error {
	if len(deviceIDs) <= 1 {
		return nil
	}
	refDevice := deviceIDs[0]
	deviceTopo, found := t.Devices[refDevice]
	if !found {
		return DeviceNotFoundErr
	}
	tier1 := strings.HasSuffix(resourceName, spyreconst.TierOneResourceNameSuffix)
	tier2 := strings.HasSuffix(resourceName, spyreconst.TierTwoResourceNameSuffix)
	for _, otherDevice := range deviceIDs[1:] {
		if _, found := deviceTopo.Peers.Peer0[otherDevice]; found {
			continue
		}
		if tier1 || tier2 {
			if _, found := deviceTopo.Peers.Peer1[otherDevice]; found {
				continue
			} else if tier2 {
				if _, found := deviceTopo.Peers.Peer2[otherDevice]; found {
					continue
				}
			}
			return OutOfExpectedTierErr
		}
		return OutOfExpectedTierErr
	}
	return nil
}

func (t Pcitopo) GetDevices() []string {
	devices := []string{}
	for device := range t.Devices {
		devices = append(devices, device)
	}
	return devices
}

// unmarshalPciTopo extracts "version" attribute if exists and calls corresponding convert function.
// returns default format of Pcitopo and error if exists.
func UnmarshalPciTopo(data []byte) (Pcitopo, error) {
	var pcitopo Pcitopo
	if err := json.Unmarshal(data, &pcitopo); err != nil {
		return pcitopo, fmt.Errorf("failed to unmarshal pcitopo data: %w", err)
	}
	return pcitopo, nil
}
