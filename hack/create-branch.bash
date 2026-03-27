#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025, 2026 All Rights Reserved                |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -eu -o pipefail
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT_DIR=${SCRIPT_DIR%/*}
readonly ARTIFACT_CONFIG=${REPO_ROOT_DIR}/release-artifacts.yaml
readonly TEST_CONFIG=${REPO_ROOT_DIR}/test/config.yaml
readonly YQ=${REPO_ROOT_DIR}/bin/yq
readonly GIT=$(command -v git)
readonly COMPONENTS=(devicePlugin devicePluginInit exporter scheduler podValidator healthChecker)
readonly OPERATOR_COMPONENTS=(operator catalog bundle)
readonly EDIT=$(command -v vi 2>/dev/null || command -v vim 2>/dev/null)
readonly YAMLFMT=${REPO_ROOT_DIR}/bin/yamlfmt

RELEASE_TYPE=""
CURRENT_VERSION=""
DRY_RUN=""
declare -i RC_NUMBER=0

function usage() {
	echo "Usage: ${0} flags"
	echo "Flags:"
	echo "  -t, --type version-upgrade|minor-release|major-release|patch-release|rc <old release candidate number>  creates a release branch"
	echo "  -d, --dry-run run the script, do not push the branch"
	echo "  -h, --help prints this message"
	exit 2
}

function validate_environment() {

	if [ "x" == "x${GIT}" ]; then
		echo "Error: GIT must have a value, git needs to be available in your path"
		exit 1
	fi

	if [ "x" == "x${EDIT}" ]; then
		echo "Error: EDIT must have a value, vi, vim or nvim needs to be available in your path"
		exit 1
	fi

	#remove all tools or files, including local.mk
	make -f ${REPO_ROOT_DIR}/Makefile clean

	if [ ! -f ${YQ} ]; then
		make -f ${REPO_ROOT_DIR}/Makefile yq
	fi

	if [ ! -f ${YAMLFMT} ]; then
		make -f ${REPO_ROOT_DIR}/Makefile yamlfmt
	fi
}

function get_current_version() {
	cat ${REPO_ROOT_DIR}/VERSION
}

function get_makefile_var_value() {
	local variable_name="${1}"
	make -f ${REPO_ROOT_DIR}/Makefile print-${variable_name}
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

function is_git_tree_clean() {
	if [[ "xTRUE" == "x${DRY_RUN}" ]]; then
		return
	fi
	local output=$(${GIT} status --porcelain)
	if [ ! -z "${output}" ]; then
		echo "The git working tree has uncommitted files."
		echo "${output}"
		exit 1
	fi
}

function is_current_branch_main() {
	if [[ "xTRUE" == "x${DRY_RUN}" ]]; then
		return
	fi
	local branch_name=${GIT_BRANCH_NAME:-}
	if [[ -z ${branch_name} ]]; then
		branch_name=$(git branch --show-current)
	fi
	if [[ -z ${branch_name} ]]; then
		branch_name=$(git rev-parse --abbrev-ref HEAD)
	fi

	if [[ ${branch_name} != "main" ]]; then
		echo "Must be on main branch to execute this script"
		exit 1
	fi
}

function is_current_branch_release() {
	if [[ "xTRUE" == "x${DRY_RUN}" ]]; then
		return
	fi
	local branch_name=${GIT_BRANCH_NAME:-}
	if [[ -z ${branch_name} ]]; then
		branch_name=$(git branch --show-current)
	fi
	if [[ -z ${branch_name} ]]; then
		branch_name=$(git rev-parse --abbrev-ref HEAD)
	fi
	if [[ ! ${branch_name} =~ ^release_v[0-9]+(\.[0-9]+)+$ ]]; then
		echo "Must be on main branch to execute this script"
		exit 1
	fi
}
function patch_artifacts() {
	local release_type=${1}
	local image_tag=${2}
	local default_channel=${3}
	case ${release_type} in
	major-release | minor-release)
		#add this release to Stable channel
		${YQ} -i ".channels.Stable.Bundles += \"${image_tag}\"" "${ARTIFACT_CONFIG}"
		;;
	patch-release)
		#add this release to Stable channel
		${YQ} -i ".channels.Stable.Bundles += \"${image_tag}\"" "${ARTIFACT_CONFIG}"
		${YQ} -i ".channels.Fast.Bundles += \"${image_tag}-dev\"" "${ARTIFACT_CONFIG}"
		for comp in "${OPERATOR_COMPONENTS[@]}"; do
			${YQ} -i ".${comp}.version=\"${image_tag}\"" "${ARTIFACT_CONFIG}"
		done
		# No further processing is necessary, leting the user to manually update elements
		return
		;;
	rc)
		#add this release to Candidates channel
		${YQ} -i ".channels.Candidates.Bundles += \"${image_tag}\"" "${ARTIFACT_CONFIG}"
		;;
	version-upgrade)
		#add this release to fast channel
		${YQ} -i ".channels.Fast.Bundles += \"${image_tag}\"" "${ARTIFACT_CONFIG}"
		for comp in "${OPERATOR_COMPONENTS[@]}"; do
			${YQ} -i ".${comp}.version=\"${image_tag}\"" "${ARTIFACT_CONFIG}"
		done
		;;
	esac
	${YQ} eval -i ".defaultChannel=\"${default_channel}\"" "${TEST_CONFIG}"
	for comp in "${COMPONENTS[@]}"; do
		${YQ} -i ".${comp}.version=\"${image_tag}\"" "${ARTIFACT_CONFIG}"
	done
}
function make_branch() {
	local branch_name=${1}
	local release_type=${2}
	local default_channel=${3}
	local current_version=${4}

	echo "Making branch '${branch_name}' for version: '${CURRENT_VERSION}'"
	${GIT} fetch --quiet origin

	if ${GIT} show-ref --quiet --verify refs/heads/${branch_name}; then
		echo "A local branch named '${branch_name}' already exists"
		exit 1
	fi

	if ${GIT} show-ref --quiet --verify refs/remotes/origin/${branch_name}; then
		echo "A remote branch named '${branch_name}' already exists"
		exit 1
	fi

	${GIT} checkout -b ${branch_name}
	sed -i.bak "s/^DEFAULT_CHANNEL.*\?=.*/DEFAULT_CHANNEL\t\t\?=${default_channel}/" ${REPO_ROOT_DIR}/Makefile
	rm Makefile.bak

	# get the base version, which value changes with branch name, for more details see
	# get-version.bash script
	local version=$(${REPO_ROOT_DIR}/hack/get-version.bash -t base -v ${current_version})

	# Modify release artifacts (bundles) and operator version
	patch_artifacts ${release_type} ${version} ${default_channel}

	# Prompt the user to modify version information
	${EDIT} -n --ttyfail ${ARTIFACT_CONFIG}

	${SCRIPT_DIR}/propagate-version.bash \
		"${version}" \
		"$(get_makefile_var_value REGISTRY | awk '{print $NF}')" \
		"$(get_makefile_var_value NAMESPACE | awk '{print $NF}')" \
		"${default_channel}"

	make -f ${REPO_ROOT_DIR}/Makefile bundle
	make -f ${REPO_ROOT_DIR}/Makefile api-docs

	# Add to the commit the only known things that are modified by propagate-version
	# The list of things added would need to be modified as propagate-version side
	# effects change
	if [[ "minor-release" != ${release_type} ]] && [[ "major-release" != ${release_type} ]]; then
		${GIT} add ${REPO_ROOT_DIR}/VERSION
	fi

	#ensure consistent formatting with the pre-commit hook
	${YAMLFMT} -conf=${REPO_ROOT_DIR}/.yamlfmt -dstar \
		"${REPO_ROOT_DIR}/config/**/*.yaml" ${REPO_ROOT_DIR}/release-artifacts.yaml \
		"${REPO_ROOT_DIR}/bundle/**/*.yaml" ${REPO_ROOT_DIR}/test/config.yaml

	# Add all modified files to the commit
	${GIT} add ${REPO_ROOT_DIR}/config ${REPO_ROOT_DIR}/release-artifacts.yaml \
		${REPO_ROOT_DIR}/bundle ${REPO_ROOT_DIR}/test/config.yaml \
		${REPO_ROOT_DIR}/Makefile ${REPO_ROOT_DIR}/docs/api \
		${REPO_ROOT_DIR}/bundle.Dockerfile

	${GIT} commit -m "feat: create branch ${branch_name}" -m "Configuration file updates for new version" --no-verify
	is_git_tree_clean # This ensures that we added all items to the commit.

	if [[ "xTRUE" == "x${DRY_RUN}" ]]; then
		return
	fi
	${GIT} push --set-upstream origin ${branch_name}
}

