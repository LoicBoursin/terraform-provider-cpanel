# Terraform Provider cPanel

Terraform provider for managing account-level API tokens, cron jobs, DNS
records, Dynamic DNS domains, web domains, HTTP redirects, email accounts,
directory indexes, custom MIME types, Apache handlers, forwarders,
autoresponders, FTP accounts, and MySQL, MariaDB, and PostgreSQL users and
databases, plus website IP blocks, through the cPanel API.

## Status

Version `0.1.0` is the latest published release. The current source tree is the
`v1.0` release candidate, certified against o2switch cPanel `134.0` build `44`.

The supported versions and API policy are documented in
[`docs/compatibility.md`](docs/compatibility.md).
## Supported resources

- `cpanel_addon_domain`
- `cpanel_apache_handler`
- `cpanel_api_token`
- `cpanel_cron_job`
- `cpanel_directory_index`
- `cpanel_dns_record`
- `cpanel_domain_alias`
- `cpanel_dynamic_dns`
- `cpanel_email_account`
- `cpanel_email_auto_responder`
- `cpanel_email_domain_forwarder`
- `cpanel_email_forwarder`
- `cpanel_ftp_account`
- `cpanel_ip_block`
- `cpanel_mime_type`
- `cpanel_mysql_database`
- `cpanel_mysql_user`
- `cpanel_postgresql_database`
- `cpanel_postgresql_user`
- `cpanel_redirect`
- `cpanel_subdomain`

Matching data sources are available for each resource family.

## Requirements

- Terraform CLI `1.14.x` or `1.15.x`
- Go `1.26.x` for development
- cPanel `134.x` over HTTPS with API Tokens, Cron, DNS Zone Editor, Dynamic
  DNS, domains, Redirects, Index Manager, MIME Types, Apache Handlers, email
  accounts, forwarders and autoresponders, FTP accounts, MySQL or MariaDB, and
  PostgreSQL enabled, plus the IP Blocker feature

## Configuration

Keep credentials outside Terraform configuration:

```shell
export CPANEL_HOST="https://cpanel.example.com:2083"
export CPANEL_USERNAME="account"
export CPANEL_API_TOKEN="token"
```

Then configure the provider without embedding secrets:

```terraform
provider "cpanel" {}
```

The same values can be supplied through the `host`, `username`, and
`api_token` provider attributes when required. `api_token` is sensitive, but
Terraform configuration and state must still be protected.

The provider serializes cPanel requests internally. Standard `terraform plan`
and `terraform apply` commands are supported; no manual `-parallelism=1` flag
is required.

## Development

Build and run the local test suite:

```shell
go install
make test
make lint
```

Generate Registry documentation:

```shell
make generate-documentation
```

Run the destructive cPanel acceptance suite:

```shell
make test-acceptance
```

Acceptance credentials, cleanup rules, and release validation are documented
in [`docs/development.md`](docs/development.md).
