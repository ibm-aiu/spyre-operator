#!/bin/bash
HEADER_FILE=${1}
TARGET_FILE=${2}
TEMPFILE=$(mktemp)

if [ ! -f ${HEADER_FILE} ]; then
	echo "Error: header file must exist"
	exit 1
fi
if [ ! -f ${TARGET_FILE} ]; then
	echo "Error: target file must exist"
	exit 1
fi
COPYRIGHTLEN=$(cat ${HEADER_FILE} | wc -l)
head -n ${COPYRIGHTLEN} ${TARGET_FILE} | diff -q ${HEADER_FILE} -
HAS_DIFF=${?}
if [ ${HAS_DIFF} -gt 0 ]; then
	(
		cat ${HEADER_FILE}
		echo
		cat ${TARGET_FILE}
	) >${TEMPFILE}
	mv ${TEMPFILE} ${TARGET_FILE}
fi
