terraform {
  required_providers {
    lakehousebox = { source = "lakehousebox/lakehousebox" }
  }
}
provider "lakehousebox" {}

variable "every" { default = "1h" }
variable "protect" { default = true }
variable "org" { default = "acme" }

resource "lakehousebox_catalog" "lab" {
  name                   = "tofu_qa"
  default_format_version = 2
  deletion_protection    = var.protect
}

resource "lakehousebox_sql_post_prefix" "api" {
  catalog    = lakehousebox_catalog.lab.name
  url_prefix = "https://example.org/api/"
}

resource "lakehousebox_catalog_agents_md" "md" {
  catalog = lakehousebox_catalog.lab.name
  text    = "# tofu_qa\n\nA test catalog.\n"
}

resource "lakehousebox_sql_sink" "const" {
  catalog        = lakehousebox_catalog.lab.name
  name           = "const_rows"
  table          = "raw.const"
  sql            = "SELECT 1::BIGINT AS n, 'a' AS label"
  every          = var.every
  offset         = "5m"
  create_table   = true
  format_version = 2
  paused         = true
}

resource "lakehousebox_table_description" "const" {
  catalog     = lakehousebox_catalog.lab.name
  table       = lakehousebox_sql_sink.const.table
  description = "Constant rows (provider test)."
  columns     = { n = "A number.", label = "A label." }
}

# what every hosted agent and analyst of the organisation may use and is told (a key with the admin role)
resource "lakehousebox_org_agent_settings" "agents" {
  org           = var.org
  models        = ["deepseek-v4-flash", "qwen3.6-35b"]
  default_model = "deepseek-v4-flash"
  instructions  = "Amounts are in euros, without VAT."
}
