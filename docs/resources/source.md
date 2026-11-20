---
page_title: "lakehousebox_source Resource"
description: |-
  A connection attached under a name: a connected catalog SQL reads where its files live.
---

# lakehousebox_source

A connection attached under a name. SQL reads its tables where they live as `<name>.<schema>.<table>`. Created, it
looks at its files once and lists the tables it found; destroyed, it is detached (the data is never touched).

```terraform
resource "lakehousebox_source" "overture" {
  org             = lakehousebox_connection.overture.org
  name            = "overture"
  connection_name = lakehousebox_connection.overture.name
  org_access      = "read"
}
```

## Arguments

- `org` (String, Required) The organisation's handle or id. `name` (String, Required) How SQL names it; not a catalog's
  name. `connection_name` (String, Required) The connection's name.
- `org_access` (String) `none` (default, Restricted: admins and those granted it) or `read` (every member). Changes in
  place.

## Attributes

- `tables` (List of String) The tables found in its files at the last look, as SQL names them. A later look
  (`lhbox source refresh`) changes them without a plan.

## Import

```shell
tofu import lakehousebox_source.overture 'acme|overture'
```
