# Development

## Toolchain

Use the Go and Terraform versions listed in
[`compatibility.md`](compatibility.md). The validation scripts use tools from
`PATH` and fail early when a required tool is unavailable.

The CI and release configuration currently pins:

- actionlint `1.7.12`;
- golangci-lint `2.12.2`;
- govulncheck `1.6.0`;
- GoReleaser `2.17.0`;
- Syft `1.42.3`.

## cPanel acceptance environment

Acceptance tests create and delete real API tokens, cron jobs, DNS records,
Dynamic DNS domains, web domains, HTTP redirects, custom MIME types, email
accounts, Apache handlers, directory indexes and privacy, Git repositories,
Passenger applications, stored public SSL certificates, email filters,
calendar delegations, forwarders and autoresponders, FTP accounts, MySQL or MariaDB users and
databases and remote hosts, PostgreSQL users and databases, and database
grants. Locale acceptance tests temporarily change the account display locale
and restore the persisted test-account baseline. Log settings tests
temporarily change archive, pruning, and retention preferences and restore the
same durable baseline. Use a dedicated cPanel test account.

The scripts load credentials from the file specified by `CPANEL_ENV_FILE`. When
that variable is unset, they use:

```text
~/.config/terraform-provider-cpanel/acceptance.env
```

The file format matches [`.env.acceptance.example`](../.env.acceptance.example):

```text
CPANEL_HOST=https://cpanel.example.com:2083
CPANEL_USERNAME=account
CPANEL_API_TOKEN=token
```

Set its permissions to `0600`. Never commit a populated credentials file.

Destructive tests and cleanup also require a persistent singleton baseline.
The scripts load it from `CPANEL_BASELINE_FILE`; when that variable is unset,
they derive `terraform-provider-cpanel-baseline.env` next to the default
credentials file. Its format matches
[`.env.acceptance-baseline.example`](../.env.acceptance-baseline.example):

```text
CPANEL_EXPECTED_LOCALE=en
CPANEL_EXPECTED_LOG_ARCHIVE=1
CPANEL_EXPECTED_LOG_PRUNE=1
CPANEL_EXPECTED_LOG_RETENTION=-1
```

`CPANEL_EXPECTED_LOG_RETENTION=-1` means the server default. Keep this file at
`0600` and set it from the known clean account configuration, not from a test
run. The persisted values let a later run recover the account even when an
earlier Terraform or shell process was interrupted after mutation.

Run the non-destructive API and capability checks with:

```shell
make smoke-test
```

Run the Go tests that do not require cPanel with:

```shell
make test
```

Run the destructive acceptance suite with:

```shell
make test-acceptance
```

The acceptance entry point first restores the persisted singleton baseline,
then performs cleanup and the smoke test. An `EXIT` finalizer repeats singleton
restoration before artifact cleanup and final inventory verification,
including when the test command fails. It refuses to run when any credential
or baseline value is missing, or when the server does not expose API Tokens,
Cron, DNS Zone Editor, Dynamic DNS, domains, Redirects, MIME Types, email and
FTP accounts, Directory Privacy, Git Version Control, MySQL or MariaDB, and
PostgreSQL, plus ModSecurity, Passenger Applications, and SSL Manager.

Before and after the suite, the acceptance entry point removes only resources
that follow the test naming contract:

- PostgreSQL databases and users beginning with `${CPANEL_USERNAME}_tf`;
- MySQL or MariaDB databases and users beginning with
  `${CPANEL_USERNAME}_tf`;
- remote MySQL hosts limited to `198.51.100.245` through
  `198.51.100.250`;
- API token names beginning with `tfcpaneltoken`;
- Dynamic DNS domains beginning with `tfcpanelddns`;
- HTTP redirect source paths beginning with `/tfcpanelredirect-`;
- custom MIME types beginning with `application/x-tfcpanel-`;
- Apache handler extensions beginning with `.tfcpanelhandler`;
- directory index settings on top-level `public_html` directories beginning
  with `tfcpanel-index-`;
