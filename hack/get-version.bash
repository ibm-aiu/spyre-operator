#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025, 2026 All Rights Reserved                |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
# Summary
# -------
# Computes a Docker-image-tag-friendly version string from a base VERSION and
# the current git branch name.
#
# Usage:
#   get-version.bash -t <type> -v <version>
#
#   -t base      Return a version without a git-hash suffix.
#   -t relative  Return a version that may include a short git-hash suffix.
#   -v <version> The base semver string (e.g. "1.2.3" or "1.2.3-rc.1").
#
# Branch classification and output:
#
#   Branch pattern                    | base output       | relative output
#   ----------------------------------|-------------------|--------------------
#   release_v<M>.<m>.<p>              | <version>         | <version>
#   main                              | <version>-dev     | <version>-dev
#   v<M>.<m>.<p>-rc.<n>               | <M>.<m>.<p>-rc.<n>| <M>.<m>.<p>-rc.<n>
#   patch_to_v<M>.<m>.<p>             | <version>         | <version>-dev
#   anything else (PR / feature /     | <version>-dev     | <version>-dev-<hash>
#     v<M>.<m> / …)                   |                   |
#
# The git short-hash and branch name can be overridden via the environment
# variables GIT_SHORT_HASH and GIT_BRANCH_NAME respectively (used in tests).
#
set -eu
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly GIT=$(command -v git)
VERSION_TYPE=""
VERSION=""

function usage() {
	echo "Usage: ${0} flags"
	echo "Flags:"
	echo "  -t, --type   base|relative"
	echo "  -v, --version value"
	echo "  -h, --help prints this message"
	exit 2
}

function validate_args() {
	if [ "x" == "x${VERSION_TYPE}" ]; then
		echo "Error: Build type needs to be supplied as the first argument."
		exit 1
	fi

	if [ "x" == "x${VERSION}" ]; then
		echo "Error: Version needs to be supplied as the second argument."
		exit 1
	fi

	if [[ ${VERSION_TYPE} != "base" && ${VERSION_TYPE} != "relative" ]]; then
		echo "Error: type can only have the values of 'base' or 'relative'."
		exit 1
	fi
}

function validate_environment() {
	if [ "x" == "x${GIT}" ]; then
		echo "Error: GIT must have a value, git needs to be available in your path"
		exit 1
	fi
}

function branch_name_check() {
	local branch_name=${1}
	local current_version=${2}
	local version_type=${3}
	local short_hash=${4}
	if [[ ${branch_name} =~ ^release_v[0-9]+(\.[0-9]+)+$ ||
		${branch_name} =~ ^v[0-9](\.[0-9]+)+-rc\.[0-9]+$ ]]; then
		echo ${current_version}
	elif [[ ${branch_name} == "main" ]]; then
		echo ${current_version}-dev
	elif [[ ${branch_name} =~ ^patch_to_v[0-9]+(\.[0-9]+)+$ ]]; then
		if [[ ${version_type} == "base" ]]; then
			echo ${current_version}
		else
			echo ${current_version}-dev
		fi
	else
		if [[ ${version_type} == "base" ]]; then
			echo ${current_version}-dev
		else
			echo ${current_version}-dev-${short_hash}
		fi
	fi
}

if [[ ${#} == 0 ]]; then
	usage
fi

declare -a POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
	case ${1} in
	-t | --type)
		VERSION_TYPE="${2}"
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

SHORT_HASH=${GIT_SHORT_HASH:-$(git rev-parse --short=7 HEAD)}
BRANCH_NAME=${GIT_BRANCH_NAME:-$(git branch --show-current)}
if [[ -z ${BRANCH_NAME} ]]; then
	BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
fi
branch_name_check ${BRANCH_NAME} ${VERSION} ${VERSION_TYPE} ${SHORT_HASH}
