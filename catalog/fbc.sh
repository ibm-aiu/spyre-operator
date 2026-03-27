#!/bin/bash

# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT_DIR=${SCRIPT_DIR%/*}
TEMPLATE=${SCRIPT_DIR}/base_template.yaml
ARTIFACT_CONFIG=${REPO_ROOT_DIR}/release-artifacts.yaml
CURRENT_VERSION=$(cat ${REPO_ROOT_DIR}/VERSION)

set -eu -o pipefail
trap exit_all INT

# create a barebone catalog semver template
function base_template() {
	cat >"${SCRIPT_DIR}"/base_template.yaml <<EOF
    Schema: olm.semver
    GenerateMajorChannels: false
    GenerateMinorChannels: true
    Fast:
        Bundles: []
    Candidate:
        Bundles: []
    Stable:
        Bundles: []
EOF
}

# create a sample catalog source with the new catalog image
function gen_catalog_source() {
	local cs="${SCRIPT_DIR}/catalog-source.yaml"
	cat >"${cs}" <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ibm-spyre-operators
  namespace: openshift-marketplace
spec:
  displayName: IBM Spyre Operator
  publisher: IBM
  sourceType: grpc
  image: ${IMG}
  updateStrategy:
    registryPoll:
      interval: 10m
EOF
	echo "Generated sample CatalogSource ${cs}"
}

# generate a fbc template which is used by opm alpha render-template <template>
function create_template() {
	base_template

	if [[ ${BUILD_TYPE} == "pr" ]]; then
		#for pr builds we only need a catalog for the current branch bundle
		return
	fi
	# Add Fast: 2.y.z-dev-xxxxx
	for tag in ${FAST_TAGS}; do
		BUNDLE_IMG=${REPO}:${tag} yq -i '.Fast.Bundles += {"Image":strenv(BUNDLE_IMG)}' "${TEMPLATE}"
	done

	# Add candidates: 2.y.z-rc.x
	for tag in ${CANDIDATE_TAGS}; do
		BUNDLE_IMG=${REPO}:${tag} yq -i '.Candidate.Bundles += {"Image":strenv(BUNDLE_IMG)}' "${TEMPLATE}"
	done

	# Add stable: 2.y.z
	for tag in ${STABLE_TAGS}; do
		# this condition is necessary as in the case of a patch release build (branch patch_to_v<M>.<m>.<p>)
		# the release artifacts yaml will contain a bundle that has not yet been build, therefore do not include
		# it in the catalog
		if [[ ${BUILD_TYPE} == "patch-release" && ${tag} == ${CURRENT_VERSION} ]]; then
			continue
		fi
		BUNDLE_IMG=${REPO}:${tag} yq -i '.Stable.Bundles += {"Image":strenv(BUNDLE_IMG)}' "${TEMPLATE}"
	done
}
# check if the bundle image exists in the image list in
# the template
function bundle_exists_in_template() {
	local bundles=${1}
	local bundle_exists="false"
	for bundle in $(yq ${bundles} "${TEMPLATE}"); do
		if [[ ${bundle} == "${BUNDLE_IMG}" ]]; then
			bundle_exists="true"
		fi
	done
	echo ${bundle_exists}
}
# Add the current bundle into the fbc template
function add_bundle() {
	if [[ "false" == $(bundle_exists_in_template ".Fast.Bundles[].Image") ]]; then
		BUNDLE_IMG=${BUNDLE_IMG=} yq -i '.Fast.Bundles += {"Image":strenv(BUNDLE_IMG)}' "${TEMPLATE}"
	fi
	if [[ ${CANDIDATE} == "true" && "false" == $(bundle_exists_in_template ".Candidate.Bundles[].Image") ]]; then
		BUNDLE_IMG=${BUNDLE_IMG=} yq -i '.Candidate.Bundles += {"Image":strenv(BUNDLE_IMG)}' "${TEMPLATE}"
	fi
	if [[ ${STABLE} == "true" && "false" == $(bundle_exists_in_template ".Stable.Bundles[].Image") ]]; then
		BUNDLE_IMG=${BUNDLE_IMG=} yq -i '.Stable.Bundles += {"Image":strenv(BUNDLE_IMG)}' "${TEMPLATE}"
	fi
}

# Update the final fbc yaml
function update_fbc_yaml() {
	# Add icon to the operator package
	source "${SCRIPT_DIR}"/catalog_icon.sh
	icon=${icon} yq -i 'select(.schema == "olm.package") += {"icon":{"base64data":strenv(icon),"mediatype":"image/png" }}' "${FBC}"
	echo "Added icon to fbc"
}

# Make sure downloaded yq binary is used
function insure_yq() {
	[ ! -x "${REPO_ROOT_DIR}"/bin/yq ] && cd "${REPO_ROOT_DIR}" && make yq && cd -
	export PATH=${REPO_ROOT_DIR}/bin/:${PATH}
}

insure_yq

action=$1

case $action in
"template")
	# The current released stable bundle
	STABLE_TAGS=$(yq '.channels.Stable.Bundles[]' "${ARTIFACT_CONFIG}")

	# The current released rc.x bundles to be included in the FBC
	CANDIDATE_TAGS=$(yq '.channels.Candidates.Bundles[]' "${ARTIFACT_CONFIG}")

	# The current released bundles to be included in the FBC
	FAST_TAGS=$(yq '.channels.Fast.Bundles[]' "${ARTIFACT_CONFIG}")

	[ -z "${STABLE_TAGS}" ] && [ -z "${CANDIDATE_TAGS}" ] && [ -z "${FAST_TAGS}" ] &&
		echo "No bundles specified in the ${ARTIFACT_CONFIG}." && exit 1

	BUILD_TYPE=${2:-}
	[ -z "${BUILD_TYPE}" ] && echo "Please provide a build type" && exit 1

	# Bundle image repo i.e.: docker.io/spyre-operator/spyre-operator-bundle
	REPO=${3:-}
	[ -z "${REPO}" ] && echo "Please provide an bundle image name" && exit

	create_template
	;;
"add_bundle")
	STABLE=${STABLE:-false}
	CANDIDATE=${CANDIDATE:-false}
	BUNDLE_IMG=${2:-}
	[ -z "${BUNDLE_IMG}" ] && echo "Please provide a bundle image." && exit 1
	add_bundle
	;;
"update_fbc")
	FBC=${2:-}
	[ -z "${FBC}" ] && echo "Please generate a file based catalog yaml first." && exit 1
	update_fbc_yaml
	;;
"gen_catalogsource_cr")
	IMG=${2:-}
	[ -z "${IMG}" ] && echo "Please provide the catalog source image." && exit 1
	gen_catalog_source
	;;
*)
	echo "Usage: $0 {template|add_bundle|update_fbc|gen_catalogsource_cr}"
	exit 1
	;;
esac
