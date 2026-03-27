#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025,2026 All Rights Reserved                 |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
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

set -eu -o pipefail
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly FIXED_HASH="abc1234"
readonly VERSION="1.2.3"

echo "============ get-version.bash unit tests ============ "

# --- Release branches (type: base) ---

echo -n "Testing branch: release_v1.2.3,       type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="release_v1.2.3" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: release_v1.2.3,       type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="release_v1.2.3" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: v1.2.3-rc.1,          type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="v1.2.3-rc.1" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION}-rc.1)
[[ ${ACTUAL} != "${VERSION}-rc.1" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: v1.2.3-rc.1,          type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="v1.2.3-rc.1" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION}-rc.1)
[[ ${ACTUAL} != "${VERSION}-rc.1" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

# --- Development branches ---

echo -n "Testing branch: main,                 type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="main" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: main,                 type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="main" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: patch_to_v1.2.3,      type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="patch_to_v1.2.3" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: patch_to_v1.2.3,      type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="patch_to_v1.2.3" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

# --- Feature / PR branches ---

echo -n "Testing branch: some-feature-branch,  type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="some-feature-branch" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: some-feature-branch,  type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="some-feature-branch" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev-${FIXED_HASH}" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: john-doe/my-fix,      type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="john-doe/my-fix" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: john-doe/my-fix,      type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="john-doe/my-fix" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev-${FIXED_HASH}" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

# --- Special PR branches ---

echo -n "Testing branch: update_to_v1.2.3,     type=base     => "
ACTUAL=$(GIT_BRANCH_NAME="update_to_v1.2.3" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t base -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo -n "Testing branch: update_to_v1.2.3,     type=relative => "
ACTUAL=$(GIT_BRANCH_NAME="update_to_v1.2.3" GIT_SHORT_HASH="${FIXED_HASH}" ${SCRIPT_DIR}/get-version.bash -t relative -v ${VERSION})
[[ ${ACTUAL} != "${VERSION}-dev-${FIXED_HASH}" ]] && echo "Fail, actual = ${ACTUAL}" && exit 1
echo "Pass, actual = ${ACTUAL}"

echo "====================================================="
