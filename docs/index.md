---
page_title: "LakehouseBox Provider"
description: |-
  Managed Apache Iceberg catalogs, operated in Europe, and what keeps them filled.
---

# LakehouseBox Provider

[LakehouseBox](https://lakehousebox.com) runs Apache Iceberg catalogs in Europe (Frankfurt, replicated to Stockholm).
This provider manages a catalog and what feeds it: scheduled SQL sinks, their connections and POST prefixes, the
catalog's AGENTS.md and table descriptions.

**Tables are not resources.** They come from migrations (`lhbox migrate`, numbered DuckDB DDL files applied once each:
[docs](https://lakehousebox.com/docs/migrations/)), so no apply can drop a table or its rows.

## Example

```terraform
provider "lakehousebox" {}   # LHBOX_API_KEY in the environment

resource "lakehousebox_catalog" "embalses" {
  name                   = "embalses"
  default_format_version = 3
}

resource "lakehousebox_sql_sink" "rain_grid" {
  catalog  = lakehousebox_catalog.embalses.name
  name     = "now_rain_grid"
  table    = "now.rain_grid"
  sql      = file("sql/now/rain_grid.sql")
  every    = "1h"
  offset   = "28m"
  mode     = "replace"
  min_rows = 1000
}
```

## Authentication

An API key or agent key in `LHBOX_API_KEY` (`lhbox login` makes one), or `api_key` in the provider block (keep it out
of files). `endpoint` (or `LHBOX_URL`) defaults to `https://api.lakehousebox.com`.

## Schema

- `api_key` (String, Sensitive) An API key or agent key; default `LHBOX_API_KEY`.
- `endpoint` (String) The API; default `LHBOX_URL`, else `https://api.lakehousebox.com`.
