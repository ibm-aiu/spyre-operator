#!/bin/bash

# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025, 2026 All Rights Reserved                |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -e -o pipefail
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT_DIR=${SCRIPT_DIR%/*/*}
readonly TEST_CONFIG=${REPO_ROOT_DIR}/test/config.yaml
readonly YQ=${REPO_ROOT_DIR}/bin/yq
readonly GIT=$(command -v git)

function usage() {
	echo "Usage: ${0} flags"
	echo "Flags:"
	echo "  -h, --help prints this message"
	exit 2
}

function validate_environment() {
	if [ ! -f ${YQ} ]; then
		echo "Error: expecting yq to be present here: ${YQ}"
		echo "Error: run make yq to download the executable"
		exit 1
	fi

	if [ "x" == "x${GIT}" ]; then
		echo "Error: GIT must have a value, git needs to be available in your path"
		exit 1
	fi
}

function get_current_version() {
	cat ${REPO_ROOT_DIR}/VERSION
}

function strip_patch() {
	local current_version=${1}
	local va=($(echo ${current_version} | sed -r 's/(\.)|(-rc.)/ /g'))

	if [[ ${#va[@]} -ne 3 && ${#va[@]} -ne 4 ]]; then
		echo "Invalid number of elements in version. Expecting either 3 or 4"
		exit 1
	fi
	echo "${va[0]}.${va[1]}"
}

function get_branch_type() {
	local branch_name=${GIT_BRANCH_NAME:-}
	if [[ -z ${branch_name} ]]; then
		branch_name=$(git branch --show-current)
	fi
	if [[ -z ${branch_name} ]]; then
		branch_name=$(git rev-parse --abbrev-ref HEAD)
	fi

	if [[ ${branch_name} =~ ^release_v[0-9]+(\.[0-9]+)+$ ]]; then
		echo "release"
	elif [[ ${branch_name} =~ ^v[0-9](\.[0-9]+)+-rc\.[0-9]+$ ]]; then
		echo "release-candidate"
	elif [[ ${branch_name} == "main" ||
		${branch_name} =~ ^patch_to_v[0-9]+(\.[0-9]+)+$ ||
		${branch_name} =~ ^update_to_v[0-9]+(\.[0-9]+)+$ ]]; then
		echo "development"
	else
		echo "pr"
	fi
}

function get_default_channel() {
	local current_version=$(get_current_version)
	local short_version=$(strip_patch ${current_version})
	local branch_type=$(get_branch_type)

	case ${branch_type} in
	release)
		echo "stable-v${short_version}"
		;;
	release-candidate)
		echo "candidate-v${short_version}"
		;;
	development | pr)
		echo "fast-v${short_version}"
		;;
	esac
}

function patch_test_config() {
	local default_channel=$(get_default_channel)
	local branch_type=$(get_branch_type)

	if [[ -n ${HAS_DEVICE} ]]; then
		echo "updating HAS_DEVICE to ${HAS_DEVICE}"
		${YQ} eval -i '.hasDevice=(strenv(HAS_DEVICE) == "true")' ${TEST_CONFIG}
	fi

	# Set the default channel directly in the test config
	${YQ} eval -i ".defaultChannel=\"${default_channel}\"" ${TEST_CONFIG}

	# Propagate version to test/config.yaml for all non-release builds.
	# get-version.bash -t relative handles all branch types:
	#   release / rc  → <version>            (but we skip propagation for those)
	#   main          → <version>-dev        (no hash suffix)
	#   patch_to_v*   → <version>-dev        (no hash suffix)
	#   update_to_v*  → <version>-dev-<hash>
	#   PR / other    → <version>-dev-<hash>
	if [[ ${branch_type} != "release" && ${branch_type} != "release-candidate" ]]; then
		local version=$(${REPO_ROOT_DIR}/hack/get-version.bash -t relative -v "$(get_current_version)")
		echo "Propagating version ${version} to test config"
		${YQ} eval -i ".operator.version=\"${version}\"" ${TEST_CONFIG}
		${YQ} eval -i ".catalog.version=\"${version}\"" ${TEST_CONFIG}
		${YQ} eval -i ".bundle.version=\"${version}\"" ${TEST_CONFIG}
	fi

	if [[ -n ${WORKLOAD_IMAGE} ]]; then
		echo "updating WORKLOAD_IMAGE to ${WORKLOAD_IMAGE}"
		${YQ} eval -i '.workloadImage=strenv(WORKLOAD_IMAGE)' ${TEST_CONFIG}
	fi

	if [[ -n ${NODE_NAME} ]]; then
		${YQ} eval -i '.nodeName=strenv(NODE_NAME)' ${TEST_CONFIG}
	fi

	if [[ -n ${PSEUDO_DEVICE_MODE} ]]; then
		echo "updating PSEUDO_DEVICE_MODE to ${PSEUDO_DEVICE_MODE}"
		${YQ} eval -i '.pseudoDeviceMode = (strenv(PSEUDO_DEVICE_MODE) == "true")' ${TEST_CONFIG}
	fi

	if [[ -n ${OPERATOR_TAG} ]]; then
		${YQ} eval -i '.operator.version=strenv(OPERATOR_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${BUNDLE_TAG} ]]; then
		${YQ} eval -i '.bundle.version=strenv(BUNDLE_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${CATALOG_TAG} ]]; then
		${YQ} eval -i '.catalog.version=strenv(CATALOG_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${EXPORTER_TAG} ]]; then
		${YQ} eval -i '.exporter.version=strenv(EXPORTER_TAG)' ${TEST_CONFIG}
		${YQ} eval -i '.mockUser.version=strenv(EXPORTER_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${DEVICE_PLUGIN_TAG} ]]; then
		${YQ} eval -i '.devicePlugin.version=strenv(DEVICE_PLUGIN_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${DEVICE_PLUGIN_INIT_TAG} ]]; then
		${YQ} eval -i '.devicePluginInit.version=strenv(DEVICE_PLUGIN_INIT_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${SCHEDULER_PLUGIN_TAG} ]]; then
		${YQ} eval -i '.scheduler.version=strenv(SCHEDULER_PLUGIN_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${VALIDATOR_TAG} ]]; then
		${YQ} eval -i '.podValidator.version=strenv(VALIDATOR_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${HEALTH_CHECKER_TAG} ]]; then
		${YQ} eval -i '.healthChecker.version=strenv(HEALTH_CHECKER_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${CARD_MGMT_TAG} ]]; then
		${YQ} eval -i '.cardManagement.version=strenv(CARD_MGMT_TAG)' ${TEST_CONFIG}
	fi

	if [[ -n ${CARD_MGMT_RUNNER_IMAGE} ]]; then
		${YQ} eval -i '.cardManagement.config.pfRunnerImage=strenv(CARD_MGMT_RUNNER_IMAGE)' ${TEST_CONFIG}
		${YQ} eval -i '.cardManagement.config.vfRunnerImage=strenv(CARD_MGMT_RUNNER_IMAGE)' ${TEST_CONFIG}
	fi

	if [[ -n ${SPYRE_FILTER} ]]; then
		${YQ} eval -i '.cardManagement.config.spyreFilter=strenv(SPYRE_FILTER)' ${TEST_CONFIG}
	fi
}
declare -a POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
	case ${1} in
	-h | --help)
		usage
		;;
	-* | --*)
		echo "Unknown option $1"
		exit 1
		;;
	*)
		POSITIONAL_ARGS+=("$1") # save positional arg
		shift                   # past argument
		;;
	esac
done

if [[ ${#POSITIONAL_ARGS[*]} -gt 0 ]]; then
	echo "Unexpected number of arguments passed"
	exit 1
fi

validate_environment
patch_test_config
