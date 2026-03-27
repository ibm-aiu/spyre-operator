#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -eu -o pipefail
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT_DIR=${SCRIPT_DIR%/*}
readonly SCRIPT_NAME=$(basename $0)
readonly BOM=${REPO_ROOT_DIR}/release-artifacts.yaml
readonly YQ=${REPO_ROOT_DIR}/bin/yq
readonly SKOPEO=$(which skopeo)
readonly GIT=$(which git)
readonly CURRENT_VERSION=$(make -f ${REPO_ROOT_DIR}/Makefile echo-version)
readonly TARGET_REPOSITORY="docker.io/spyre-operator"

function validate_environment() {

	if [ "x" == "x${SKOPEO}" ]; then
		echo "Error: SKOPEO must have a value, skopeo needs to be available in your path"
		exit 1
	fi

	if [ "x" == "x${GIT}" ]; then
		echo "Error: GIT must have a value, skopeo needs to be available in your path"
		exit 1
	fi

	if [ ! -f ${YQ} ]; then
		make -f ${SCRIPT_DIR}/Makefile yq
	fi

}

function copy_bundles() {
	local type=${1}
	local bundle_tags=$(${YQ} ".channels.${type}.Bundles | join (\" \")" ${BOM})

	echo "${SCRIPT_NAME} : Copy ${type} bundle images from artifactory to ICR repos..."
	for bundle_tag in ${bundle_tags}; do
		local src_bundle="${SOURCE_REPOSITORY}/spyre-operator-bundle:${bundle_tag}"
		local target_bundle="${TARGET_REPOSITORY}/spyre-operator-bundle:${bundle_tag}"
		echo "${SCRIPT_NAME} : Copying '${src_bundle}' to '${target_bundle}'"
		${SKOPEO} copy -q --multi-arch all --preserve-digests docker://${src_bundle} docker://${target_bundle}
	done
	echo "${SCRIPT_NAME} : Done."
}

validate_environment
copy_bundles Stable
copy_bundles Candidates
copy_bundles Fast
