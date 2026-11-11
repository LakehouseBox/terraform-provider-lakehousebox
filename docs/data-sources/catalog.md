---
page_title: "lakehousebox_catalog Data Source"
description: |-
  A catalog the key can read, by name.
---

# lakehousebox_catalog (Data Source)

```terraform
data "lakehousebox_catalog" "pubdata" { name = "embalses" }
```

Attributes: `id`, `project_id`, `default_format_version`, `public`, `bucket`.
