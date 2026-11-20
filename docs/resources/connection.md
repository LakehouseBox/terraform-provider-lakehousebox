---
page_title: "lakehousebox_connection Resource"
description: |-
  A connection to another system that connected catalogs (sources) read with: an S3-compatible bucket and folder.
---

# lakehousebox_connection

A stored login to an S3-compatible bucket and folder, with a key or none (a public bucket). Its maker owns it and it is
private to them until shared (`use_access`). The key is write-only: never read back, and a new key pair is replaced in
place, so the sources that read with it never stop. Where it points (endpoint, bucket, folder, region, URL style,
anonymous) never changes in place: another bucket or folder is another connection.

```terraform
resource "lakehousebox_connection" "overture" {
  org       = "acme"
  name      = "overture"
  anonymous = true
  endpoint  = "s3.us-west-2.amazonaws.com"
  region    = "us-west-2"
  bucket    = "overturemaps-us-west-2"
  prefix    = "release/2026-11-18.0"
}
```

A Google Cloud Storage bucket is read the same way, with an HMAC key of a service account that can only read:

```terraform
resource "lakehousebox_connection" "exports" {
  org               = "acme"
  name              = "exports"
  endpoint          = "storage.googleapis.com"
  region            = "auto"
  bucket            = "acme-exports"
  prefix            = "daily"
  access_key_id     = var.gcs_hmac_access_id
  secret_access_key = var.gcs_hmac_secret
}
```

The guide for each kind of store: <https://lakehousebox.com/docs/connected-catalogs/>.

## Arguments

- `org` (String, Required) The organisation's handle or id. `name` (String, Required) Lower case letters, digits and `_`.
- `endpoint` (String, Required) The store's host name (https). `bucket` (String, Required).
- `prefix` (String) A folder inside the bucket: only keys under it are read. `region` (String, default `us-east-1`).
  `url_style` (String) `path` (default) or `vhost`. `type` (String, default `s3`).
- `anonymous` (Bool) A public bucket, read with no key. Or `access_key_id` and `secret_access_key` (String,
  Sensitive): never readable back, kept in state as sensitive values are.
- `use_access` (String) Who besides its owner and the admins may attach sources from it: `owner` (default) or
  `everyone`.

## Attributes

- `owner_id` (String) Its owner. `state` (String) `active`, or `stopped` when its owner left the organisation (an
  admin takes it over).

## Import

```shell
tofu import lakehousebox_connection.overture 'acme|overture'
```

The key is never readable: the first apply after an import sends the configured key pair in place.
