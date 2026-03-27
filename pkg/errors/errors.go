/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package errors

import (
	"errors"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	ErrNilSpyreClusterPolicy = errors.New("a non nil SpyreClusterPolicy is required")
	ErrParseFile             = errors.New("failed to parse file")
)

func LogErrUpdate(logger logr.Logger, err error) {
	logger.Error(err, "failed to update")
}

func LogErrCreate(logger logr.Logger, err error) {
	logger.Error(err, "failed to create")
}

func LogWarningDelete(logger logr.Logger, err error) {
	logger.Error(err, "failed to delete")
}

func LogErrGet(logger logr.Logger, err error) {
	if apiErrors.IsNotFound(err) {
		logger.Info(err.Error())
	} else {
		logger.Error(err, "failed to get")
	}
}

func LogErrControllerReferenceSet(logger logr.Logger, err error) {
	logger.Error(err, "failed to set ControllerReference")
}
