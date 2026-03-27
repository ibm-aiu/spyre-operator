#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
#
# Unit test for hack/create-branch.bash
#
# Copies the YAML files the script modifies into a temp directory and
# runs the script in dry-run mode (-d) with fake git, make, vi, and
# yamlfmt binaries so that no real repo mutations occur.
# The tests assert the YAML mutations performed by patch_artifacts and
# propagate-version.bash for each supported branch type.
#
set -eu -o pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT="${SCRIPT_DIR%/*}"
readonly YQ="${REPO_ROOT}/bin/yq"

# ── helpers ──────────────────────────────────────────────────────────────────

PASS=0
FAIL=0

pass() {
	echo "  PASS: $*"
	PASS=$((PASS + 1))
}
fail() {
	echo "  FAIL: $*"
	FAIL=$((FAIL + 1))
}

assert_eq() {
	local description="$1"
	local expected="$2"
	local actual="$3"
	if [ "${expected}" = "${actual}" ]; then
		pass "${description}"
	else
		fail "${description} — expected '${expected}', got '${actual}'"
	fi
}

assert_contains() {
	local description="$1"
	local expected="$2"
	local actual="$3"
	if echo "${actual}" | grep -qF "${expected}"; then
		pass "${description}"
	else
		fail "${description} — expected to contain '${expected}', got '${actual}'"
	fi
}

# ── setup ────────────────────────────────────────────────────────────────────

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

echo "Test temp directory: ${TMPDIR}"

# Mirror the directory structure expected by create-branch.bash.
# The script lives at hack/create-branch.bash and derives
# REPO_ROOT_DIR via ${SCRIPT_DIR%/*} (one level up).
mkdir -p \
	"${TMPDIR}/hack" \
	"${TMPDIR}/bin" \
	"${TMPDIR}/test" \
	"${TMPDIR}/config/manager" \
	"${TMPDIR}/config/olm" \
	"${TMPDIR}/config/samples" \
	"${TMPDIR}/bundle/metadata" \
	"${TMPDIR}/docs/api"

# Symlink the scripts under test and their dependencies
ln -s "${REPO_ROOT}/hack/create-branch.bash" "${TMPDIR}/hack/create-branch.bash"
ln -s "${REPO_ROOT}/hack/get-version.bash" "${TMPDIR}/hack/get-version.bash"
ln -s "${REPO_ROOT}/hack/increment-version.bash" "${TMPDIR}/hack/increment-version.bash"
ln -s "${REPO_ROOT}/hack/propagate-version.bash" "${TMPDIR}/hack/propagate-version.bash"
ln -s "${REPO_ROOT}/bin/yq" "${TMPDIR}/bin/yq"
ln -s "${REPO_ROOT}/bin/yamlfmt" "${TMPDIR}/bin/yamlfmt"

# Copy the .yamlfmt config file
cp "${REPO_ROOT}/.yamlfmt" "${TMPDIR}/.yamlfmt"

# Copy the YAML files that propagate-version.bash reads and modifies
cp "${REPO_ROOT}/config/manager/kustomization.yaml" "${TMPDIR}/config/manager/kustomization.yaml"
cp "${REPO_ROOT}/config/olm/catalog-source.yaml" "${TMPDIR}/config/olm/catalog-source.yaml"
cp "${REPO_ROOT}/config/olm/subscription.yaml" "${TMPDIR}/config/olm/subscription.yaml"
cp "${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml" "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml"
cp "${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy_minimum.yaml" "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy_minimum.yaml"
cp "${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy_skip_components.yaml" "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy_skip_components.yaml"
cp "${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml" "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml"
cp "${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml" "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml"
cp "${REPO_ROOT}/bundle/metadata/annotations.yaml" "${TMPDIR}/bundle/metadata/annotations.yaml"
cp "${REPO_ROOT}/test/config.yaml" "${TMPDIR}/test/config.yaml"

# Setup version specific file as input to the script
cat <<EOF >"${TMPDIR}/release-artifacts.yaml"
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
buildRepository: docker.io/spyre-operator
quayRepository: quay.io/ibm-aiu
channels:
  Stable:
    Bundles:
    - 1.0.0
    - 1.1.0
    - 1.1.1
  Candidates:
    Bundles:
    - 1.1.0-rc.1
    - 1.1.0-rc.2
    - 1.1.0-rc.3
  Fast:
    Bundles:
    - 1.0.0-dev
    - 1.1.0-dev
    - 1.1.1-dev
    - 1.2.0-dev
    - 7.7.0-dev
