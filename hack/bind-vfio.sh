#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -e

PCI_DEVICES=$(lspci -n -d 1014:06a7 | cut -d " " -f1 | paste -sd "," -)

echo "$PCI_DEVICES"
IFS=',' read -ra DEVICES <<<"$PCI_DEVICES"

for VFIODEVICE in "${DEVICES[@]}"; do
	cd /sys/bus/pci/devices/"${VFIODEVICE}" || continue

	if [ ! -f "driver/unbind" ]; then
		echo "File driver/unbind not found for ${VFIODEVICE}"
		exit 1
	fi

	if ! echo -n "vfio-pci" >driver_override; then
		echo "Could not write vfio-pci to driver_override"
		exit 1
	fi

	if ! [ -f driver/unbind ] && echo -n "$VFIODEVICE" >driver/unbind; then
		echo "Could not write the VFIODEVICE: ${VFIODEVICE} to driver/unbind"
		exit 1
	fi

done
