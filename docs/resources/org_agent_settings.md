---
page_title: "lakehousebox_org_agent_settings Resource"
description: |-
  The models an organisation's hosted agents may use, the default model, and the instructions every agent is given.
---

# lakehousebox_org_agent_settings

What an organisation's admins decide about its hosted agents and analysts. Use a key with the admin role.

- `models`: the models agents may use (`lhbox agent models` lists them). The others are switched off. An agent already
  on a model that is switched off keeps it, and its runs use the default until the model is switched on again.
- `default_model`: the model a new agent gets and every person's analyst runs on. It must be one of `models`.
- `instructions`: given to every agent and analyst of the organisation after LakehouseBox's own rules and before the
  agent's task, 4,000 characters at most. They cannot widen what an agent may read or do.

`models` and `default_model` left out keep what is set now. `instructions` left out means none. Destroying the resource
puts the deployment's settings back: every model on, its default, no instructions.

```terraform
resource "lakehousebox_org_agent_settings" "acme" {
  org           = "acme"
  models        = ["deepseek-v4-flash", "qwen3.6-35b"]
  default_model = "deepseek-v4-flash"
  instructions  = file("agents/instructions.md")
}
```

## Import

```shell
tofu import lakehousebox_org_agent_settings.acme acme
```
