#!/bin/bash

# +-------------------------------------------------------------------+
# | Copyright (c) 2025, 2026 IBM Corp.                                |
# | SPDX-License-Identifier: Apache-2.0                               |
# +-------------------------------------------------------------------+

ssh_priv_key_file=$1
base64key=$(cat ${ssh_priv_key_file} | base64 -w 0)
cat <<EOF
kind: Secret
apiVersion: v1
metadata:
  name: github-ssh-credentials
data:
  id_rsa: $base64key
EOF
