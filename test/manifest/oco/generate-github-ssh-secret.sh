#!/bin/bash

# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
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
