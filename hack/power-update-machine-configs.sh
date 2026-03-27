#!/usr/bin/env bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+
# Check all args are provided
if [ $# -ne 3 ]; then
	echo "Usage: $0 <butane binary> <repo root> <bind-vfio file>"
	exit 1
fi

BUTANE=$1
MACHINE_CONFIG_PATH="${2}/config/machineconfig"
BIND_VFIO_FILE="${3}"
PLATFORM=$(uname)
echo "PLATFORM=$PLATFORM"

echo "${BIND_VFIO_FILE}"

echo "creating 99-worker-vfiopci.machineconfig.yaml from 99-worker-vfiopci.machineconfig.bu"
"$BUTANE" "${MACHINE_CONFIG_PATH}/ppc64le/butane/99-worker-vfiopci.machineconfig.bu" -o "${MACHINE_CONFIG_PATH}/ppc64le/99-worker-vfiopci.machineconfig.yaml"

echo "Patching 99-vhostuser-bind.yaml"

# Detect base64 and sed options for cross-platform compatibility
if [[ $PLATFORM == "Linux" ]]; then
	BASE64_NOWRAP="base64 -w 0"
elif [[ $PLATFORM == "Darwin" ]]; then
	BASE64_NOWRAP="base64 -i"
else
	echo "No suitable platform found."
	exit 1
fi

VFIO_SH_B64=$($BASE64_NOWRAP "${BIND_VFIO_FILE}")

# Escape & and \ in the base64 string for sed replacement
VFIO_SH_B64_ESCAPED=$(printf '%s' "$VFIO_SH_B64" | sed 's/[&\\/]/\\&/g')

# Detect platform for sed -i compatibility
if [[ $PLATFORM == "Linux" ]]; then
	SED_INPLACE="sed -i"
elif [[ $PLATFORM == "Darwin" ]]; then
	SED_INPLACE="sed -i ''"
else
	echo "No suitable platform found."
	exit 1
fi

set -x
# Patch the vhostuser machine config
$SED_INPLACE "s|source: .*|source: data:text/plain;charset=utf-8;base64,${VFIO_SH_B64_ESCAPED}|" "${MACHINE_CONFIG_PATH}/ppc64le/99-vhostuser-bind.yaml"