exporter:
  image: spyre-exporter
  version: 7.7.7-dev
  architectures:
  - amd64
  - ppc64le
catalog:
  image: spyre-operator-catalog
  version: 7.7.7-dev
bundle:
  image: spyre-operator-bundle
  version: 7.7.7-dev
operator:
  image: spyre-operator
  version: 7.7.7-dev
  architectures:
  - amd64
  - s390x
  - ppc64le
devicePlugin:
  image: spyre-device-plugin
  version: 7.7.7-dev
  architectures:
  - amd64
  - s390x
  - ppc64le
devicePluginInit:
  image: spyre-device-plugin-init
  version: 7.7.7-dev
  architectures:
  - amd64
  - s390x
  - ppc64le
scheduler:
  image: spyre-scheduler
  version: 7.7.7-dev
  architectures:
  - amd64
  - s390x
  - ppc64le
podValidator:
  image: spyre-webhook-validator
  version: 7.7.7-dev
  architectures:
  - amd64
  - s390x
  - ppc64le
healthChecker:
  image: spyre-health-checker
  version: 7.7.7-dev
  architectures:
  - amd64
  - s390x
  - ppc64le
EOF

# Fake git binary
# - show-ref returns 1 so the script believes the branch does not yet exist
# - status --porcelain returns empty (clean working tree)
# - all mutating commands (fetch, checkout, add, commit, push) are no-ops
# - branch / rev-parse honour GIT_BRANCH_NAME for version resolution
cat >"${TMPDIR}/bin/git" <<'EOF'
#!/bin/bash
case "$1" in
fetch)    exit 0 ;;
show-ref) exit 1 ;;   # branch does not exist — allows make_branch to proceed
checkout) exit 0 ;;
add)      exit 0 ;;
commit)   exit 0 ;;
push)     exit 0 ;;
status)   echo ""; exit 0 ;;   # clean working tree
branch)   echo "${GIT_BRANCH_NAME:-main}"; exit 0 ;;
rev-parse)
	if [[ "$2" == "--abbrev-ref" ]]; then
		echo "${GIT_BRANCH_NAME:-main}"
	elif [[ "$2" == "--short=7" ]]; then
		echo "${GIT_SHORT_HASH:-abc1234}"
	fi
	exit 0 ;;
esac
exit 0
EOF
chmod +x "${TMPDIR}/bin/git"

# Fake make — no-op for most targets; print-REGISTRY and print-NAMESPACE return
# the values that propagate-version.bash requires.
cat >"${TMPDIR}/bin/make" <<'EOF'
#!/bin/bash
for arg in "$@"; do
	case "${arg}" in
	print-REGISTRY)  echo "REGISTRY = docker.io"; exit 0 ;;
	print-NAMESPACE) echo "NAMESPACE = spyre-operator"; exit 0 ;;
	esac
done
exit 0
EOF
chmod +x "${TMPDIR}/bin/make"

# Fake vi editor — no-op (patch_artifacts already modified the file before the editor opens)
cat >"${TMPDIR}/bin/vi" <<'EOF'
#!/bin/bash
exit 0
EOF
chmod +x "${TMPDIR}/bin/vi"

# Prepend our fake bin/ so all fakes shadow the real binaries
export PATH="${TMPDIR}/bin:${PATH}"

# Fixed git overrides used across all tests
export GIT_SHORT_HASH="abc1234"

# Minimal Makefile — only needs a DEFAULT_CHANNEL line for the sed substitution
cat >"${TMPDIR}/Makefile" <<'EOF'
DEFAULT_CHANNEL		?=fast-v0.0
EOF

