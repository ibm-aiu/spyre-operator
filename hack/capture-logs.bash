#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT=${SCRIPT_DIR%/*}
readonly POD_NAME=${1}
readonly NAMESPACE=${2}
readonly BARRIER_FILE=${3}

function usage() {
	echo "Usage: capture-logs.bash <pod_name> <namespace> <barrier_file_name>"
	exit 2
}

if [[ "x" == "x${POD_NAME}" ]]; then
	echo "pod name is required"
	usage
fi

if [[ "x" == "x${NAMESPACE}" ]]; then
	echo "name space required"
	usage
fi

if [[ "x" == "x${BARRIER_FILE}" ]]; then
	echo "barrier file is required"
	usage
fi

if [[ ! -f ${REPO_ROOT}/bin/stern ]]; then
	echo "stern binary not found in '${REPO_ROOT}/bin/'"
	exit 2
fi

trap 'kill $(jobs -p) 2>/dev/null || true' EXIT
if [[ -f ${BARRIER_FILE} ]]; then
	${REPO_ROOT}/bin/stern ${POD_NAME} -n ${NAMESPACE} &
	STERN_PID=${!}
fi

while [[ -f ${BARRIER_FILE} ]]; do
	sleep 30s
done

if [[ -n ${STERN_PID} ]]; then
	echo "Barrier file removed, kill stern process =${STERN_PID}"
	kill -9 ${STERN_PID}
fi
exit 0
