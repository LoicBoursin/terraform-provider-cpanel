---
page_title: "Compatibility - terraform-provider-cpanel"
description: |-
  Supported Terraform, Go, and cPanel versions for terraform-provider-cpanel.
---

# Compatibility

## Supported versions

| Component | Supported or tested versions |
| --- | --- |
| Terraform CLI | `1.14.x` and `1.15.x` |
| Terraform provider protocol | `6.0` |
| Go development toolchain | `1.26.x` |
| cPanel & WHM | `134.x` |
| Tested o2switch environment | cPanel `134.0` build `45` |

The provider supports the two most recent stable Terraform minor releases at
the time of a provider release. Older Terraform versions may continue to work,
but they are not part of the tested compatibility matrix.

Development, documentation generation, and releases use the current stable Go
release. Patch updates are adopted when they contain relevant fixes.

## cPanel requirements

The provider connects to the cPanel account API over HTTPS, normally on port
`2083`, and authenticates with an account username and API token. Plain HTTP
endpoints are rejected.

Availability of individual resources depends on the corresponding cPanel
feature being enabled for the account. The complete list of implemented
resources and data sources is available in the
[README](../README.md#supported-resources) and generated Registry
documentation.

The provider targets account-level cPanel APIs. WHM server-administration
operations are outside its scope.

## Tested environment

On July 20, 2026, the complete acceptance suite passed on the o2switch test
account running cPanel `134.0` build `45` with Terraform `1.14.9` and `1.15.8`.

The test matrix covered:

- provider authentication and capability checks;
- resource creation, reading, updates, imports, drift detection, remote
  deletion, recreation, and destruction;
- read-only account inventory data sources;
- restoration of account-level settings changed during tests;
- cleanup of all test-managed cPanel artifacts.

Each Terraform version completed with the persisted account baselines restored
and an empty test-artifact inventory.

## Compatibility policy

Resource schemas use stable cPanel account identities and fail when required
remote state is missing, malformed, contradictory, or incomplete. Mutations
are verified through follow-up reads whenever the cPanel API permits it.

Credentials, passwords, private keys, and write-only values are treated as
sensitive. Terraform configuration and state files must still be protected by
the user.

The provider serializes cPanel requests internally. Standard `terraform plan`
and `terraform apply` commands are supported without a provider-specific
`-parallelism=1` requirement.
