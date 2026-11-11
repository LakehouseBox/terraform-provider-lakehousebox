---
page_title: "lakehousebox_sql_sink Resource"
description: |-
  A scheduled SQL sink: one DuckDB SELECT on a schedule, its rows committed to one table.
---

# lakehousebox_sql_sink

One DuckDB `SELECT` run by LakehouseBox on a schedule, its rows committed to one table: over public URLs (with
`http_get`, `http_post`, connections for keys), or over the catalog's own tables (a derived sink). Its query is
previewed before it is made or changed. Destroying it deletes the sink only: its table and rows stay.
[Docs](https://lakehousebox.com/docs/scheduled-sql/).

```terraform
resource "lakehousebox_sql_sink" "readings" {
  catalog = "embalses"
  name    = "saih_ebro"
  table   = "obs.reservoir_readings"
  sql     = file("sql/ingest/ebro.sql")
  every   = "15m"
  offset  = "4m"
}
```

## Arguments

- `catalog`, `name`, `table` (`namespace.table`) (String, Required; a change makes a new sink).
- `sql` (String, Required) The SELECT. `every` (String, Required) `15m`, `1h`, `1d`. `offset` (String) Where in the
  interval it runs.
- `mode` (String) `append` (default) or `replace` (each run replaces the table, format-version 2). A change makes a new sink.
- `min_rows` (Number) A replace run with fewer rows stores nothing (default 1).
- `key` (List of String) Rows sharing it replace each other.
- `engine` (String) `2.0` (default) or `1.5`. `paused` (Boolean).
- `create_table`, `format_version`, `replace_existing`: used when the sink is made (make its table from the query's
  columns; replace a table that holds rows at the first run).

## Attributes

- `id` The sink id. `reads`: `urls`, or `catalog` for a derived sink.

## Import

```shell
tofu import lakehousebox_sql_sink.readings <sink id>
```
