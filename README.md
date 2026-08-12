# Terraform Provider cPanel

Terraform provider for managing account-level API tokens, cron jobs, DNS
records, Dynamic DNS domains, account locale, account notification preferences,
documented SpamAssassin preferences,
web domains, HTTP redirects,
email accounts, calendar delegations, account-level filters and mailbox-level
filters, stored SSL certificates and
certificate signing requests, public-only OpenPGP keys, read-only public
OpenSSH keys, Passenger
applications, raw access log settings, ModSecurity settings, filesystem
directories and UTF-8 text files, directory indexes and privacy, Git
repositories, custom MIME types, Apache handlers, forwarders, autoresponders,
BoxTrapper settings, Mailman mailing lists, FTP accounts, and MySQL, MariaDB,
and PostgreSQL users and databases, remote MySQL hosts, plus website IP blocks
and email routing, through the cPanel API.

## Status

The current source tree is version `1.0.0` and is tested against o2switch
cPanel `134.0` build `49`.

The supported versions and API policy are documented in
[`docs/compatibility.md`](docs/compatibility.md).
## Supported resources

- `cpanel_account_email_filter`
- `cpanel_addon_domain`
- `cpanel_apache_handler`
- `cpanel_api_token`
- `cpanel_boxtrapper_settings`
- `cpanel_calendar_delegate`
- `cpanel_cron_job`
- `cpanel_directory_index`
- `cpanel_directory_privacy`
- `cpanel_directory_privacy_user`
- `cpanel_dns_record`
- `cpanel_domain_alias`
- `cpanel_dynamic_dns`
- `cpanel_email_account`
- `cpanel_email_account_suspension`
- `cpanel_email_auto_responder`
- `cpanel_email_domain_forwarder`
- `cpanel_email_filter`
- `cpanel_email_forwarder`
- `cpanel_email_mailing_list`
- `cpanel_email_routing`
- `cpanel_filesystem_directory`
- `cpanel_filesystem_text_file`
- `cpanel_ftp_account`
- `cpanel_gpg_public_key`
- `cpanel_git_repository`
- `cpanel_ip_block`
- `cpanel_locale`
- `cpanel_log_settings`
- `cpanel_notification_preferences`
- `cpanel_mime_type`
- `cpanel_modsecurity_domain`
- `cpanel_mysql_database`
- `cpanel_mysql_remote_host`
- `cpanel_mysql_user`
- `cpanel_passenger_application`
- `cpanel_postgresql_database`
- `cpanel_postgresql_user`
- `cpanel_redirect`
- `cpanel_spam_preference`
- `cpanel_ssl_certificate`
- `cpanel_ssl_csr`
- `cpanel_subdomain`

Matching data sources are available for each resource family.

Additional read-only account data sources:

- `cpanel_account_capabilities`
- `cpanel_calendar_delegates`
- `cpanel_dav_users`
- `cpanel_domains`
- `cpanel_email_accounts`
- `cpanel_email_auto_responders`
- `cpanel_email_domain_forwarders`
- `cpanel_email_domains`
- `cpanel_email_mailing_lists`
- `cpanel_email_routings`
- `cpanel_mysql_databases`
- `cpanel_mysql_remote_hosts`
- `cpanel_mysql_restrictions`
- `cpanel_mysql_users`
- `cpanel_resource_usage`
- `cpanel_ssh_public_key`
- `cpanel_ssh_public_keys`
- `cpanel_ssl_certificates`
- `cpanel_ssl_csrs`
- `cpanel_ssl_installed_hosts`
- `cpanel_ssl_keys`

## Requirements

- Terraform CLI `1.14.x` or `1.15.x`
- Go `1.26.x` for development
- cPanel `134.x` over HTTPS with API Tokens, Cron, DNS Zone Editor, Dynamic
  DNS, domains, Redirects, Index Manager, Directory Privacy, MIME Types,
  Apache Handlers, ModSecurity, Git Version Control, Passenger Applications,
  SSL Manager with an existing RSA key, File Manager, account locales, Raw
  Access log settings, Contact Information notification preferences,
  SpamAssassin user preferences,
  public-key GPG import and inventory, public SSH key inventory, email
  accounts,
  BoxTrapper, account-level and mailbox-level filters, forwarders and
  autoresponders, email routing, CalDAV
  calendar delegation, Mailman mailing lists, FTP accounts, MySQL or MariaDB
  with remote-host access, and PostgreSQL enabled, plus the IP Blocker feature

## Configuration

Keep credentials outside Terraform configuration:

```shell
export CPANEL_HOST="https://cpanel.example.com:2083"
export CPANEL_USERNAME="account"
export CPANEL_API_TOKEN="token"
export CPANEL_API_TOKEN_NAME="terraform-provider"
```

Then configure the provider without embedding secrets:

```terraform
provider "cpanel" {}
```

The same values can be supplied through the `host`, `username`, `api_token`,
and `api_token_name` provider attributes when required. `api_token` is
sensitive, but Terraform configuration and state must still be protected.
`api_token_name` lets the provider refuse to rename or revoke its own
authentication token and is required before renaming or destroying an imported
`cpanel_api_token` resource.

The provider serializes cPanel requests internally. Standard `terraform plan`
and `terraform apply` commands are supported; no manual `-parallelism=1` flag
is required.

## Development

Build and run the local test suite:

```shell
go install
make tools
make verify
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
