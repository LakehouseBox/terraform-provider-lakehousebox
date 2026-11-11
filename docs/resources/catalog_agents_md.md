---
page_title: "lakehousebox_catalog_agents_md Resource"
description: |-
  The catalog's AGENTS.md.
---

# lakehousebox_catalog_agents_md

What the catalog's tables mean, how they join and how fresh they are, for the agents that read it (64 KB at most).

```terraform
resource "lakehousebox_catalog_agents_md" "embalses" {
  catalog = "embalses"
  text    = file("docs/AGENTS.md")
}
```

## Import

```shell
tofu import lakehousebox_catalog_agents_md.md embalses
```
