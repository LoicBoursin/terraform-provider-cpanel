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

- Email account resource and data source with password and quota updates,
  import, replacement, drift recovery, and cPanel 134 acceptance coverage.
- MySQL and MariaDB user and database resources, matching data sources, import,
  drift recovery, privilege management, and cPanel 134 acceptance coverage.
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
