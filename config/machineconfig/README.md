# Machine Configuration Files

This directory contains machine configuration files used to configure OpenShift nodes for Spyre workloads.

## Directory Structure

- `amd64/` - Machine configs for x86_64 architecture
  - `butane/` - Butane source files (human-readable format)
  - YAML machine config files

## Machine Config Files

- `05-aiu-kernel-commandline.yaml` - Kernel parameters and VFIO-PCI driver settings
  - Source: `butane/05-aiu-kernel-commandline.bu`
  - Enables Intel and AMD IOMMU with passthrough mode
  - Loads vfio-pci and vfio_iommu_type1 kernel modules
  - Sets up VFIO device IDs for Spyre cards (1014:06a7, 1014:06a8)
  - Configures CRI-O memory limits
  - Sets up udev rules for VFIO devices

- `08-pciacs-v1.yaml` - PCIe Access Control Services (ACS) configuration
  - Source: `butane/08-pciacs-v1.bu`
  - Contains Perl script to disable PCIe ACS SrcValid for Spyre card branches
  - Runs as systemd service after kubelet starts
  - Required for proper PCIe device isolation and passthrough

- `09-vfstart.yaml` - SR-IOV Virtual Function creation
  - Source: `butane/09-vfstart.bu`
  - Applies to nodes with `vf` role label
  - Creates 2 virtual functions per Spyre physical function
  - Runs as systemd service to enable SR-IOV at boot
  - Sets permissions on /dev/vfio/* devices

- `mcp-vf.yaml` - Machine Config Pool for VF-enabled nodes
  - Defines a custom machine config pool for nodes that need SR-IOV VFs
  - Nodes must be labeled with `node-role.kubernetes.io/vf=""`

- `50-spyre-device-plugin-selinux-minimal.yaml` - SELinux configuration for the Spyre device plugin
