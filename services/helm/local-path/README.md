# local-path

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.0.34](https://img.shields.io/badge/AppVersion-v0.0.34-informational?style=flat-square)

local-path provisioner StorageClass for k3s lab clusters

## Under the hood

This is a **custom chart** (no upstream dependencies) that declares a single
`StorageClass` backed by the `rancher/local-path-provisioner` controller.

k3s already deploys the controller in `kube-system`, so this chart only creates
the `StorageClass` resource and marks it as the cluster default.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| storageClass.name | string | `"local-path"` | StorageClass name |
| storageClass.isDefault | bool | `true` | Mark as default StorageClass |
| storageClass.provisioner | string | `"rancher.io/local-path"` | Provisioner name |
| storageClass.reclaimPolicy | string | `"Delete"` | Reclaim policy |
| storageClass.volumeBindingMode | string | `"WaitForFirstConsumer"` | Volume binding mode |
