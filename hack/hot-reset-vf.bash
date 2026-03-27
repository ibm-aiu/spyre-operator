#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

readonly OC=$(command -v oc)

function usage() {
	echo "Usage:   $0 <worker node name>"
	exit 2
}

function hot_reset_vf() {
	local worker=$1

	READY=$(${OC} get node "$worker" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
	if [ "$READY" != "True" ]; then
		echo "Node $worker is not Ready, SKIP"
		exit 0
	fi

	echo "disable vf"
	${OC} debug --quiet nodes/"${worker}" -- sh -c 'lspci -D | grep -E "IBM|Spyre" | grep 06a7 | awk "{print \$1}" | xargs -I{} sh -c "echo 0 > /sys/bus/pci/devices/{}/sriov_numvfs"'

	echo "set 2 vf for each pf"
	${OC} debug --quiet nodes/"${worker}" -- sh -c 'lspci -D | grep -E "IBM|Spyre" | grep 06a7 | awk "{print \$1}" | xargs -I{} sh -c "echo 2 > /sys/bus/pci/devices/{}/sriov_numvfs"'

	echo "list devices"
	${OC} debug --quiet nodes/"${worker}" -- sh -c 'lspci -D | grep -E "IBM|Spyre"'
}

if [[ "x" == "x${OC}" ]]; then
	echo "oc must be available on the path"
	exit 1
fi

if [[ $1 == "" ]]; then
	usage
fi

# main
hot_reset_vf "$1"
