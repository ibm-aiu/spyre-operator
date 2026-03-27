#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025, 2026 All Rights Reserved                |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
set -eu -o pipefail
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"

echo "============ get-build-type.bash unit tests ============ "
echo -n "Testing for branch: main                     => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="main" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "development" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: release_v1.2.3           => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="release_v1.2.3" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "release" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: v1.2.3-rc.1              => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="v1.2.3-rc.1" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "release" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: release_v1.2.3_foo       => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="release_v1.2.3_foo" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: release_v1.2.3foo        => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="release_v1.2.3foo" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: relase_v1.2.3            => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="relase_v1.2.3" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: john-doe/release_v1.2.3  => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="john-doe/release_v1.2.3" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: john-doe/release_v1.2.3  => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="john-doe/release_v1.2.3" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: john-doe/some-branch     => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="john-doe/some-branch" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: some-branch              => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="some-branch" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: update_to_v1.2.3         => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="update_to_v1.2.3" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "pr" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo -n "Testing for branch: patch_to_v1.2.3         => "
ACTUAL_BUILD_TYPE=$(GIT_BRANCH_NAME="patch_to_v1.2.3" ${SCRIPT_DIR}/get-build-type.bash)
[[ ${ACTUAL_BUILD_TYPE} != "patch-release" ]] && echo "Fail, actual type = ${ACTUAL_BUILD_TYPE}" && exit 1
echo "Pass, actual type = ${ACTUAL_BUILD_TYPE}"

echo "========================================================"
