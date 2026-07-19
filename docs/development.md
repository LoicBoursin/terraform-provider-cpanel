# Development

## Requirements

Use the Go and Terraform versions listed in
[`compatibility.md`](compatibility.md). The validation scripts resolve their
tools from `PATH`.

The CI and release configuration pins:

- actionlint `1.7.12`;
- golangci-lint `2.12.2`;
- govulncheck `1.6.0`;
- GoReleaser `2.17.0`;
- Syft `1.42.3`.

The repository uses the golangci-lint version 2 configuration schema.

## Local validation

Run the complete repository validation with:

```shell
make verify
```

This verifies Go modules, runs race-enabled tests, vet, lint, vulnerability
scanning, workflow and shell validation, documentation generation, generated
file checks, and GoReleaser configuration validation.

Generate Registry documentation separately with:

```shell
make generate-documentation
```

Build release artifacts without publishing them with:

```shell
make release-snapshot
```

Syft must be available on `PATH` for release snapshots.

## cPanel acceptance environment

Acceptance tests create, update, import, and delete real cPanel objects. Use a
dedicated test account.

Credentials are loaded from the file named by `CPANEL_ENV_FILE`. When the
variable is unset, the default path is:

```text
~/.config/terraform-provider-cpanel/acceptance.env
```

The file format matches
[`.env.acceptance.example`](../.env.acceptance.example):

```text
CPANEL_HOST=https://cpanel.example.com:2083
CPANEL_USERNAME=account
CPANEL_API_TOKEN=token
CPANEL_API_TOKEN_NAME=terraform-provider-acceptance
CPANEL_EXPECTED_TEST_HOST=https://cpanel.example.com:2083
CPANEL_EXPECTED_TEST_USERNAME=account
CPANEL_EXPECTED_VERSION='134.0 (build 45)'
CPANEL_ACCEPT_DESTRUCTIVE=1
CPANEL_TEST_SSL_KEY_ID=
```

Set the file permissions to `0600`. Never commit populated credentials.
`CPANEL_TEST_SSL_KEY_ID` is optional and identifies an existing RSA key used
only through its public metadata by CSR tests.

The destructive scripts refuse to run unless `CPANEL_ACCEPT_DESTRUCTIVE=1`
and the configured host and username exactly match
`CPANEL_EXPECTED_TEST_HOST` and `CPANEL_EXPECTED_TEST_USERNAME`.
They also require the exact `CPANEL_EXPECTED_VERSION` value returned by the
cPanel StatsBar API, so a server upgrade cannot silently run a destructive
suite against an unvalidated release.
`CPANEL_API_TOKEN_NAME` identifies the provider authentication token so
imported API token resources cannot revoke it.

Acceptance cleanup also requires the known clean values of singleton account
settings. They are loaded from `CPANEL_BASELINE_FILE`, or from
`terraform-provider-cpanel-baseline.env` next to the default credentials file.
The format is documented in
[`.env.acceptance-baseline.example`](../.env.acceptance-baseline.example).
Keep this file at `0600` and populate it from the dedicated account's clean
configuration, not from values discovered during a test run.
The complete suite requires `CPANEL_ALLOW_GPG_KEYPAIR_DELETE=1` and an explicit
zero secret-key baseline because its GPG resource tests clean up generated key
pairs.

Run non-destructive account and capability checks with:

```shell
make smoke-test
```

Run the destructive acceptance suite with:

```shell
make test-acceptance
```

Run cleanup and require an empty test-artifact inventory with:

```shell
make clean-acceptance
```

The acceptance entry point restores the persisted singleton baseline before
the suite. An `EXIT` finalizer restores it again, removes only artifacts using
the provider's reserved test naming conventions, and requires the final
test-artifact inventory to be empty.

Tests preserve unrelated account objects. Cleanup refuses ambiguous or unsafe
deletions and reports them for manual review.

Run the acceptance suite once with each supported Terraform minor version on
`PATH` before a release.
