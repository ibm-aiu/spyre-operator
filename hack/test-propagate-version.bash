#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
#
# Unit test for hack/propagate-version.bash
# Copies all files the script modifies into a temp directory and runs
# the script against them with a known version (7.7.7).
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

# ── setup ────────────────────────────────────────────────────────────────────

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

echo "Test temp directory: ${TMPDIR}"

# Mirror the directory structure expected by propagate-version.bash
mkdir -p \
	"${TMPDIR}/hack" \
	"${TMPDIR}/bin" \
	"${TMPDIR}/config/manager" \
	"${TMPDIR}/config/olm" \
	"${TMPDIR}/config/samples" \
	"${TMPDIR}/bundle/metadata" \
	"${TMPDIR}/test"

# Symlink the script under hack/ so SCRIPT_DIR / REPO_ROOT resolve correctly
ln -s "${SCRIPT_DIR}/propagate-version.bash" "${TMPDIR}/hack/propagate-version.bash"

# Symlink tool binaries and config
ln -s "${REPO_ROOT}/bin/yq" "${TMPDIR}/bin/yq"
ln -s "${REPO_ROOT}/bin/yamlfmt" "${TMPDIR}/bin/yamlfmt"
ln -s "${REPO_ROOT}/.yamlfmt" "${TMPDIR}/.yamlfmt"

# Copy all files that the script modifies
cp "${REPO_ROOT}/release-artifacts.yaml" "${TMPDIR}/release-artifacts.yaml"
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

# ── run the script ────────────────────────────────────────────────────────────

readonly TEST_VERSION="7.7.7"
readonly TEST_REGISTRY="test.registry.io"
readonly TEST_NAMESPACE="test-namespace"
readonly TEST_CHANNEL="stable-v7.7"

echo ""
echo "Running propagate-version.bash ${TEST_VERSION} ${TEST_REGISTRY} ${TEST_NAMESPACE} ${TEST_CHANNEL} ..."
# The script uses relative paths (e.g. config/manager/kustomization.yaml) that resolve
# against the current working directory, so we must cd into TMPDIR first.
(cd "${TMPDIR}" && "${TMPDIR}/hack/propagate-version.bash" \
	"${TEST_VERSION}" \
	"${TEST_REGISTRY}" \
	"${TEST_NAMESPACE}" \
	"${TEST_CHANNEL}")

# ── assertions ────────────────────────────────────────────────────────────────

echo ""
echo "=== Asserting config/manager/kustomization.yaml ==="
assert_eq "images[0].newTag" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.images[0].newTag' "${TMPDIR}/config/manager/kustomization.yaml")"
assert_eq "labels[0].pairs.operator-version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.labels[0].pairs.operator-version' "${TMPDIR}/config/manager/kustomization.yaml")"

echo ""
echo "=== Asserting config/olm/catalog-source.yaml ==="
assert_eq "spec.image" \
	"${TEST_REGISTRY}/${TEST_NAMESPACE}/spyre-operator-catalog:${TEST_VERSION}" \
	"$("${YQ}" -r '.spec.image' "${TMPDIR}/config/olm/catalog-source.yaml")"

echo ""
echo "=== Asserting config/olm/subscription.yaml ==="
assert_eq "spec.channel" \
	"${TEST_CHANNEL}" \
	"$("${YQ}" -r '.spec.channel' "${TMPDIR}/config/olm/subscription.yaml")"

echo ""
echo "=== Asserting release-artifacts.yaml (BOM) ==="
assert_eq "catalog.version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "bundle.version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/release-artifacts.yaml")"
assert_eq "operator.version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/release-artifacts.yaml")"

# Read component versions from the BOM (as updated by the script) for downstream assertions
EXPORTER_VERSION="$("${YQ}" -r '.exporter.version' "${TMPDIR}/release-artifacts.yaml")"
DEVICE_PLUGIN_VERSION="$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/release-artifacts.yaml")"
DEVICE_PLUGIN_INIT_VERSION="$("${YQ}" -r '.devicePluginInit.version' "${TMPDIR}/release-artifacts.yaml")"
SCHEDULER_VERSION="$("${YQ}" -r '.scheduler.version' "${TMPDIR}/release-artifacts.yaml")"
POD_VALIDATOR_VERSION="$("${YQ}" -r '.podValidator.version' "${TMPDIR}/release-artifacts.yaml")"
HEALTH_CHECKER_VERSION="$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/release-artifacts.yaml")"

echo ""
echo "=== Asserting config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml ==="
assert_eq "spec.metricsExporter.version" \
	"${EXPORTER_VERSION}" \
	"$("${YQ}" -r '.spec.metricsExporter.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml")"
assert_eq "spec.devicePlugin.version" \
	"${DEVICE_PLUGIN_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml")"
assert_eq "spec.devicePlugin.initContainer.version" \
	"${DEVICE_PLUGIN_INIT_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.initContainer.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml")"
assert_eq "spec.scheduler.version" \
	"${SCHEDULER_VERSION}" \
	"$("${YQ}" -r '.spec.scheduler.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml")"
assert_eq "spec.podValidator.version" \
	"${POD_VALIDATOR_VERSION}" \
	"$("${YQ}" -r '.spec.podValidator.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml")"
assert_eq "spec.healthChecker.version" \
	"${HEALTH_CHECKER_VERSION}" \
	"$("${YQ}" -r '.spec.healthChecker.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml")"

