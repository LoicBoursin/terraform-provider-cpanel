---
page_title: "Compatibility - terraform-provider-cpanel"
description: |-
  Supported Terraform, Go, and cPanel versions for terraform-provider-cpanel.
---

# Compatibility

## Version policy

The provider supports the two most recent stable Terraform minor releases at
the time of a provider release. Older Terraform releases may continue to work,
but they are not part of the tested compatibility contract.

Development, documentation generation, and releases use the current stable Go
release. Patch updates are adopted promptly, especially when they contain
security fixes.

## v1.0 target matrix

| Component | Supported or certified versions |
| --- | --- |
| Terraform CLI | 1.14.x and 1.15.x |
| Terraform provider protocol | 6.0 |
| Go development toolchain | 1.26.x |
| cPanel & WHM | 134.x |
| Certified o2switch environment | 134.0 build 44 |

The o2switch certification environment uses the cPanel account API over HTTPS
on port 2083. Certification covers authentication, cron jobs, PostgreSQL
databases, PostgreSQL users, imports, drift detection, and cleanup.

## API policy

PostgreSQL operations use cPanel UAPI.

Cron operations currently use cPanel API 2 because cPanel does not provide UAPI
equivalents for the required cron functions. API 2 is deprecated, so each
provider release must run the cron acceptance suite against the certified
cPanel environment.

## Concurrency

The provider serializes all cPanel requests through its shared client. Users do
not need to disable Terraform parallelism.

Each HTTP request has a 30-second timeout and honors Terraform context
cancellation. The provider does not automatically retry API calls. cPanel
mutations are not reliably idempotent, so retrying after an interrupted
response could apply an operation twice. Terraform instead refreshes the
remote state before deciding what must be retried.

## Compatibility evidence

A release is compatible only when:

- unit tests pass without a cPanel account;
- acceptance tests pass on every supported Terraform minor release;
- the complete acceptance suite passes on the certified cPanel environment;
- a second refresh after the test run reports no unexpected state changes;
- the test account contains no residual resources after cleanup.

The release workflow repeats the complete acceptance suite on Terraform
`1.14.9` and `1.15.8` before it can sign and publish artifacts.