if [[ ${#} == 0 ]]; then
	usage
fi

declare -a POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
	case ${1} in
	-t | --type)
		RELEASE_TYPE="${2}"
		shift # past argument
		shift # past value
		if [[ ${RELEASE_TYPE} == "rc" ]]; then
			RC_NUMBER=$1
			shift
		fi
		;;
	-d | --dry-run)
		DRY_RUN="TRUE"
		shift # past argument
		;;
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
is_git_tree_clean

case ${RELEASE_TYPE} in
minor-release)
	is_current_branch_main
	CURRENT_VERSION=$(get_current_version)
	BRANCH_NAME="release_v${CURRENT_VERSION}"
	DEFAULT_CHANNEL="stable-v$(strip_patch ${CURRENT_VERSION})"
	make_branch ${BRANCH_NAME} ${RELEASE_TYPE} ${DEFAULT_CHANNEL} ${CURRENT_VERSION}
	;;
major-release)
	is_git_tree_clean
	is_current_branch_main
	CURRENT_VERSION=$(get_current_version)
	BRANCH_NAME="release_v${CURRENT_VERSION}"
	DEFAULT_CHANNEL="stable-v$(strip_patch ${CURRENT_VERSION})"
	make_branch ${BRANCH_NAME} ${RELEASE_TYPE} ${DEFAULT_CHANNEL} ${CURRENT_VERSION}
	;;
