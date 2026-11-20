# A connected catalog: Overture Maps' public release, read where it lives (the organisation's `sources` switch on).
# Applied against production in a QA account on 2026-11-19 (2 created, 15 tables, a second plan with no changes).
terraform {
  required_providers {
    lakehousebox = { source = "lakehousebox/lakehousebox" }
  }
}

variable "org" {}

resource "lakehousebox_connection" "overture" {
  org       = var.org
  name      = "overture"
  anonymous = true
  endpoint  = "s3.us-west-2.amazonaws.com"
  region    = "us-west-2"
  bucket    = "overturemaps-us-west-2"
  prefix    = "release/2026-11-18.0"
}

resource "lakehousebox_source" "overture" {
  org             = lakehousebox_connection.overture.org
  name            = "overture"
  connection_name = lakehousebox_connection.overture.name
}

output "tables" { value = lakehousebox_source.overture.tables }
