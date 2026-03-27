#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -eu
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT=${SCRIPT_DIR%/*}
readonly CURRENT_VERSION=$(cat ${REPO_ROOT}/VERSION)

LAST_RC_TAG=$(git ls-remote --quiet --exit-code --tags --sort="-creatordate" | grep -E "refs/tags/spyre-v${CURRENT_VERSION}-rc\.[0-9]+$|refs/tags/v${CURRENT_VERSION}-rc\.[0-9]+-spyre$" | head -n 1 | awk '{print $2}')
if [ -z ${LAST_RC_TAG} ]; then
	echo "0"
	exit 0
fi

declare -a VA
VA=($(echo $LAST_RC_TAG | sed -r 's/refs\/tags\/(v|spyre)|(\.)|(-rc.)|(-spyre)/ /g'))

if [[ ${#VA[@]} -eq 3 ]]; then
	echo "0"
elif [[ ${#VA[@]} -eq 4 ]]; then
	echo "${VA[3]}"
else
	>&2 echo "Invalid number of elements in tag (${LAST_RC_TAG}). Expecting either 3 or 4, found ${#VA[@]}"
	exit 1
fi
