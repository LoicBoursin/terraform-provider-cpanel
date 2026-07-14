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

Acceptance tests create and delete real cron jobs, DNS records, domains, email
accounts, forwarders and autoresponders, FTP accounts, MySQL or MariaDB users
and databases, PostgreSQL users and databases, and database grants. Use a
dedicated cPanel test account.

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

The acceptance entry point performs the smoke test first. It refuses to run
when any credential is missing or when the server does not expose Cron, DNS
Zone Editor, domains, email and FTP accounts, MySQL or MariaDB, and PostgreSQL.

Before and after the suite, the acceptance entry point removes only resources
that follow the test naming contract:

- PostgreSQL databases and users beginning with `${CPANEL_USERNAME}_tf`;
- MySQL or MariaDB databases and users beginning with
  `${CPANEL_USERNAME}_tf`;
- email account local parts beginning with `tfcpanel`;
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
