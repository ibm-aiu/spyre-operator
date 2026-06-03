#!/bin/bash

# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly OC=$(command -v oc)
readonly BARRIER_FILE=${1}

function usage() {
	echo "Usage: monitor-openshift-marketplace.bash <barrier_file_name>"
	exit 2
}

if [[ "x" == "x${BARRIER_FILE}" ]]; then
	echo "barrier file is required"
	usage
fi

if [[ "x" == "x${OC}" ]]; then
	echo "oc must be available on the path"
	exit 1
fi

trap 'kill $(jobs -p) 2>/dev/null || true' EXIT
if [[ -f ${BARRIER_FILE} ]]; then
	echo "---------------------------------------------- Pods in openshift-marketplace        ------------------------------------------------------------------------------------------"
	${OC} get pods -n openshift-marketplace -o wide || true
	echo "---------------------------------------------- Installed cluster service versions   ------------------------------------------------------------------------------------------"
	${OC} get clusterserviceversions -A -o wide || true
	echo "---------------------------------------------- Existing spyre operator subscription ------------------------------------------------------------------------------------------"
	(${OC} get subscription spyre-operator -n spyre-operator -o json | jq '.status.conditions') || true
	echo "----------------------------------------------- Catalog source for sypre operator   -----------------------------------------------------------------------------------------"
	${OC} get pods -n openshift-marketplace -l olm.catalogSource=spyre-operators -o wide -w &
	GETPODS_PID=${!}
fi

while [[ -f ${BARRIER_FILE} ]]; do
	sleep 30s
done

if [[ -n ${GETPODS_PID} ]]; then
	echo "Barrier file removed, kill process: ${GETPODS_PID}"
	kill -9 ${GETPODS_PID} ${GETSUBS_PID}
fi
exit 0
