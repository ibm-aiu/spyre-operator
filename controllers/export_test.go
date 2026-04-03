/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package controllers

import (
	"github.com/ibm-aiu/spyre-operator/internal/state"
	"go.uber.org/zap/zapcore"
)

var ProcessOverallStatus = processOverallStatus
var NodeUpdateNeedsReconcile = nodeUpdateNeedsReconcile

func (rec *SpyreClusterPolicyReconciler) SetStateController(stateController *state.StateController) {
	rec.stateController = stateController
}

func (rec *SpyreClusterPolicyReconciler) ApplyLogLevel(logLevel zapcore.Level) {
	rec.stateController.ClusterState.SetLogLevel(logLevel)
}