- Directory Privacy settings on top-level `public_html` directories beginning
  with `tfcpanel-privacy-`, plus only their matching password directories
  below `.htpasswds/public_html`; Directory Privacy user tests use only these
  isolated directories;
- Git repositories and top-level account-home directories beginning with
  `tfcpanel-git-`, plus only matching Git deletion markers in the account home
  or cPanel trash;
- Passenger applications beginning with `tfcpanelpassenger`; cleanup
  unregisters them before removing their matching Git fixture directories;
- stored SSL certificate friendly names beginning with `tfcpanelsslcert`;
  cleanup refuses to delete a matching certificate that cPanel reports as
  configured or installed, and re-reads both inventories immediately before
  each deletion;
- email account local parts beginning with `tfcpanel`;
- CalDAV calendar delegations whose delegator or delegatee local part begins
  with `tfcpanelcal`; cleanup removes these relationships before deleting
  either mailbox;
- login, incoming-mail, and outgoing-mail restrictions on those test email
  accounts; cleanup refuses to delete a test mailbox with held outgoing mail;
- user-level email filter names beginning with `tfcpanelfilter`, only on test
  mailboxes whose local parts begin with `tfcpanelfilter`; cleanup deletes
  these filters before deleting their mailboxes;
- email forwarder source local parts beginning with `tfcpanelfwd`;
- email domain forwarder destinations beginning with `tfcpaneldomainfwd`;
- email autoresponder local parts beginning with `tfcpanelauto`;
- FTP account names beginning with `tfcpanelftp`;
- IP blocks limited to `198.51.100.253`, `198.51.100.254`,
  `198.51.100.240-198.51.100.242`, `203.0.113.248/30`, and the
  `2001:db8:ffff::/48` documentation prefix;
- DNS record names beginning with `tfcpaneldns`;
- addon domains beginning with `tfcpaneladdon`;
- domain aliases beginning with `tfcpanelalias`;
- web subdomains beginning with `tfcpanelsub`;
- disabled ModSecurity domains beginning with
  `tfcpanelsubmodsecurity`; cleanup re-enables them before subdomain removal;
- the account locale is not prefix-addressable, so each locale acceptance test
  captures and restores its original value with an independent Go test cleanup;
  the acceptance finalizer independently restores and verifies the persisted
  locale baseline before artifact cleanup;
- account log settings are also singleton values; the finalizer restores and
  verifies the persisted `archive_logs`, `prune_archive`, and configured
  retention baseline before artifact cleanup;
- top-level test directories in `public_html` beginning with `tfcpanel-`;
- cron commands containing `# terraform-provider-cpanel-`;
- the empty `MAILTO` and default `SHELL=/bin/bash` lines that cPanel creates
  automatically when the test account has no remaining cron command.

It then requires the test-managed inventory to be empty. Resources on the
account that do not match the test naming contract are preserved. Run the
cleanup without the test suite with:

```shell
make clean-acceptance
```

## Acceptance test naming

Tests must derive resource names from `CPANEL_USERNAME` and append a unique,
short suffix. Tests must not depend on resources created outside the current
test case, and every test case must verify remote cleanup.

## Complete local validation

With the pinned tools available on `PATH`, run:

```shell
make verify
```

This verifies modules, runs race-enabled tests, vet, lint, vulnerability
scanning, documentation generation, generated-file checks, and GoReleaser
configuration validation.

Run both supported Terraform acceptance matrices by placing each Terraform
binary on `PATH` in turn:

```shell
make test-acceptance
```

## Release snapshot

Syft must be available on `PATH` before GoReleaser can create SBOMs. A local
snapshot builds every advertised platform, archives the provider, generates an
SBOM for each archive, and calculates checksums without publishing:

```shell
make release-snapshot
```

The snapshot intentionally skips GPG signing. Tagged releases import the
configured GPG key, sign the checksum file, and publish only after quality and
both Terraform acceptance jobs succeed.
