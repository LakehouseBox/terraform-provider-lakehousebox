---
page_title: "lakehousebox_sql_post_prefix Resource"
description: |-
  A URL prefix the catalog's scheduled SQL may POST to.
---

# lakehousebox_sql_post_prefix

Scheduled SQL sends a request body (`http_post`) only to URLs under a prefix the catalog declared.

```terraform
resource "lakehousebox_sql_post_prefix" "segura" {
  catalog    = "embalses"
  url_prefix = "https://saihweb.chsegura.es/apps/iVisor/"
}
```

## Arguments

- `catalog`, `url_prefix` (String, Required): `https://…/`, ending in `/`.

## Import

```shell
tofu import lakehousebox_sql_post_prefix.api 'embalses|https://example.org/api/'
```
