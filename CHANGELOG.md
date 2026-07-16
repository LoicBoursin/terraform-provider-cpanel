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
- BoxTrapper settings resource and data source for existing mailboxes and the
  cPanel system account, with status and configuration updates, import, drift
  recovery, restoration of the complete managed pre-management settings,
  status-only support for null sender names, exact sender-name preservation
  for configuration mutations, queue-preserving cleanup, and cPanel 134
  acceptance coverage.
- Calendar delegate resource and data source for the default cPanel CalDAV
  calendar, with read-only or read/write access, composite import, drift
  recovery, strict ownership checks, current CPDAVD support, unit-tested legacy
  CCS compatibility, and cPanel 134 CPDAVD acceptance coverage.
- Complete DAV user, collection, and calendar delegation inventory data
  sources, with deterministic ordering, strict duplicate rejection, and no
  mutation operations, certified on cPanel 134 with Terraform `1.14.9` and
  `1.15.8`.
- User-level email filter resource and data source with ordered rules and
  actions, enable or disable updates, rename, import, drift recovery,
  mailbox-scoped mutation serialization, safe-action enforcement, read-only
  observation of external filters, and cPanel 134 acceptance coverage.
- Account-level email filter resource and data source with the same ordered,
  safe filter model, exact global-scope UAPI calls, import, rename, drift
  recovery, isolated cleanup, and cPanel 134 acceptance coverage.
- Mailman mailing list resource and data source with write-only administrator
  passwords, explicit password-version rotations, exact and mixed access
  settings, in-place password and privacy updates, import, drift recovery,
  exact-address deletion, and cPanel 134 acceptance coverage.
- Filesystem directory resource and data source below `public_html`, with a
  private Terraform ownership marker, safe import, non-destructive handling of
  unmarked directories, refusal to delete populated directories, and cPanel
  134 acceptance coverage.
- Filesystem UTF-8 text file resource and data source below `public_html`, with
  sensitive content, a 1 MiB limit, a private ownership token and path-specific
  sidecar containing the exact size and SHA-256 digest, non-destructive import
  of unmarked files, drift repair, refusal to delete changed content, and
  cPanel 134 acceptance coverage.
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
- Raw access log settings resource and data source with server-default or
  explicit retention, import, drift recovery, restoration of the complete
  pre-management settings, and cPanel 134 acceptance coverage.
- Account notification preferences resource and data source with a complete
  bool map, exact key-set validation, singleton import, drift recovery,
  restoration of the complete pre-management settings, JSON UAPI mutations,
  and cPanel 134 acceptance coverage.
- Resource usage data source with strict mixed scalar decoding, stable metric
  ordering, omission of internal navigation URLs, localized descriptions, and
  raw metric errors, plus cPanel 134 acceptance coverage.
- SpamAssassin preference resource and data source for `required_score`,
  `score`, `whitelist_from`, and `blacklist_from`, with exact readback,
  singleton restoration, concurrent-change protection, import, drift recovery,
  and cPanel 134 acceptance coverage.
- Public OpenSSH key inventory and singular lookup data sources with
  public-only list and fetch requests, strict RSA, Ed25519, and ECDSA
  validation, tolerant documented authorization decoding, and explicit
  exclusion of unsafe name-only mutations, certified on cPanel 134 with
  Terraform `1.14.9` and `1.15.8`.
- Complete domain inventory data source with normalized, typed, sorted, and
  duplicate-checked main, addon, subdomain, and alias identities, certified on
  cPanel 134 with Terraform `1.14.9` and `1.15.8`. Incomplete category
  responses and malformed domain names are rejected instead of publishing
  partial state.
- Stored SSL certificate signing request resource and data source with
  generation from an existing cPanel key, RSA and ECDSA public-key
  verification, in-place friendly-name updates, import, drift recovery,
  lost-response recovery without mutation replay, strict PKCS#10 validation,
  and cPanel 134 acceptance coverage.
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
- Restore persisted locale and raw-log singleton baselines from an acceptance
  `EXIT` finalizer before cleaning prefix-addressable test artifacts.
- Restore and verify the complete account notification preference baseline
  from the same acceptance finalizer.
- Restore and verify the supported SpamAssassin preference baseline from the
  same acceptance finalizer without changing unrelated custom preferences.
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
