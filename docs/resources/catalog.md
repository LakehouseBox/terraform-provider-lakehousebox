---
page_title: "lakehousebox_catalog Resource"
description: |-
  A catalog: an Iceberg REST catalog and its storage.
---

# lakehousebox_catalog

A catalog. Its name and project are fixed once it is made (a plan that changes them is refused). Destroying it needs
`deletion_protection = false` and starts a 7-day grace period (restorable), after which its tables and files are purged.

```terraform
resource "lakehousebox_catalog" "lab" {
  name                   = "lab"
  default_format_version = 3
}
```

## Arguments

- `name` (String, Required) `^[a-z][a-z0-9_]{0,62}$`.
- `project_id` (String) The project it is made in (default: the organisation's first).
- `default_format_version` (Number) 2 or 3: the Iceberg format version LakehouseBox's own create paths give a new table.
- `public` (Boolean) Readable by anyone: every data file and retained metadata. Admins only. Default false.
- `deletion_protection` (Boolean) Default true: destroy refuses while it is true.

## Attributes

- `id` The catalog id. `bucket` Its table bucket.

## Import

```shell
tofu import lakehousebox_catalog.lab lab
```
