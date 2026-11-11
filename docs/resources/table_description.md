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
