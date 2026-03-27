#!/bin/bash
# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

FILENAME=$1
TITLE=$2
OUTPUT=./docs/${FILENAME}.md

# write header
echo "# ${TITLE}" >${OUTPUT}
echo "" >>${OUTPUT}
echo "Test item | Case description | File location" >>${OUTPUT}
echo "---|---|---" >>${OUTPUT}

# extract test specs
CSV=$(mktemp)
jq -r '.[]|select(.SpecReports!=null)|.SpecReports[]|select(.ContainerHierarchyTexts!=null) | [(.ContainerHierarchyTexts | join("/")), .LeafNodeText, .LeafNodeLocation.FileName] | @csv ' <${FILENAME}.json | sort -f >${CSV}

# filter specs for e2e/integration test
ALL_ROWS=$(mktemp)
sed 's:'$(pwd)/'::g' ${CSV} | sed 's/","/|/g; s/"//g' >${ALL_ROWS}
if [ "${TITLE}" == "E2E Tests" ]; then
	cat ${ALL_ROWS} | fgrep 'e2e_test.go' >>${OUTPUT}
elif [ "${TITLE}" == "Integration Tests" ]; then
	cat ${ALL_ROWS} | fgrep 'integration_test.go' >>${OUTPUT}
else
	cat ${ALL_ROWS} >>${OUTPUT}
fi