# Preserve originals for reset between test runs
readonly ORIG_ARTIFACT_CONFIG="${TMPDIR}/release-artifacts.yaml.orig"
readonly ORIG_TEST_CONFIG="${TMPDIR}/test/config.yaml.orig"
readonly ORIG_MAKEFILE="${TMPDIR}/Makefile.orig"
readonly ORIG_CONFIG_MANAGER_KUSTOMIZATION="${TMPDIR}/config/manager/kustomization.yaml.orig"
readonly ORIG_CONFIG_OLM_CATALOG="${TMPDIR}/config/olm/catalog-source.yaml.orig"
readonly ORIG_CONFIG_OLM_SUBSCRIPTION="${TMPDIR}/config/olm/subscription.yaml.orig"
readonly ORIG_BUNDLE_ANNOTATIONS="${TMPDIR}/bundle/metadata/annotations.yaml.orig"
cp "${TMPDIR}/release-artifacts.yaml" "${ORIG_ARTIFACT_CONFIG}"
cp "${TMPDIR}/test/config.yaml" "${ORIG_TEST_CONFIG}"
cp "${TMPDIR}/Makefile" "${ORIG_MAKEFILE}"
cp "${TMPDIR}/config/manager/kustomization.yaml" "${ORIG_CONFIG_MANAGER_KUSTOMIZATION}"
cp "${TMPDIR}/config/olm/catalog-source.yaml" "${ORIG_CONFIG_OLM_CATALOG}"
cp "${TMPDIR}/config/olm/subscription.yaml" "${ORIG_CONFIG_OLM_SUBSCRIPTION}"
cp "${TMPDIR}/bundle/metadata/annotations.yaml" "${ORIG_BUNDLE_ANNOTATIONS}"

# ── helpers ───────────────────────────────────────────────────────────────────

reset_configs() {
	cp "${ORIG_ARTIFACT_CONFIG}" "${TMPDIR}/release-artifacts.yaml"
	cp "${ORIG_TEST_CONFIG}" "${TMPDIR}/test/config.yaml"
	cp "${ORIG_MAKEFILE}" "${TMPDIR}/Makefile"
	cp "${ORIG_CONFIG_MANAGER_KUSTOMIZATION}" "${TMPDIR}/config/manager/kustomization.yaml"
	cp "${ORIG_CONFIG_OLM_CATALOG}" "${TMPDIR}/config/olm/catalog-source.yaml"
	cp "${ORIG_CONFIG_OLM_SUBSCRIPTION}" "${TMPDIR}/config/olm/subscription.yaml"
	cp "${ORIG_BUNDLE_ANNOTATIONS}" "${TMPDIR}/bundle/metadata/annotations.yaml"
	# sed -i.bak creates Makefile.bak next to the Makefile; the script then
	# does `rm Makefile.bak` (relative). Pre-touch it so the rm never fails
	# even if sed exits before writing the backup.
	touch "${TMPDIR}/Makefile.bak"
}

# Run the script from TMPDIR in dry-run mode so git push is skipped.
# Caller sets VERSION file and GIT_BRANCH_NAME before calling.
#
# Exit-code note: the last statement in make_branch is:
#   [[ "x" == "x${DRY_RUN}" ]] && git push ...
# When DRY_RUN=TRUE the condition is false, so the script exits 1.
# Exit codes 0 and 1 are therefore both acceptable; anything higher
# indicates a genuine error.
run_script() {
	local rc=0
	(cd "${TMPDIR}" && "${TMPDIR}/hack/create-branch.bash" -d "$@") || rc=$?
	if [[ ${rc} -gt 1 ]]; then
		echo "  ERROR: create-branch.bash exited with unexpected code ${rc}" >&2
		exit "${rc}"
	fi
}

# ── Branch-type tests ─────────────────────────────────────────────────────────
#
# patch_artifacts behaviour per branch type:
#
#   minor-release  → channels.Stable.Bundles   += current version
#                    component versions         = current version
#                    test/config.yaml channel   = stable-v<short>
#
#   major-release  → same as minor-release
#
#   patch-release  → channels.Stable.Bundles   += current version
#					 channels.Fast.Bundles     += current version - dev
#                    component versions         = image_tag
#                    test/config.yaml channel   = stable-v<short>
#
#   rc             → channels.Candidates.Bundles += current version.rc.n
#                    component versions           = current version.rc.n
#                    test/config.yaml channel     = candidate-v<short>
#
#   version-upgrade → channels.Fast.Bundles     += current version-dev
#                     operator/catalog/bundle    = current version-dev
#                     component versions         = current version-dev
#                     test/config.yaml channel   = fast-v<short>
#
# The test will not assert that all the version has been propagated correctly to all files,
# as it being tested separately.

echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo "Branch-type tests"
echo "════════════════════════════════════════════════════════════════════════"

# ── Scenario 1: minor-release ─────────────────────────────────────────────────
# VERSION=7.7.7 → branch release_v7.7.7, channel stable-v7.7
# get-version.bash -t base -v 7.7.7 on release_v7.7.7 → 7.7.7

echo ""
echo "--- Scenario: minor-release (VERSION=7.7.0) ---"
reset_configs
echo "7.7.0" >"${TMPDIR}/VERSION"
export GIT_BRANCH_NAME="release_v7.7.0"
run_script -t minor-release
unset GIT_BRANCH_NAME

assert_contains "minor-release: channels.Stable.Bundles contains 7.7.0" \
	"7.7.0" \
	"$("${YQ}" -r '.channels.Stable.Bundles[]' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: defaultChannel" \
	"stable-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"
assert_eq "minor-release: catalog.version" \
	"7.7.0" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: bundle.version" \
	"7.7.0" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: operator.version" \
	"7.7.0" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: devicePlugin.version" \
	"7.7.0" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: exporter.version" \
	"7.7.0" \
	"$("${YQ}" -r '.exporter.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: scheduler.version" \
	"7.7.0" \
	"$("${YQ}" -r '.scheduler.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: podValidator.version" \
	"7.7.0" \
	"$("${YQ}" -r '.podValidator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: devicePluginInit.version" \
	"7.7.0" \
	"$("${YQ}" -r '.devicePluginInit.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: healthChecker.version" \
	"7.7.0" \
	"$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/release-artifacts.yaml")"

assert_eq "minor-release: testConfig .defaultChannel" \
	"stable-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"

# ── Scenario 2: major-release ─────────────────────────────────────────────────
# Same patch_artifacts path as minor-release

echo ""
echo "--- Scenario: major-release (VERSION=7.0.0) ---"
reset_configs
echo "7.0.0" >"${TMPDIR}/VERSION"
export GIT_BRANCH_NAME="release_v7.0.0"
run_script -t major-release
unset GIT_BRANCH_NAME

assert_contains "major-release: channels.Stable.Bundles contains 7.0.0" \
	"7.0.0" \
	"$("${YQ}" -r '.channels.Stable.Bundles[]' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: defaultChannel" \
	"stable-v7.0" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"
assert_eq "major-release: catalog.version" \
	"7.0.0" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: bundle.version" \
	"7.0.0" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: operator.version" \
	"7.0.0" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: devicePlugin.version" \
	"7.0.0" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: exporter.version" \
	"7.0.0" \
	"$("${YQ}" -r '.exporter.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: scheduler.version" \
	"7.0.0" \
	"$("${YQ}" -r '.scheduler.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: podValidator.version" \
	"7.0.0" \
	"$("${YQ}" -r '.podValidator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: devicePluginInit.version" \
	"7.0.0" \
	"$("${YQ}" -r '.devicePluginInit.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "major-release: healthChecker.version" \
	"7.0.0" \
	"$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "minor-release: testConfig .defaultChannel" \
	"stable-v7.0" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"

# ── Scenario 3: patch-release ─────────────────────────────────────────────────
# increment-version.bash --patch on 7.7.7 → 7.7.8
# branch = patch_to_v7.7.8, channel = stable-v7.7
# get-version.bash -t base -v 7.7.8 on release_v7.7.8 → 7.7.8
echo ""
echo "--- Scenario: patch-release (VERSION=7.7.7 → 7.7.8 after increment) ---"
reset_configs
echo "7.7.7" >"${TMPDIR}/VERSION"
export GIT_BRANCH_NAME="patch_to_v7.7.8"
run_script -t patch-release
unset GIT_BRANCH_NAME

assert_contains "patch-release: channels.Stable.Bundles contains 7.7.8" \
	"7.7.8" \
	"$("${YQ}" -r '.channels.Stable.Bundles[]' "${TMPDIR}/release-artifacts.yaml")"
