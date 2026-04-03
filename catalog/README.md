
# Create File Based Catalog (FBC) image

Before running these make target, verify the bundles are specified in [artifact config](../release-artifacts.yaml).

For currently released bundled images:

```bash
make fbc-gen-base-template && make fbc-build
```

If working on a new release and need to add a new bundle to the catalog:

```bash
# Assuming bundle image exists in a registry or the fbc-build may fail to extract bundle metadata.
# make bundle && make bundle-build && make bundle-push
make fbc-bundle-add && make fbc-build

# If the release is also a candidate release
CANDIDATE=true make fbc-bundle-add && make fbc-build

# If the release is also a stable release
STABLE=true make fbc-bundle-add && make fbc-build
```

Push catalog image:

```bash
make fbc-push
```

Create the catalog in the cluster:

```bash
oc apply -f catalog/catalog-source.yaml
```

Reference:

These files are generate during the process:

1. A sample CatalogSource CR `catalog/catalog-source.yaml` used for creating and testing the catalog.

1. A [semver catalog template](https://olm.operatorframework.io/docs/reference/catalog-templates/#semver-template) file `catalog/base_template.yaml`
   is used by opm cli to generate the FBC yaml.
1. FBC yaml `catalog/spyre-operator/spyre-operator.yaml` is an OLM [file based catalog](https://olm.operatorframework.io/docs/reference/file-based-catalogs/).
   It is the recommended way to build a catalog image. It contains three schemas: olm.package, olm.channel, and olm.bundle.
    Each operator package in a file based catalog requires exactly:
    * One olm.package blob: Defines package-level metadata for an operator. For example:

        ```yaml
        defaultChannel: stable
        name: spyre-operator
        schema: olm.package
        ```

    * At least one olm.channel blob: Defines a channel within a package. Use the following blob as an example to add a channel into the catalog:

        ```yaml
        entries:
        - name: spyre-operator.v2.1.0-rc.2
        name: release-v2.1
        package: spyre-operator
        schema: olm.channel
        ```

    * At least one olm.bundle blobs: Defines an individually installable version of an operator within a package. For example:

        ```yaml
        schema: olm.bundle
        image: docker.io/spyre-operator/spyre-operator-bundle:2.3.0-dev
        name: spyre-operator.v2.3.0-dev
        package: spyre-operator
        ...
        ```
