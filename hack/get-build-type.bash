#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025, 2026 All Rights Reserved                |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -e
BRANCH_NAME=${GIT_BRANCH_NAME:-}
if [[ -z ${BRANCH_NAME} ]]; then
	BRANCH_NAME=$(git branch --show-current)
fi
if [[ -z ${BRANCH_NAME} ]]; then
	BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
fi

if [[ ${BRANCH_NAME} =~ ^release_v[0-9]+(\.[0-9]+)+$ ||
	${BRANCH_NAME} =~ ^v[0-9](\.[0-9]+)+-rc\.[0-9]+$ ]]; then
	echo "release"
elif [[ ${BRANCH_NAME} == "main" ]]; then
	echo "development"
elif [[ ${BRANCH_NAME} =~ ^patch_to_v[0-9]+(\.[0-9]+)+$ ]]; then
	echo "patch-release"
else
	echo "pr"
fi
