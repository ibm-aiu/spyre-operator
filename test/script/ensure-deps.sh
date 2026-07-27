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

	if ! command -v oc >/dev/null 2>&1; then
		if crc version >/dev/null 2>&1; then
			eval $(crc oc-env)
		else
			echo "oc not found in path, is required"
			exit 1
		fi
	fi

	echo "Done."
}

# wait for the marketplace catalog source ($1) to be serving before subscribing against it
function wait_for_catalog_source() {
	local name=$1
	local namespace="openshift-marketplace"
	for _ in $(seq 1 60); do
		oc -n "$namespace" get catalogsource "$name" &>/dev/null && break
		sleep 10
	done
	oc -n "$namespace" wait catalogsource/"$name" --for=jsonpath='{.status.connectionState.lastObservedState}'=READY --timeout=600s
}

# wait for "Succeed" in phase of CSV instance generated from the subscription ($2) in namespace ($1)
function wait_for_operator() {
	local namespace=$1
	local sub_name=$2
	# poll until OLM resolves the subscription to a CSV, otherwise currentCSV is null and the wait below has nothing to target
	local csv_name=""
	for _ in $(seq 1 60); do
		csv_name=$(oc -n "$namespace" get sub "$sub_name" -o jsonpath='{.status.currentCSV}' 2>/dev/null)
		[ -n "$csv_name" ] && break
		sleep 10
	done
	if [ -z "$csv_name" ]; then
		echo "ERROR: subscription ${namespace}/${sub_name} did not resolve to a CSV within timeout." >&2
		oc -n "$namespace" get sub "$sub_name" -o yaml >&2 || true
		return 1
	fi
	oc -n "$namespace" wait csv/"$csv_name" --for=jsonpath='{.status.phase}'=Succeeded --timeout=600s
}

function deploy_dependencies() {

	# ensure the catalog serving the dependent operators is ready before subscribing
	wait_for_catalog_source redhat-operators

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
