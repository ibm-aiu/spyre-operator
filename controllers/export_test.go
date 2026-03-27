/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
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