assert_contains "patch-release: channels.Fast.Bundles contains 7.7.8-dev" \
	"7.7.8" \
	"$("${YQ}" -r '.channels.Fast.Bundles[]' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: defaultChannel" \
	"stable-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"
assert_eq "patch-release: catalog.version" \
	"7.7.8" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: bundle.version" \
	"7.7.8" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: operator.version" \
	"7.7.8" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/release-artifacts.yaml")"
# The expectation is that for the components not related to the operator they will
# be untouched by the script, unless the user edits them when presented with the option.
# For operator only patch releases, it is expected that the component version will not change.
# For operator and one more component, the user will first set the component version to the <M>.<m>.<p>-dev version,
# run the tests, release the component then, updates the component version in the file.
assert_eq "patch-release: devicePlugin.version" \
	"7.7.7-dev" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: exporter.version" \
	"7.7.7-dev" \
	"$("${YQ}" -r '.exporter.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: scheduler.version" \
	"7.7.7-dev" \
	"$("${YQ}" -r '.scheduler.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: podValidator.version" \
	"7.7.7-dev" \
	"$("${YQ}" -r '.podValidator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: devicePluginInit.version" \
	"7.7.7-dev" \
	"$("${YQ}" -r '.devicePluginInit.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: healthChecker.version" \
	"7.7.7-dev" \
	"$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "patch-release: testConfig .defaultChannel" \
	"stable-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"

# ── Scenario 4: rc ────────────────────────────────────────────────────────────
# increment-version.bash --rc 1 on 7.7.7-rc.1 (RC_NUMBER=1) → ((++RC_NUMBER))=2 → 7.7.7-rc.2
# branch = v7.7.7-rc.2, channel = candidate-v7.7
# get-version.bash -t base -v 7.7.7-rc.2 on v7.7.7-rc.2 → 7.7.7-rc.2

echo ""
echo "--- Scenario: rc (VERSION=7.7.7, RC_NUMBER=1 → 7.7.7-rc.2 after increment) ---"
reset_configs
echo "7.7.7-rc.1" >"${TMPDIR}/VERSION"
export GIT_BRANCH_NAME="v7.7.7-rc.2"
run_script -t rc 1
unset GIT_BRANCH_NAME

assert_contains "rc: channels.Candidates.Bundles contains 7.7.7-rc.2" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.channels.Candidates.Bundles[]' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: defaultChannel" \
	"candidate-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"
assert_eq "rc: devicePlugin.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: catalog.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: bundle.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: operator.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: devicePlugin.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: exporter.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.exporter.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: scheduler.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.scheduler.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: podValidator.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.podValidator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: devicePluginInit.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.devicePluginInit.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: healthChecker.version" \
	"7.7.7-rc.2" \
	"$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "rc: testConfig .defaultChannel" \
	"candidate-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"

# ── Scenario 5: version-upgrade ───────────────────────────────────────────────
# VERSION=7.7.8 → branch update_to_v7.7.8, channel fast-v7.7
# get-version.bash -t base -v 7.7.8 on update_to_v7.7.8 → 7.7.8-dev

echo ""
echo "--- Scenario: version-upgrade (VERSION=7.7.8) ---"
reset_configs
echo "7.7.8" >"${TMPDIR}/VERSION"
export GIT_BRANCH_NAME="update_to_v7.7.8"
run_script -t version-upgrade
unset GIT_BRANCH_NAME

assert_contains "version-upgrade: channels.Fast.Bundles contains 7.7.8-dev" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.channels.Fast.Bundles[]' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: defaultChannel" \
	"fast-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"
assert_eq "version-upgrade: operator.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: catalog.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: bundle.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: devicePlugin.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: exporter.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.exporter.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: scheduler.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.scheduler.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: podValidator.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.podValidator.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: healthChecker.version" \
	"7.7.8-dev" \
	"$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "version-upgrade: testConfig .defaultChannel" \
	"fast-v7.7" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"

# ── summary ───────────────────────────────────────────────────────────────────

echo ""
echo "────────────────────────────────────────────────────────────────────────"
echo "Results: ${PASS} passed, ${FAIL} failed"
echo "────────────────────────────────────────────────────────────────────────"

if [ "${FAIL}" -gt 0 ]; then
	exit 1
fi
