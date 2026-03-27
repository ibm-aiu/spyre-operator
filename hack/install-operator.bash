#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

## Usage:
#   make install-operator
#   make install-operator CATALOG_IMG=docker.io/spyre-operator/spyre-operator-catalog:0.3.0 CHANNELS=stable-v0.3

set -eu -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
readonly SCRIPT_DIR
readonly REPO_ROOT=${SCRIPT_DIR%/*}
YQ=${REPO_ROOT}/bin/yq
readonly YQ
KUBECTL=$(command -v oc 2>/dev/null || echo kubectl)
readonly KUBECTL

function usage() {
	echo "Usage: $(basename $0) install [OPERATOR_NAMESPACE] [CATALOG_IMG] [CHANNELS]"
	echo "       $(basename $0) uninstall [OPERATOR_NAMESPACE]"
	exit 2
}

# Check if at least one argument is provided
if [[ $# -lt 2 ]]; then
	usage
fi

# Default ARCH to empty if not set
ARCH="${ARCH:-}"

# Check if 'go' is installed
if ! [ -x "$(command -v go)" ]; then
	# 'go' not found; check if ARCH is set
	if [[ -z $ARCH ]]; then
		echo "Need to set ARCH or have 'go' installed"
		exit 1
	fi
else
	# 'go' is found; set ARCH from go env if ARCH not set
	if [[ -z $ARCH ]]; then
		ARCH=$(go env GOARCH)
	fi
fi

readonly ARCH

function install_operator() {
	CLUSTER_POLICY_FILE=spyre_v1alpha1_spyreclusterpolicy.yaml
	if [[ ${ARCH} != "amd64" ]]; then
		CLUSTER_POLICY_FILE=spyre_v1alpha1_spyreclusterpolicy.${ARCH}.yaml
	fi
	CLUSTER_POLICY_FULL_PATH=${REPO_ROOT}/config/samples/${CLUSTER_POLICY_FILE}
	if [ ! -f "$CLUSTER_POLICY_FULL_PATH" ]; then
		echo "[ERROR] File '$CLUSTER_POLICY_FULL_PATH' does not exist."
		return 1
	fi

	echo "[STEP 1 of 4] Creating ${OPERATOR_NAMESPACE} namespace if not exists"
	${KUBECTL} get namespace ${OPERATOR_NAMESPACE} || ${KUBECTL} create namespace ${OPERATOR_NAMESPACE}
	echo "  [OK] namespace prepared."
	echo "[STEP 2 of 4] Configuring olm resources to ${REPO_ROOT}/_deploy_olm"
	mkdir -p ${REPO_ROOT}/_deploy_olm
	cp ${REPO_ROOT}/config/olm/catalog-source.yaml ${REPO_ROOT}/_deploy_olm/catalog-source.yaml
	cp ${REPO_ROOT}/config/olm/subscription.yaml ${REPO_ROOT}/_deploy_olm/subscription.yaml
	${YQ} eval -i ".spec.image=\"${CATALOG_IMG}\"" ${REPO_ROOT}/_deploy_olm/catalog-source.yaml
	${YQ} eval -i ".spec.channel=\"${CHANNELS}\"" ${REPO_ROOT}/_deploy_olm/subscription.yaml
	echo "  [OK] resource configured."
	echo "[STEP 3 of 4] Deploying operator resources"
	${KUBECTL} apply -f ${REPO_ROOT}/config/olm/operator-group.yaml
	${KUBECTL} apply -f ${REPO_ROOT}/_deploy_olm
	rm -rf ${REPO_ROOT}/_deploy_olm
	echo "  [OK] configured operator resources deployed."
	echo "  [WAIT] waiting for crd to be created ..."
	until ${KUBECTL} get crd spyreclusterpolicies.spyre.ibm.com 2>/dev/null; do
		sleep 2
	done
	echo "  [OK] spyreclusterpolicies.spyre.ibm.com custom resource created."
	echo "[STEP 4 of 4] Deploying ${CLUSTER_POLICY_FILE}"
	${KUBECTL} apply -f ${CLUSTER_POLICY_FULL_PATH}
	echo "  [OK] ${CLUSTER_POLICY_FILE} deployed."
	echo "Completed."
}

function uninstall_operator() {
	echo "[STEP 1 of 2] Deleting spyreclusterpolicy"
	${KUBECTL} delete spyreclusterpolicy --all || true
	echo "  [WAIT] waiting for SpyreClusterPolicy to be deleted..."
	until ! ${KUBECTL} get spyreclusterpolicy spyreclusterpolicy 2>/dev/null; do
		sleep 2
	done
	echo " [OK] SpyreClusterPolicy deleted."
	echo "[STEP 2 of 2] Deleting operator resources"
	${KUBECTL} delete -f ${REPO_ROOT}/config/olm
	${KUBECTL} delete csv -l operators.coreos.com/spyre-operator.spyre-operator -A || true
	${KUBECTL} delete csv -l olm.copiedFrom=spyre-operator -A || true
	${KUBECTL} delete crd spyreclusterpolicies.spyre.ibm.com spyrenodestates.spyre.ibm.com
	echo "  [OK] operator, csv, and crd deleted."
	echo "Completed."
}

readonly COMMAND=${1}
readonly OPERATOR_NAMESPACE=${2}

case "$COMMAND" in
install)
	if [[ $# -lt 4 ]]; then
		echo "[ERROR] 'install' requires [OPERATOR_NAMESPACE], [CATALOG_IMG] and [CHANNELS]."
		usage
	fi

	CATALOG_IMG=${3}
	CHANNELS=${4}

	echo "Installing..."
	echo "Architecture: $ARCH"
	echo "Namespace: $OPERATOR_NAMESPACE"
	echo "Catalog Image: $CATALOG_IMG"
	echo "Channel: $CHANNELS"
	echo "----------------------------------"
	install_operator
	;;

uninstall)
	if [[ $# -lt 2 ]]; then
		echo "[ERROR] 'uninstall' requires [OPERATOR_NAMESPACE]."
		usage
	fi

	echo "Uninstalling..."
	echo "Namespace: $OPERATOR_NAMESPACE"
	echo "----------------------------------"
	uninstall_operator
	;;

*)
	echo "[ERROR] Unknown command '$COMMAND'"
	usage
	;;
esac