patch-release)
	is_git_tree_clean
	is_current_branch_release
	${SCRIPT_DIR}/increment-version.bash --patch
	CURRENT_VERSION=$(get_current_version)
	BRANCH_NAME="patch_to_v${CURRENT_VERSION}"
	DEFAULT_CHANNEL="stable-v$(strip_patch ${CURRENT_VERSION})"
	make_branch ${BRANCH_NAME} ${RELEASE_TYPE} ${DEFAULT_CHANNEL} ${CURRENT_VERSION}
	;;
rc)
	is_git_tree_clean
	is_current_branch_main
	${SCRIPT_DIR}/increment-version.bash --rc ${RC_NUMBER}
	CURRENT_VERSION=$(get_current_version)
	BRANCH_NAME="v${CURRENT_VERSION}"
	DEFAULT_CHANNEL="candidate-v$(strip_patch ${CURRENT_VERSION})"
	make_branch ${BRANCH_NAME} ${RELEASE_TYPE} ${DEFAULT_CHANNEL} ${CURRENT_VERSION}
	;;
version-upgrade)
	is_git_tree_clean
	is_current_branch_main
	CURRENT_VERSION=$(get_current_version)
	BRANCH_NAME="update_to_v${CURRENT_VERSION}"
	DEFAULT_CHANNEL="fast-v$(strip_patch ${CURRENT_VERSION})"
	make_branch ${BRANCH_NAME} ${RELEASE_TYPE} ${DEFAULT_CHANNEL} ${CURRENT_VERSION}
	;;
*)
	echo "Unsupported branch type:'${RELEASE_TYPE}'"
	usage
	;;
esac
