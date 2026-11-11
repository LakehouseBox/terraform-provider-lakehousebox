# terraform-provider-lakehousebox

The [LakehouseBox](https://lakehousebox.com) provider for OpenTofu and Terraform: managed Apache Iceberg catalogs,
operated in Europe, and what keeps them filled.

```hcl
terraform {
  required_providers {
    lakehousebox = { source = "lakehousebox/lakehousebox" }
  }
}

provider "lakehousebox" {}   # LHBOX_API_KEY in the environment (`lhbox login` makes a key)

resource "lakehousebox_catalog" "embalses" {
  name                   = "embalses"
  default_format_version = 3
}

resource "lakehousebox_sql_sink" "rain_grid" {
  catalog = lakehousebox_catalog.embalses.name
  name    = "now_rain_grid"
  table   = "now.rain_grid"
  sql     = file("sql/now/rain_grid.sql")
  every   = "1h"
  offset  = "28m"
  mode    = "replace"
}
```

| resource | what |
|---|---|
| `lakehousebox_catalog` | a catalog: `default_format_version`, `public`; `deletion_protection` (default true) |
| `lakehousebox_sql_sink` | a scheduled SQL sink: one SELECT on a schedule into one table (its query previewed first) |
| `lakehousebox_sql_connection` | a credential for scheduled SQL, bound to a URL prefix: an organisation web key or a value |
| `lakehousebox_sql_post_prefix` | a URL prefix the catalog's scheduled SQL may POST to |
| `lakehousebox_catalog_agents_md` | the catalog's AGENTS.md |
| `lakehousebox_table_description` | a table's description and its columns' |

Tables are not resources: they come from migrations (`lhbox migrate`, see the
[docs](https://lakehousebox.com/docs/migrations/)), so no apply ever drops a table. Destroying a sink deletes the sink,
never its table or rows; destroying a catalog needs `deletion_protection = false` and starts a 7-day grace period.

Import: `tofu import lakehousebox_catalog.x <catalog id or name>`, `tofu import lakehousebox_sql_sink.x <sink id>`.
