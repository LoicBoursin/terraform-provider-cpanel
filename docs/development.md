# Development

## Requirements

Use the Go and Terraform versions listed in
[`compatibility.md`](compatibility.md).

The CI and release configuration pins:

- actionlint `1.7.12`;
- Gitleaks `8.30.1`;
- golangci-lint `2.12.2`;
- govulncheck `1.6.0`;
- GoReleaser `2.17.0`;
- ShellCheck `0.11.0`;
- Syft `1.48.0`.

The repository uses the golangci-lint version 2 configuration schema.
Install the exact validation tools into the Git-local, untracked
`.git/tools` directory with:

```shell
make tools
```

The Makefile invokes those pinned binaries directly and refuses complete
validation when they are missing.

Install either tested Terraform version into the Git-local, untracked
`.git/tools` directory with:

```shell
./scripts/install-terraform.sh 1.14.9
./scripts/install-terraform.sh 1.15.8
```

The installer accepts only the versions declared in
[`scripts/tool-versions.sh`](../scripts/tool-versions.sh), verifies the pinned
archive checksum for the current platform, and prints the directory to prepend
to `PATH`.

## Local validation

Run the complete repository validation with:

```shell
make verify
```

This verifies that Go module files are tidy and their downloads match recorded
checksums, then runs race-enabled tests, vet, lint, vulnerability scanning,
workflow and shell validation, documentation generation, generated file
checks, and GoReleaser configuration validation.

Generate Registry documentation separately with:

```shell
make generate-documentation
```

Build release artifacts without publishing them with:

```shell
make release-snapshot
```

The pinned Syft binary installed by `make tools` is used for release snapshots.
The target also materializes the standalone Terraform Registry manifest and
verifies all archive, SBOM, manifest, and checksum artifacts before returning.

Verify that two clean builds produce identical provider archives and Terraform
Registry manifests with:

```shell
make release-reproducibility
```

Syft SBOMs and GoReleaser metadata contain generation timestamps and are
validated for structure and checksums rather than byte-for-byte
reproducibility.

The release workflow also downloads every published asset, verifies the exact
archive, SBOM, manifest, checksum, and signature inventory, and validates the
detached GPG signature before completing.

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
CPANEL_EXPECTED_VERSION='134.0 (build 49)'
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

GitHub Actions needs only four repository secrets: `CPANEL_HOST`,
`CPANEL_USERNAME`, `CPANEL_API_TOKEN`, and `CPANEL_API_TOKEN_NAME`. The CI
entry point derives the matching host and username guards and loads the tested
cPanel version and the non-secret account baseline from
[`scripts/cpanel-ci-baseline.env`](../scripts/cpanel-ci-baseline.env). No
GitHub environment or additional `CPANEL_EXPECTED_*` secret or variable is
required.

Initialize the dedicated account once, and again whenever the host, username,
or active API token changes:

```shell
CPANEL_INITIALIZE_TEST_ACCOUNT=1 \
  ./scripts/cpanel-initialize-test-account.sh
```

This writes a private marker to the account and immediately verifies that the
configured active token name exists exactly once. Every destructive entry
point requires the marker to match the configured host, username, and active
token.

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
the suite. Each generated remote identity is synchronously recorded in the
private artifact manifest next to the credentials file before Terraform can
create it. An `EXIT` finalizer restores the baseline again, removes only
exactly manifested identities, and requires the final test-artifact inventory
to be empty. Filesystem ownership markers are verified as a secondary
consistency check; they never authorize deletion of an unregistered path. The
manifest is kept when a command or cleanup fails so the next run can safely
recover the interrupted suite, and is deleted only after a successful
empty-inventory check.

Tests preserve unrelated account objects. Cleanup refuses ambiguous or unsafe
deletions, never removes FTP home directories by name alone, and reports
unattributed paths for manual review.

Run the acceptance suite once with each supported Terraform patch version
declared in `scripts/tool-versions.sh` before a release. For example:

```shell
PATH="$PWD/.git/tools/terraform-1.14.9:$PATH" make test-acceptance
PATH="$PWD/.git/tools/terraform-1.15.8:$PATH" make test-acceptance
```
