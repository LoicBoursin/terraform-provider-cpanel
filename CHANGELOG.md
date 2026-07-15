# Changelog

## Unreleased

### Breaking changes

- Require an HTTPS cPanel endpoint.
- Support Terraform `1.14.x` and `1.15.x`.
- Remove `last_updated` from resources and data sources.
- Remove `password` from the PostgreSQL user data source.
- Make PostgreSQL user resource passwords write-only and require a matching
  positive `password_version`. Before upgrading an existing configuration,
  remove `password` to preserve the current remote password, or add
  `password_version = 1` to intentionally set the configured password.
- Make PostgreSQL database `users` an unordered set.

### Added

- Full-access API token resource and metadata data source with expiration,
  rename, import, drift recovery, sensitive state, and cPanel 134 acceptance
  coverage.
- Dynamic DNS resource and data source with description updates, sensitive
  webcall metadata, import, replacement, drift recovery, and cPanel 134
  acceptance coverage.
- HTTP redirect resource and data source with permanent or temporary status,
  stable www matching modes, wildcard behavior, import, drift recovery, and
  cPanel 134 acceptance coverage.
- Custom MIME type resource and data source with unordered extensions, import,
  replacement rollback, drift recovery, and cPanel 134 acceptance coverage.
- Apache handler resource and data source with import, replacement rollback,
  drift recovery, and cPanel 134 acceptance coverage.
- Directory index resource and data source with inherited-setting cleanup,
  import, rollback, drift recovery, and cPanel 134 acceptance coverage.
- Directory Privacy resource and data source with in-place authentication-label
  updates, import, drift recovery, non-destructive removal, and cPanel 134
  acceptance coverage.
- Directory Privacy authorized-user resource and data source with sensitive
  password state, password updates, composite import, isolated deletion, and
  cPanel 134 acceptance coverage.
- Git repository resource and data source with optional source cloning, name
  updates, import, replacement, drift recovery, non-destructive removal by
  default, explicit recursive deletion, and cPanel 134 acceptance coverage.
- Email account resource and data source with password and quota updates,
  import, replacement, drift recovery, and cPanel 134 acceptance coverage.
- Email account suspension resource and data source with independently managed
  login, incoming-mail, and outgoing-mail restrictions, import, drift
  recovery, safe reset on removal, and cPanel 134 acceptance coverage.
- Calendar delegate resource and data source for the default cPanel CalDAV
  calendar, with read-only or read/write access, composite import, drift
  recovery, strict ownership checks, current CPDAVD support, unit-tested legacy
  CCS compatibility, and cPanel 134 CPDAVD acceptance coverage.
- User-level email filter resource and data source with ordered rules and
  actions, enable or disable updates, rename, import, drift recovery,
  mailbox-scoped mutation serialization, safe-action enforcement, read-only
  observation of external filters, and cPanel 134 acceptance coverage.
- MySQL and MariaDB user and database resources, matching data sources, import,
  drift recovery, privilege management, and cPanel 134 acceptance coverage.
- Remote MySQL host resource and data source with IPv4, CIDR, wildcard, and
  hostname identities, optional notes, import, drift recovery, isolated
  cleanup, and cPanel 134 acceptance coverage.
- ModSecurity domain resource and data source with dependency metadata,
  in-place enable or disable updates, import, drift recovery, safe reset on
  removal, and cPanel 134 acceptance coverage.
- Account locale resource and data source with import, drift recovery,
  restoration of the pre-management locale, and cPanel 134 acceptance
  coverage.
- Import support for cron jobs by cPanel line key.
- Stable drift and remote-deletion handling for every resource.
- cPanel 134 acceptance coverage for CRUD, import, drift, recreation, and
  cleanup.
- Complete client, schema, validation, and state-mapping unit tests.
- Signed release automation with per-archive SBOMs and checksums.

### Changed

- Serialize cPanel requests internally instead of requiring
  `-parallelism=1`.
- Send mutations with POST, honor Terraform cancellation, and enforce request
  timeouts and response-size limits.
- Validate cron expressions and cPanel-prefixed PostgreSQL names before API
  mutation.
- Verify PostgreSQL privilege changes and require inspection and re-import
  after partial non-atomic failures instead of attempting automatic rollback.

### Security

- Keep API tokens and database passwords out of URLs and diagnostics.
- Pin CI actions and release tools.
- Add race, lint, vulnerability, dependency, and workflow validation.

## 0.1.0 - 2024-02-10

- Initial experimental cron and PostgreSQL provider release.
