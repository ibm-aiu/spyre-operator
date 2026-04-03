#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright (c) 2025, 2026 IBM Corp.                                |
# | SPDX-License-Identifier: Apache-2.0                               |
# +-------------------------------------------------------------------+

set -eu -o pipefail
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly REPO_ROOT=${SCRIPT_DIR%/*}
readonly BOM=${REPO_ROOT}/release-artifacts.yaml
readonly YQ=${REPO_ROOT}/bin/yq
readonly YAMLFMT=${REPO_ROOT}/bin/yamlfmt
readonly VERSION=${1}
readonly REGISTRY=${2}
readonly DEFAULT_CHANNEL=${3}

if [ "x" == "x${VERSION}" ]; then
	echo "Error: Version needs to be supplied as the first argument."
	exit 1
fi

if [ "x" == "x${REGISTRY}" ]; then
	echo "Error: REGISTRY needs to be supplied as the second argument."
	exit 1
fi

if [ "x" == "x${DEFAULT_CHANNEL}" ]; then
	echo "Error: DEFAULT_CHANNEL needs to be supplied as the fourth argument."
	exit 1
fi

if [ ! -f ${YQ} ]; then
	make -f ${REPO_ROOT}/Makefile yq
fi

if [ ! -f ${YAMLFMT} ]; then
	make -f ${REPO_ROOT}/Makefile yamlfmt
fi

function image_tag() {
	local component=${1}
	local tag=$(${YQ} -r .${component}.version ${BOM})
	echo ${tag}
}

function propagate_version() {
	${YQ} eval -i ".images[0].newTag=\"${VERSION}\"" ${REPO_ROOT}/config/manager/kustomization.yaml
	${YQ} eval -i ".labels[0].pairs.operator-version=\"${VERSION}\"" ${REPO_ROOT}/config/manager/kustomization.yaml
	${YQ} eval -i ".spec.image=\"${REGISTRY}/spyre-operator-catalog:${VERSION}\"" ${REPO_ROOT}/config/olm/catalog-source.yaml
	${YQ} eval -i ".spec.channel=\"${DEFAULT_CHANNEL}\"" ${REPO_ROOT}/config/olm/subscription.yaml

	#patch version in the BOM
	${YQ} eval -i ".catalog.version=\"${VERSION}\"" ${BOM}
	${YQ} eval -i ".bundle.version=\"${VERSION}\"" ${BOM}
	${YQ} eval -i ".operator.version=\"${VERSION}\"" ${BOM}

	#patch cluster service with versions
	${YQ} eval -i ".spec.metricsExporter.version=\"$(image_tag exporter)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml
	${YQ} eval -i ".spec.devicePlugin.version=\"$(image_tag devicePlugin)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml
	${YQ} eval -i ".spec.devicePlugin.version=\"$(image_tag devicePlugin)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy_minimum.yaml
	${YQ} eval -i ".spec.devicePlugin.version=\"$(image_tag devicePlugin)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy_skip_components.yaml
	${YQ} eval -i ".spec.devicePlugin.initContainer.version=\"$(image_tag devicePluginInit)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml
	${YQ} eval -i ".spec.scheduler.version=\"$(image_tag scheduler)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml
	${YQ} eval -i ".spec.podValidator.version=\"$(image_tag podValidator)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml
	${YQ} eval -i ".spec.healthChecker.version=\"$(image_tag healthChecker)\"" ${REPO_ROOT}/config/samples/spyre_v1alpha1_spyreclusterpolicy.yaml

	#patch  test/config.yaml
	${YQ} eval -i ".operator.version=\"${VERSION}\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".catalog.version=\"${VERSION}\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".bundle.version=\"${VERSION}\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".exporter.version=\"$(image_tag exporter)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".mockUser.version=\"$(image_tag exporter)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".devicePlugin.version=\"$(image_tag devicePlugin)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".devicePluginInit.version=\"$(image_tag devicePluginInit)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".scheduler.version=\"$(image_tag scheduler)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".podValidator.version=\"$(image_tag podValidator)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".healthChecker.version=\"$(image_tag healthChecker)\"" ${REPO_ROOT}/test/config.yaml
	${YQ} eval -i ".defaultChannel=\"${DEFAULT_CHANNEL}\"" ${REPO_ROOT}/test/config.yaml

	# patch bundle annotation
	${YQ} eval -i ".annotations[\"operators.operatorframework.io.bundle.channels.v1\"]=\"${DEFAULT_CHANNEL}\"" ${REPO_ROOT}/bundle/metadata/annotations.yaml
	${YQ} eval -i ".annotations[\"operators.operatorframework.io.bundle.channel.default.v1\"]=\"${DEFAULT_CHANNEL}\"" ${REPO_ROOT}/bundle/metadata/annotations.yaml

	# format all modified yaml files
	${YAMLFMT} -conf=${REPO_ROOT}/.yamlfmt -dstar "${REPO_ROOT}/config/**/*.yaml" "${REPO_ROOT}/bundle/**/*.yaml" \
		${REPO_ROOT}/test/config.yaml ${REPO_ROOT}/release-artifacts.yaml

}

propagate_version
