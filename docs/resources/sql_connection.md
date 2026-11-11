---
page_title: "lakehousebox_sql_connection Resource"
description: |-
  A credential for the catalog's scheduled SQL, bound to one URL prefix.
---

# lakehousebox_sql_connection

A write-only credential the catalog's scheduled SQL sends to URLs under one prefix: an organisation web key
(`integration`, the key never copied) or a value. A new `value` is rotated in place; any other change makes a new
connection (sinks that use a deleted connection pause).

```terraform
resource "lakehousebox_sql_connection" "aemet" {
  catalog     = "embalses"
  name        = "aemet"
  integration = "aemet"
}
```

## Arguments

- `catalog`, `name` (String, Required).
- `integration` (String) An organisation web key's name. Or:
- `url_prefix` (String) `https://…/`; `kind` (String) `bearer`, `header` or `basic`; `header_name` (String, with
  `header`); `value` (String, Sensitive) never readable back (kept in state as sensitive values are: prefer
  `integration`).

## Import

```shell
tofu import lakehousebox_sql_connection.aemet 'embalses|aemet'
```

The value is never readable: the first apply after an import sends the configured `value` in place (a web-key
connection reads back its key's name). Nothing is deleted, so no sink pauses.
