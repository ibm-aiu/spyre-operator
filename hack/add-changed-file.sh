#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

FILE=$1
if [ -f "${FILE}" ] && ! git diff --name-only | grep -q "${FILE}"; then
	echo "skip hook. ${FILE} is not changed."
	exit 0
fi
git add ${FILE}
