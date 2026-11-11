---
page_title: "lakehousebox_table_description Resource"
description: |-
  A table's description and its columns'.
---

# lakehousebox_table_description

The description of a table and of its columns. The table comes from migrations; this resource sets only what it
declares and, destroyed, removes only that.

```terraform
resource "lakehousebox_table_description" "readings" {
  catalog     = "embalses"
  table       = "obs.reservoir_readings"
  description = "Live reservoir readings, one row per station and reading."
  columns     = { stored_volume_hm3 = "Water stored, hm³." }
}
```

## Import

```shell
tofu import lakehousebox_table_description.t 'embalses|raw.readings'
```

An import reads the table's description and every column description, so a configuration that declares them plans
no change; one it does not name shows in the plan as a removal.
