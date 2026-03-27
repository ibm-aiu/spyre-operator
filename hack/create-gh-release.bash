#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -eu
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT=${SCRIPT_DIR%/*}
readonly GIT=$(command -v git)
readonly GH=$(command -v gh)
readonly ZIP=$(command -v zip)
BUILD_TYPE=""
VERSION=""

function usage() {
	echo "Usage: ${0} flags"
	echo "Flags:"
	echo "  -b, --build-type   <build type>"
	echo "  -v, --version       <version> "
	echo "  -h, --help prints this message"
	exit 2
}

function validate_args() {
	if [ "x" == "x${BUILD_TYPE}" ]; then
		echo "Error: Build type needs to be supplied as the first argument."
		exit 1
	fi

	if [ "x" == "x${VERSION}" ]; then
		echo "Error: Version needs to be supplied as the second argument."
		exit 1
	fi

	if [[ ${BUILD_TYPE} != "release" ]]; then
		echo "Error: a github release can only be created on a release branch"
		exit 1
	fi
}

function validate_environment() {
	if [ "x" == "x${GIT}" ]; then
		echo "Error: GIT must have a value, git needs to be available in your path"
		exit 1
	fi

	if [ "x" == "x${GH}" ]; then
		echo "Error: GH must have a value, gh needs to be available in your path"
		exit 1
	fi

	if [ "x" == "x${ZIP}" ]; then
		echo "Error: ZIP must have a value, zip needs to be available in your path"
		exit 1
	fi
}
function get_last_release_tag() {
	local last_release_tag=$(${GIT} ls-remote --quiet --exit-code --tags --sort="-creatordate" | grep -E "refs/tags/spyre-v([0-9]|\.)+$|refs/tags/v([0-9]|\.)+(-spyre)$" | head -n 1 | awk '{gsub ("refs/tags/", "", $2); print $2}')
	if [ -z ${last_release_tag} ]; then
		echo "NONE"
		return
	fi
	echo ${last_release_tag}
}

if [[ ${#} == 0 ]]; then
	usage
fi

declare -a POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
	case ${1} in
	-b | --build-type)
		BUILD_TYPE="${2}"
		shift # past argument
		shift # past value
		;;
	-v | --version)
		VERSION="${2}"
		shift # past argument
		shift # past value
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
validate_args

LAST_RELEASE_TAG=$(get_last_release_tag)
GIT_TAG="v${VERSION}-spyre"
GH_RELEASE_TITLE="v${VERSION}-spyre"

echo "Creating git tag: '${GIT_TAG}'"
${GIT} fetch origin --tags --quiet
LAST_RELEASE_TAG=$(get_last_release_tag)
if ${GIT} rev-parse -q --verify "refs/tags/${GIT_TAG}" 1>/dev/null; then
	echo "Error: tag '${GIT_TAG}' already exists"
	exit 1
fi
${GIT} tag ${GIT_TAG} --annotate -m "Created tag ${GIT_TAG} for release "
${GIT} push -q origin tag ${GIT_TAG}

echo "Creating github release: '${GH_RELEASE_TITLE}'"
echo "Last release tag set to '${LAST_RELEASE_TAG}'"
if [[ "NONE" == ${LAST_RELEASE_TAG} ]]; then
	${GH} release create ${GIT_TAG} -t ${GH_RELEASE_TITLE} --verify-tag --latest --generate-notes --draft
else
	${GH} release create ${GIT_TAG} -t ${GH_RELEASE_TITLE} --verify-tag --latest --notes-start-tag ${LAST_RELEASE_TAG} --generate-notes --draft
fi
if [ -d ${REPO_ROOT}/twistlock-scan-output ]; then
	echo "Zipping Twistlock scan results for release..."
	${ZIP} -r ${REPO_ROOT}/twistlock-scan-results.zip ${REPO_ROOT}/twistlock-scan-output/
	${GH} release upload ${GIT_TAG} twistlock-scan-results.zip#twistlock-scan-results.zip --clobber
else
	echo "Directory '${REPO_ROOT}/twistlock-scan-output' not found, twistlock-scan-results.zip not attached to the release"
fi
