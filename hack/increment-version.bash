#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

set -e

readonly INPUT_VERSION_FILE="VERSION"
readonly OUTPUT_VERSION_FILE="VERSION"
MAJOR=false
MINOR=false
PATCH=false
RC=false
RC_NUMBER=0

function usage() {
	cat <<EOF
    increment-version.bash

        Increments the semantic version number if a major, minor, or patch if they are supplied or increments the rc number if
        requested

    ARGUMENTS:
        -M | --major                                Increment the major version number.
        -m | --minor                                Increment the minor version number.
        -p | --patch                                Increment the patch version number.
        -r | --rc <last rc number, default is 0>    Increment the rc candidate number.
        -h | --help                                 Print this message
EOF
	exit 2
}

function increment_rc() {
	((++RC_NUMBER))
	echo "${VA[0]}.${VA[1]}.${VA[2]}-rc.${RC_NUMBER}" >${OUTPUT_VERSION_FILE}
}
function increment_version() {
	# Increment version numbers as requested.
	if [[ ${MAJOR} == "true" ]]; then
		((++VA[0]))
		VA[1]=0
		VA[2]=0
	fi

	if [[ ${MINOR} == "true" ]]; then
		((++VA[1]))
		VA[2]=0
	fi

	if [[ ${PATCH} == "true" ]]; then
		((++VA[2]))
	fi

	echo "${VA[0]}.${VA[1]}.${VA[2]}" >${OUTPUT_VERSION_FILE}
}

while [ "$1" != "" ]; do
	case $1 in
	-M | --major)
		MAJOR=true
		shift
		;;
	-m | --minor)
		MINOR=true
		shift
		;;
	-p | --patch)
		PATCH=true
		shift
		;;
	-r | --rc)
		RC=true
		shift
		RC_NUMBER=${1:-0}
		shift
		;;
	-h | --help)
		usage
		;;
	*)
		echo "Unknown command line argument: '${1}'"
		usage
		;;
	esac
done

VERSION=$(cat ${INPUT_VERSION_FILE})
echo "Current version: ${VERSION}"
# Build array from version string, by first splitting the string along . and -rc if present
declare -a VA
VA=($(echo $VERSION | sed -r 's/(\.)|(-rc.)/ /g'))

if [[ ${#VA[@]} -ne 3 && ${#VA[@]} -ne 4 ]]; then
	echo "Invalid number of elements in version. Expecting either 3 or 4"
	usage
	exit 1
fi
if [[ ${RC} == "true" ]]; then
	increment_rc
else
	increment_version
fi
VERSION=$(cat ${OUTPUT_VERSION_FILE})
echo "New version: ${VERSION}"