echo ""
echo "=== Asserting config/samples/spyre_v1alpha1_spyreclusterpolicy_minimum.yaml ==="
assert_eq "spec.devicePlugin.version" \
	"${DEVICE_PLUGIN_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy_minimum.yaml")"

echo ""
echo "=== Asserting config/samples/spyre_v1alpha1_spyreclusterpolicy_skip_components.yaml ==="
assert_eq "spec.devicePlugin.version" \
	"${DEVICE_PLUGIN_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy_skip_components.yaml")"

echo ""
echo "=== Asserting config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml ==="
assert_eq "spec.metricsExporter.version" \
	"${EXPORTER_VERSION}" \
	"$("${YQ}" -r '.spec.metricsExporter.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml")"
assert_eq "spec.devicePlugin.version" \
	"${DEVICE_PLUGIN_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml")"
assert_eq "spec.devicePlugin.initContainer.version" \
	"${DEVICE_PLUGIN_INIT_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.initContainer.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml")"
assert_eq "spec.scheduler.version" \
	"${SCHEDULER_VERSION}" \
	"$("${YQ}" -r '.spec.scheduler.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml")"
assert_eq "spec.podValidator.version" \
	"${POD_VALIDATOR_VERSION}" \
	"$("${YQ}" -r '.spec.podValidator.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml")"
assert_eq "spec.healthChecker.version" \
	"${HEALTH_CHECKER_VERSION}" \
	"$("${YQ}" -r '.spec.healthChecker.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.ppc64le.yaml")"

echo ""
echo "=== Asserting config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml ==="
assert_eq "spec.metricsExporter.version" \
	"${EXPORTER_VERSION}" \
	"$("${YQ}" -r '.spec.metricsExporter.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml")"
assert_eq "spec.devicePlugin.version" \
	"${DEVICE_PLUGIN_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml")"
assert_eq "spec.devicePlugin.initContainer.version" \
	"${DEVICE_PLUGIN_INIT_VERSION}" \
	"$("${YQ}" -r '.spec.devicePlugin.initContainer.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml")"
assert_eq "spec.scheduler.version" \
	"${SCHEDULER_VERSION}" \
	"$("${YQ}" -r '.spec.scheduler.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml")"
assert_eq "spec.podValidator.version" \
	"${POD_VALIDATOR_VERSION}" \
	"$("${YQ}" -r '.spec.podValidator.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml")"
assert_eq "spec.healthChecker.version" \
	"${HEALTH_CHECKER_VERSION}" \
	"$("${YQ}" -r '.spec.healthChecker.version' "${TMPDIR}/config/samples/spyre_v1alpha1_spyreclusterpolicy.s390x.yaml")"

echo ""
echo "=== Asserting test/config.yaml ==="
assert_eq "operator.version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.operator.version' "${TMPDIR}/test/config.yaml")"
assert_eq "catalog.version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.catalog.version' "${TMPDIR}/test/config.yaml")"
assert_eq "bundle.version" \
	"${TEST_VERSION}" \
	"$("${YQ}" -r '.bundle.version' "${TMPDIR}/test/config.yaml")"
assert_eq "exporter.version" \
	"${EXPORTER_VERSION}" \
	"$("${YQ}" -r '.exporter.version' "${TMPDIR}/test/config.yaml")"
assert_eq "mockUser.version" \
	"${EXPORTER_VERSION}" \
	"$("${YQ}" -r '.mockUser.version' "${TMPDIR}/test/config.yaml")"
assert_eq "devicePlugin.version" \
	"${DEVICE_PLUGIN_VERSION}" \
	"$("${YQ}" -r '.devicePlugin.version' "${TMPDIR}/test/config.yaml")"
assert_eq "devicePluginInit.version" \
	"${DEVICE_PLUGIN_INIT_VERSION}" \
	"$("${YQ}" -r '.devicePluginInit.version' "${TMPDIR}/test/config.yaml")"
assert_eq "scheduler.version" \
	"${SCHEDULER_VERSION}" \
	"$("${YQ}" -r '.scheduler.version' "${TMPDIR}/test/config.yaml")"
assert_eq "podValidator.version" \
	"${POD_VALIDATOR_VERSION}" \
	"$("${YQ}" -r '.podValidator.version' "${TMPDIR}/test/config.yaml")"
assert_eq "healthChecker.version" \
	"${HEALTH_CHECKER_VERSION}" \
	"$("${YQ}" -r '.healthChecker.version' "${TMPDIR}/test/config.yaml")"
assert_eq "defaultChannel" \
	"${TEST_CHANNEL}" \
	"$("${YQ}" -r '.defaultChannel' "${TMPDIR}/test/config.yaml")"

echo ""
echo "=== Asserting bundle/metadata/annotations.yaml ==="
assert_eq "bundle.channels.v1" \
	"${TEST_CHANNEL}" \
	"$("${YQ}" -r '.annotations["operators.operatorframework.io.bundle.channels.v1"]' "${TMPDIR}/bundle/metadata/annotations.yaml")"
assert_eq "bundle.channel.default.v1" \
	"${TEST_CHANNEL}" \
	"$("${YQ}" -r '.annotations["operators.operatorframework.io.bundle.channel.default.v1"]' "${TMPDIR}/bundle/metadata/annotations.yaml")"

# ── summary ───────────────────────────────────────────────────────────────────

echo ""
echo "────────────────────────────────────────────────────────────────────────"
echo "Results: ${PASS} passed, ${FAIL} failed"
echo "────────────────────────────────────────────────────────────────────────"

if [ "${FAIL}" -gt 0 ]; then
	exit 1
fi
