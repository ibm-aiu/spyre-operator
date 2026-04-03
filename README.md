# IBM Spyre Operator

IBM Spyre Operator deploys and manages Spyre components on Red Hat OpenShift.

## Overview

Use this operator to install and manage the Spyre software stack on OpenShift clusters. The operator reconciles a `SpyreClusterPolicy` and rolls out components in three ordered phases:

- `state-init`: common prerequisites before core component
- `state-core-components`: core component to manage Spyre cards
- `state-plugin-components`: optional plugin services

This phased rollout helps ensure prerequisites are ready before core and optional components are deployed.

## Documentation

- IBM Z and LinuxONE user guide: https://www.ibm.com/docs/en/rhocp-ibm-z?topic=spyre-operator-z-linuxone-users-guide
- Red Hat Ecosystem Catalog entry: https://catalog.redhat.com/en/software/container-stacks/detail/683da25176619df6976559d0

## Repository links

- [Contributing](CONTRIBUTING.md)
- [Maintainers](MAINTAINERS.md)
- [License](LICENSE)
