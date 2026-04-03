#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright (c) 2025, 2026 IBM Corp.                                |
# | SPDX-License-Identifier: Apache-2.0                               |
# +-------------------------------------------------------------------+
# This script ensures dependent operators.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT_DIR=${SCRIPT_DIR%/*/*}
MANIFEST_PATH=${REPO_ROOT_DIR}/test/manifest
readonly YQ=${REPO_ROOT_DIR}/bin/yq
readonly KUSTOMIZE=${REPO_ROOT_DIR}/bin/kustomize
# ensure "oc" both for crc/non-crc environment
function validate_environment() {

	echo "Validating environment..."

	if [ ! -f ${YQ} ]; then
		make -f ${REPO_ROOT_DIR}/Makefile yq
	fi

	if [ ! -f ${KUSTOMIZE} ]; then
		make -f ${REPO_ROOT_DIR}/Makefile kustomize
	fi

	if ! command -v oc 1 2>&1 >/dev/null; then
		if crc version 2>&1 >/dev/null; then
			eval $(crc oc-env)
		else
			echo "oc not found in path, is required"
			exit 1
		fi
	fi

	echo "Done."
}

# wait for "Succeed" in phase of CSV instance generated from the subscription ($2) in namespace ($1)
function wait_for_operator() {
	local namespace=$1
	local sub_name=$2
	oc -n $namespace wait sub/$sub_name --for=jsonpath='{.status.state}'=AtLatestKnown --timeout=600s
	csv_name=$(oc -n $namespace get sub $sub_name -o yaml | ${YQ} '.status.currentCSV')
	oc -n $namespace wait csv/$csv_name --for=jsonpath='{.status.phase}'=Succeeded --timeout=600s
}

function deploy_dependencies() {

	# deploy operators
	oc apply -f $MANIFEST_PATH/dependencies/nfd/operator.yaml
	oc apply -f $MANIFEST_PATH/dependencies/cert-manager/operator.yaml
	oc apply -f $MANIFEST_PATH/dependencies/secondary-scheduler/operator.yaml

	# wait for operators
	wait_for_operator openshift-nfd nfd
	wait_for_operator cert-manager-operator openshift-cert-manager-operator
	wait_for_operator openshift-secondary-scheduler-operator openshift-secondary-scheduler-operator

	# apply configs
	oc apply -f $MANIFEST_PATH/dependencies/nfd/config.yaml

}

validate_environment
deploy_dependencies
