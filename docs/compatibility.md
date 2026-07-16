---
page_title: "Compatibility - terraform-provider-cpanel"
description: |-
  Supported Terraform, Go, and cPanel versions for terraform-provider-cpanel.
---

# Compatibility

## Version policy

The provider supports the two most recent stable Terraform minor releases at
the time of a provider release. Older Terraform releases may continue to work,
but they are not part of the tested compatibility contract.

Development, documentation generation, and releases use the current stable Go
release. Patch updates are adopted promptly, especially when they contain
security fixes.

## v1.0 target matrix

| Component | Supported or certified versions |
| --- | --- |
| Terraform CLI | 1.14.x and 1.15.x |
| Terraform provider protocol | 6.0 |
| Go development toolchain | 1.26.x |
| cPanel & WHM | 134.x |
| Certified o2switch environment | 134.0 build 45 |

The o2switch certification environment uses the cPanel account API over HTTPS
on port 2083. Certification covers authentication, full-access API tokens, cron
jobs, DNS records, Dynamic DNS domains, addon domains, domain aliases, web
subdomains, HTTP redirects, email accounts, forwarders and autoresponders, FTP
accounts, directory indexes and privacy, custom MIME types, Apache handlers,
Git repositories, website IP blocks, MySQL or MariaDB databases and users,
remote MySQL hosts, PostgreSQL databases and users, imports, drift detection,
per-domain ModSecurity status, stored SSL certificates, email account
suspension, stored SSL certificate signing requests, default-calendar
delegation, user-level email filters, BoxTrapper settings, Mailman mailing
lists, public OpenPGP and OpenSSH keys, account notification preferences,
documented SpamAssassin preferences, and cleanup.
OpenSSH certification is read-only; all other keys named in this list follow
their resource-specific lifecycle policy below.
Certification also covers per-domain
email routing transitions and restoration without changing DNS MX records.
Certification also covers reading and changing the account display locale,
including restoration of its pre-test value, plus Passenger application
registration, updates, replacement, import, drift detection, remote deletion,
recreation, and cleanup.

Account capability reads use UAPI `Features::list_features`,
`StatsBar::get_stats`, and `Variables::get_user_information`. The
`cpanel_account_capabilities` data source exposes the complete feature flag map,
the cPanel version, selected account limits, and a strict allowlist of
non-sensitive account identity fields. It deliberately omits contact e-mail
addresses, IP addresses, UUIDs, API credentials, and other unrelated fields
returned by cPanel.

Resource usage reads use UAPI `ResourceUsage::get_usages`. The
`cpanel_resource_usage` data source preserves numeric precision by exposing
usage and maximum values as strings, keeps nullable limits explicit, and omits
cPanel navigation URLs, localized descriptions, and raw metric errors from
Terraform state.

## API policy

Email account, direct and domain email forwarder, autoresponder, FTP account,
MySQL, MariaDB, and PostgreSQL operations use cPanel UAPI.

API token operations use UAPI `Tokens::create_full_access`, `Tokens::list`,
`Tokens::rename`, and `Tokens::revoke`. cPanel returns a token secret only when
it creates the token. Terraform therefore stores newly created secrets as
sensitive state, while imported resources expose metadata only and cannot
recover the existing secret. Changing `expires_at` replaces the token because
cPanel does not expose an expiration update operation.

The API token that authenticates the provider must remain outside the resource
being managed. In particular, do not import that active token into
`cpanel_api_token`: cPanel does not identify the current authentication token
in list responses, and revoking it would interrupt all subsequent provider
operations.

Dynamic DNS operations use UAPI `DynamicDNS::create`,
`DynamicDNS::set_description`, `DynamicDNS::list`, and
`DynamicDNS::delete`. The webcall ID controls address updates and is therefore
treated as a secret along with the derived webcall URL. Both values remain
recoverable from cPanel after creation and import. Recreating a webcall URL
outside Terraform is observed as computed-state drift on the next refresh.
Deleting the Dynamic DNS resource also removes the DNS record that cPanel
created for it.

HTTP redirect operations use UAPI `Mime::list_redirects`,
`Mime::add_redirect`, and `Mime::delete_redirect`. A redirect is identified by
its account domain and absolute source path. Changing its destination, status
type, www matching mode, or wildcard behavior performs a verified
delete-and-create transition and attempts to restore the previous definition
if replacement fails.

cPanel reports the same `matchwww` value for rules that match both www and
non-www requests and rules that match www only. The provider therefore exposes
only the stable `both` and `without` modes and deliberately does not advertise
the unreadable www-only mode. Redirects that use that mode outside Terraform
must be changed to a supported mode before import.

Custom MIME type operations use UAPI `Mime::list_mime`, `Mime::add_mime`, and
`Mime::delete_mime`. The provider reads only user-defined MIME types and uses
the lowercase media type as stable identity. cPanel groups every extension for
one media type into a single inventory entry, so Terraform models extensions
as an unordered set. Removing or adding an extension performs a verified
delete-and-create transition and restores the previous mapping if replacement
fails.

Apache handler operations use UAPI `Mime::list_handlers`,
`Mime::add_handler`, and `Mime::delete_handler`. The provider reads only
user-defined handlers and uses the file extension as stable identity. Changing
the handler performs a verified delete-and-create transition and restores the
previous definition if replacement fails.

Directory index operations use UAPI `DirectoryIndexes::get_indexing` and
`DirectoryIndexes::set_indexing`. The provider resolves each normalized path
relative to the account home through `Variables::get_user_information` and
`Fileman::list_files`, so it can distinguish a deleted directory from an API
failure. Removing the Terraform resource preserves the directory and restores
its indexing mode to `inherit`.

Directory Privacy operations use UAPI
`DirectoryPrivacy::is_directory_protected` and
`DirectoryPrivacy::configure_directory_protection`. Paths use the same
account-home resolution as directory indexes. Removing the Terraform resource
disables HTTP Basic protection without deleting the directory, its contents,
or separately managed Directory Privacy users.

Directory Privacy authorized-user operations use UAPI
`DirectoryPrivacy::list_users`, `DirectoryPrivacy::add_user`, and
`DirectoryPrivacy::delete_user`. cPanel returns usernames but never their
passwords. Terraform therefore stores the configured password as sensitive
state, and updating a password calls `add_user` again for the same identity.
An imported user has no password in state until configuration sets it. Password
changes made outside Terraform cannot be detected during refresh.

Filesystem directory operations use UAPI `Variables::get_user_information`,
`Fileman::list_files`, `Fileman::save_file_content`, and
`Fileman::get_file_content`, plus API 2 `Fileman::mkdir` and
`Fileman::fileop`. Managed paths are normalized relative to the account home
and are restricted strictly below `public_html`; the provider never manages
the account home, `public_html` itself, the cPanel-controlled
`public_html/cgi-bin` tree, or paths containing commas because API 2 treats
commas as path separators.

Each directory created by Terraform contains a reserved
`.terraform-cpanel-directory` marker containing a random ownership token. A
matching copy is kept in Terraform private state. Destroy verifies both copies,
refuses to remove a directory containing any other entry, removes the marker,
rechecks that the directory is empty, and restores the marker if directory
deletion fails. Import recovers ownership for a valid provider marker. An
imported directory without a marker remains non-owned and is preserved even if
a marker appears later.

Filesystem text file operations use UAPI `Fileman::list_files`,
`Fileman::get_file_content`, and `Fileman::save_file_content`, plus API 2
`Fileman::mkfile` and `Fileman::fileop`. Paths follow the same normalization
and `public_html` restrictions as filesystem directories. Content must be
valid UTF-8 without null bytes and cannot exceed 1 MiB. Terraform treats the
content as sensitive state and verifies the exact returned byte size after
every read.

Each file created by Terraform has a hidden sibling named from the SHA-256
digest of its managed path. The canonical JSON sidecar contains a random
ownership token, the managed path, the exact content size, and its SHA-256
digest; the token is also kept in Terraform private state. Updates recheck both
the target and sidecar before writing. Destroy rechecks the private token,
sidecar, target identity, size, and digest, then deletes the target before the
sidecar. Terraform refuses deletion when content or ownership has changed.
Import recovers a valid matching provider marker. An imported unmarked file
remains non-owned permanently and is preserved even if a marker appears later.
The same remote path must be managed by only one Terraform state; importing it
into multiple independent states is unsupported, as for any single remote
object.

cPanel exposes no conditional Fileman write or delete operation. The provider
serializes its own mutations and re-reads the exact target and sidecar
immediately before each write or deletion, but an external mutation in the
narrow interval after the final read cannot be made atomic. Avoid concurrent
File Manager or API changes to a Terraform-managed path. On the certified
cPanel 134 environment, API 2 `Fileman::fileop` with `op=unlink` is verified
against ordinary files; link entries are not accepted as managed text files.

Git repository operations use UAPI `VersionControl::retrieve`,
`VersionControl::create`, `VersionControl::update`, and
`VersionControl::delete`. Repository roots are normalized relative to the
cPanel account home and cannot use cPanel-controlled directories. Terraform
refuses to take ownership of an existing directory or an already registered
repository implicitly; import the cPanel-managed repository instead.

The source repository URL is immutable and sensitive in state. Embedded
passwords or tokens are rejected. cPanel exposes the current branch, available
branches, clone URLs, source remote, and deployable status as read-only
metadata.

Destroying `cpanel_git_repository` is non-destructive by default: Terraform
forgets the resource while the cPanel registration and repository contents
remain intact. Setting `delete_contents_on_destroy = true` first unregisters
the repository, renames its directory to a deterministic deletion marker,
moves that marker to cPanel trash through API 2 `Fileman::fileop`, and
permanently purges only that marker through UAPI `Fileman::empty_trash`. This
explicit sequence is required because cPanel 134 can leave Git metadata behind
after `VersionControl::delete`; the provider never empties unrelated trash.

Passenger application operations use UAPI
`PassengerApps::list_applications`, `PassengerApps::register_application`,
`PassengerApps::edit_application`, and
`PassengerApps::unregister_application`. The application directory and domain
must already exist. Terraform refuses to take ownership of an application
whose name is already registered; import it instead.

The application name, path, domain, deployment mode, enabled state, and full
environment-variable map are verified after every mutation. Environment
variable values are sensitive in Terraform state. cPanel cannot edit
`base_uri`, so changing it replaces the registration. Destroy unregisters the
application without deleting its directory or executing dependency commands;
the provider deliberately never calls `PassengerApps::ensure_deps`.

Stored SSL certificate operations use UAPI `SSL::list_certs`,
`SSL::show_cert`, `SSL::upload_cert`, `SSL::set_cert_friendly_name`,
`SSL::delete_cert`, and `SSL::installed_hosts`. The resource accepts and
uploads only one public X.509 certificate. It never accepts, reads, sends, or
stores a private key.

The resource manages only certificates that cPanel reports as neither
configured for a domain nor installed on an SSL virtual host. Import, update,
and destroy fail safely if either condition is true. Changing the friendly
name updates the existing certificate; changing the X.509 certificate replaces
the resource. Equivalent PEM formatting is treated as the same certificate,
and cPanel may reuse its deterministic certificate ID after remote deletion
and recreation.

Safety decisions require explicit certificate and installed-host inventory
fields. Missing, `null`, or malformed status data fails the operation instead
of being treated as an unconfigured or uninstalled certificate. If an upload
fails without returning a trustworthy new certificate ID, Terraform does not
guess ownership from a later inventory and therefore does not attempt
destructive cleanup.

Stored SSL certificate signing request operations use UAPI `SSL::list_keys`,
`SSL::list_csrs`, `SSL::show_csr`, `SSL::generate_csr`,
`SSL::set_csr_friendly_name`, and `SSL::delete_csr`. The resource uses an
existing cPanel key ID and reads only its public algorithm, RSA modulus, or
ECDSA curve and public point. It never reads, creates, uploads, changes, or
deletes private-key material.

The provider parses and verifies the signed PKCS#10 request, permits only DNS
subject alternative names, and rejects extra extensions, attributes, subject
values, or malformed response types. Generation succeeds only when the CSR
public key matches the selected key's public metadata. A lost or malformed
generation response is not retried: for up to 30 seconds, the provider polls
the read-only post-operation inventory against its baseline and adopts only one
newly observed CSR whose definition and public key match exactly.

The reconciliation continues even when the original Terraform request is
canceled because the generation POST may already have reached cPanel; it never
replays that POST. cPanel exposes no mutation idempotency key, so a concurrently
created CSR with the exact same subject, domains, friendly name, and public key
is indistinguishable and may be adopted as an equivalent object after an
ambiguous response. Avoid concurrent generation of identical CSRs outside
Terraform.

`SSL::list_csrs` is the source of truth for RSA and ECDSA public-key metadata.
`SSL::show_csr` may omit optional algorithm-specific fields, but any field it
does return must agree with the inventory. The provider then binds the
inventory metadata directly to the signed PKCS#10 public key.

If generation succeeds but Terraform cannot persist local state, the provider
reports the verified CSR ID for import and leaves the remote object unchanged.
It does not delete an object whose creation provenance may be ambiguous.

The cPanel API does not expose conditional rename or delete operations.
Terraform therefore re-reads and verifies the CSR ID, SHA-256 PKCS#10
fingerprint, and expected current friendly name immediately around mutations.
An external mutation in the narrow interval between the final read and write
cannot be made atomic; ambiguous or conflicting observations fail without
guessing ownership.

Public OpenPGP key operations use UAPI `GPG::list_public_keys`,
`GPG::list_secret_keys`, `GPG::import_key`, and `GPG::export_public_key`. The
resource accepts exactly one armored RSA version 4 public key with a 2048,
3072, or 4096-bit primary key. It accepts non-semantic ASCII armor headers and
rejects private-key packets, unknown or non-transferable packet tags, multiple
raw primary-key packets, unsupported algorithms or versions, weak keys,
oversized input, and keys without an identity before any cPanel mutation.

The provider never imports, exports, reads, manages, or deletes private-key
material. It reads secret-key metadata before creation, import, and refresh,
and refuses resource management whenever a matching secret ID appears. Stable
identity combines the complete uppercase 40-character primary fingerprint and
the lowercase SHA-256 of the canonical set of decoded public packets. Harmless
armor and packet ordering differences are ignored, while added, removed, or
changed identities, subkeys, certifications, or other public packets are
detected instead of adopted.

cPanel exposes only `GPG::delete_keypair`; there is no public-only deletion
operation. The Terraform resource deliberately never calls that function.
Destroy and replacement remove the old object from Terraform state while
preserving its remote public key. If Terraform cannot save state after a
successful import, the diagnostic reports the exact key ID for a subsequent
import instead of risking pair deletion.

Import is refused when the public key already existed before the requested
creation, and a successful import must return the expected key ID,
fingerprint, complete public packet digest, and no matching secret key.
Deterministic API errors are never treated as success. Any post-import
transport, response-decoding, delayed-visibility, or transient read failure is
reconciled for a bounded period through a cancellation-independent read-only
context; the import mutation is never replayed.

Acceptance cleanup can call `GPG::delete_keypair` only on the dedicated test
account, with `CPANEL_ALLOW_GPG_KEYPAIR_DELETE=1`, an expected secret-key count
of zero, an empty live secret inventory, an exact reserved user-ID prefix, and
two matching public exports. It sends the deletion POST once and reconciles an
ambiguous response through read-only inventories. Because cPanel offers no
conditional operation, a secret key imported concurrently after the final read
remains an unavoidable server-side race; production resource deletion is
disabled for that reason.

Public OpenSSH reads use the remaining cPanel API 2 `SSH::listkeys` and
`SSH::fetchkey` functions because cPanel does not expose UAPI equivalents.
Every request hard-codes `pub=1`, so the provider never requests private-key
metadata or material. The inventory accepts the documented boolean and 0/1
authorization fields as well as the textual status values observed on cPanel
134, ignores additive response fields, and rejects contradictory identity or
authorization data.

The provider deliberately exposes no mutable OpenSSH resource.
`SSH::importkey` can overwrite a concurrent key with the same name, while
`SSH::authkey` and `SSH::delkey` identify the target only by name. cPanel
offers no atomic create-if-absent or fingerprint-conditioned mutation, so a
client-side read cannot close the replacement window. Automatic SSH cleanup is
disabled for the same reason; a reserved `tfcpanelssh` key causes cleanup to
fail for manual review instead of mutating it.

DNS record reads and mutations use UAPI `DNS::parse_zone` and
`DNS::mass_edit_zone`. Every mutation uses the current SOA serial and is
verified against the parsed zone. Records are tracked by cPanel line index,
name, and type; the provider can relocate a record after unrelated zone edits
unless duplicate records make the identity ambiguous. The certified record
types are `A`, `AAAA`, `CAA`, `CNAME`, `MX`, `SRV`, and `TXT`. The certified
account rejects account-level `NS` and `PTR` creation. Its capability endpoints
report optional `ALIAS` and `HTTPS` support as disabled, and it does not expose
the `SVCB` capability function.

The email account resource deliberately uses the email-service API instead of
creating a cPanel subaccount. It manages only the mailbox and cannot
accidentally enable or delete FTP and WebDisk services that share a username.

Calendar delegation reads use `CPDAVD::list_users` and
`CPDAVD::list_delegates` on the certified cPanel 134 environment. cPanel 120
and later use `CPDAVD`; older releases can expose the legacy `CCS` module.
The client probes `CPDAVD` first and falls back to `CCS` only when cPanel
explicitly reports that `CPDAVD` is unavailable. It never retries an ambiguous
write through another module. The `CPDAVD` path is acceptance-tested on cPanel
134; the legacy `CCS` response and mutation formats are covered by unit tests
but are not acceptance-certified.

The input-free DAV inventory data sources expose complete mailbox usernames,
collection identifiers, collection display names and types, and delegation
relationships. They deliberately omit CPDAVD or CCS internal identifiers and
descriptions. Mailbox addresses, display names, and delegation relationships
are stored in plaintext Terraform state and must be protected accordingly.
Both inventories are normalized by the client, sorted by stable identities,
and rejected when cPanel returns duplicate object keys or delegation
identities.

The resource manages only the default collection identifier `calendar`.
Create and update require both complete mailbox addresses to expose that
CalDAV collection. Terraform refuses an existing relationship unless it is
imported explicitly, verifies every access change, and removes only the exact
delegator, calendar, and delegatee identity it owns. Import identifiers use
`delegator|calendar|delegatee`.

The provider serializes mutations for the same delegation identity within one
process and performs exact state reads around create, update, deletion, and
rollback. cPanel does not expose conditional delegation mutations, so an
out-of-band change in the narrow interval between a read and its following
write cannot be made atomic. In particular, after an ambiguous transport
failure, an identical relationship created concurrently outside Terraform may
be indistinguishable from the attempted creation. Deterministic cPanel API and
HTTP errors are never treated as successful creation and never authorize a
create rollback.

Email account suspension reads use `Email::list_pops_with_disk` with
`get_restrictions=1`. On the certified cPanel 134 environment,
`suspended_login` is `null` rather than `0` when login is allowed; the provider
normalizes that value to `false` while rejecting a missing restriction field.
Mutations use the separate `suspend_*` and `unsuspend_*` UAPI operations for
login, incoming mail, and outgoing mail. Terraform reports held outgoing mail
but never calls `hold_outgoing` or `release_outgoing`, because releasing a
queue is an imperative delivery action. Removing the resource unsuspends the
three managed restrictions without deleting the mailbox or releasing held
mail.

BoxTrapper account discovery uses API 2
`BoxTrapper::accountmanagelist`. Status and configuration reads use UAPI
`BoxTrapper::get_status` and `BoxTrapper::get_configuration`; mutations use
`BoxTrapper::set_status` and `BoxTrapper::save_configuration`. The mailbox or
cPanel system account must already exist.

Terraform manages enabled status, automatic allowlisting, sender addresses,
queue retention, SpamAssassin score, and association-based allowlisting. cPanel
stores the score to one decimal place, so the provider rejects values that
cannot be represented exactly at that precision. `from_name` is read-only:
cPanel reports null for a new mailbox but cannot restore that null after an
explicit name has been saved. The enabled status can still be changed while
`from_name` is null. Before any configuration mutation, the provider requires
a non-null observed name, sends that exact value to
`BoxTrapper::save_configuration`, and verifies that it remains unchanged.
Terraform refuses the mutation before the POST when the name is null.

The resource captures all managed values before its first mutation and restores
them on destroy. Complete transitions are serialized per account and apply
configuration before status. Ambiguous failures trigger a bounded reread and
rollback only while the observed settings still match the attempted
transition. Terraform does not manage challenge queues, logs, allowlists,
blocklists, templates, or messages. Acceptance cleanup checks
`BoxTrapper::list_queued_messages`, refuses to delete a test mailbox with a
non-empty queue, repeats that check immediately before mailbox deletion, and
never deletes queued messages.

User-level email filter operations use UAPI `Email::list_filters`,
`Email::get_filter`, `Email::store_filter`, `Email::enable_filter`,
`Email::disable_filter`, and `Email::delete_filter`. The mailbox must already
exist.

Account-level email filters use the same UAPI operations but deliberately omit
the `account` parameter from every request. Supplying the cPanel username on
the certified cPanel 134 environment selects a different and incompatible
scope. The computed `account` attribute records the cPanel username in
Terraform state, while the filter name is the import identity.

Raw access log settings use UAPI `LogManager::get_settings` and
`LogManager::set_settings`. The resource owns the complete account-level
archive, monthly pruning, and retention configuration. A `retention_days`
value of `-1` clears the per-account override and uses the server default;
`effective_retention_days` reports the account's effective value, where `0`
means indefinite retention. Removing the resource restores the full
configuration captured before Terraform management. The acceptance wrapper
uses a persisted clean-account baseline and an `EXIT` finalizer to restore
these singleton values before artifact cleanup, including after a failed test.

Account notification preference operations use UAPI
`ContactInformation::get_notification_preferences` and
`ContactInformation::set_notification_preferences`. The mutation endpoint
receives a JSON object, and the provider verifies the full readback after every
change. The resource requires the configured key set to exactly match the
account inventory so a new or removed cPanel preference cannot be silently
ignored. Removing the resource restores the complete bool map captured before
Terraform management. The acceptance finalizer independently restores and
verifies a persisted clean-account JSON baseline.

SpamAssassin preference operations use UAPI
`SpamAssassin::get_user_preferences` and
`SpamAssassin::update_user_preference`. The provider deliberately exposes only
the documented `required_score`, `score`, `whitelist_from`, and
`blacklist_from` keys. It does not provide an arbitrary custom-configuration
escape hatch. Configured values are represented as an unordered set and every
mutation is followed by an exact readback. Removing the resource restores
whether the preference existed and its complete prior value set. A guarded
restore refuses to overwrite a value that no longer matches the Terraform
transition. The acceptance finalizer independently restores the four-key
clean-account baseline and leaves unrelated custom preferences unchanged.

The resource preserves rule and action order, supports cPanel's string and
numeric match operators, and limits actions to `deliver`, `fail`, and
`finish`. It rejects `save` and `pipe`, which can write files or execute
commands. The read-only data source can still observe those external actions
without taking ownership. cPanel expands `save` destinations in
`list_filters` but returns logical mailbox-relative paths in `get_filter`; the
provider canonicalizes that known difference while continuing to verify both
inventories.

Each complete filter read or mutation sequence is serialized per mailbox or
for the complete account-level filter scope. Creating a resource refuses an
existing filter instead of taking ownership implicitly, and rollback deletion
requires the complete definition and enabled state to match the attempted
state. Import identifiers use `account|name` for mailbox-level filters and
`name` for account-level filters.

Mailman mailing list operations use UAPI `Email::list_lists`,
`Email::add_list`, `Email::passwd_list`,
`Email::set_list_privacy_options`, and `Email::delete_list`. The complete list
address is the resource identity. Terraform manages the exact `advertised`,
`archive_private`, and `subscribe_policy` values. The computed `private`
summary is true only when the list is not advertised, its archive is private,
and subscription requires administrator approval; every other valid
combination is public.

cPanel never returns the administrator password. The `password` attribute is
write-only, so Terraform does not store it in plan or state artifacts.
Configure it together with a positive `password_version`. Changing that
version applies the accompanying password in place. Omitting both attributes
after import preserves the remote password; setting both performs the first
managed rotation.

Creation starts with private settings before applying the exact configured
options. Password and privacy updates are serialized per complete address and
use cPanel's in-place operations, preserving subscribers, archives, and
Mailman settings outside Terraform state. Destroy still deletes the complete
Mailman list and its nested data. Create refuses an existing list, rollback is
allowed only after cPanel confirms the creation and is bounded by the internal
list identifier plus exact attempted privacy. An ambiguous creation response
never adopts or deletes the observed list automatically. Cleanup deletes only
complete addresses whose local part begins with the reserved test prefix; that
prefix is the ownership boundary for disposable acceptance-test lists.

Email routing operations use UAPI `Email::list_mxs` and
`Email::set_always_accept`. The resource manages cPanel's local delivery
classification for one existing account domain; it does not create, update, or
delete DNS MX records. Terraform exposes `backup` while cPanel represents that
mode as `secondary` in API responses.

The provider reads the complete unfiltered routing inventory because cPanel
134 does not reliably scope `list_mxs` to a requested domain. It validates the
one-hot routing flags, detected mode, ordered MX entries, primary exchanger,
status, and the complete mutation response before accepting state. A domain
with no MX entry has a null `primary_exchanger`.

The resource captures the configured mode that preceded Terraform management
and restores it on destroy. Transitions are serialized per domain. Ambiguous
mutation failures trigger a bounded reread and rollback only when the observed
state still matches the attempted transition; deterministic cPanel rejections
and concurrent out-of-band changes are not overwritten. cPanel warnings are
reported as Terraform warnings.

The email forwarder resource manages one direct source-to-destination email
address pair. cPanel permits multiple destinations for the same source address,
so both addresses form the resource identity. Domain-level forwarders, failure
routes, pipes, and system-account routes use different cPanel semantics and are
not represented by this resource.

The email domain forwarder resource manages the single destination associated
with one source mail domain. Changing the destination performs a verified
delete-and-create transition and attempts to restore the previous destination
if the replacement fails.

The email autoresponder resource manages all fields returned by cPanel,
including its schedule and HTML flag. cPanel stores a trailing newline in the
message body; the provider normalizes that server-added newline so refreshes do
not create a perpetual diff.

IP-block mutations use UAPI `BlockIP::add_ip` and `BlockIP::remove_ip`. The
current blocked-address inventory is read with the cPanel API 2
`DenyIp::listdenyips` function because cPanel does not expose an equivalent
UAPI read operation. IPv4, CIDR, explicit ranges, and IPv6 are normalized
before comparison. When cPanel decomposes an explicit range into multiple CIDR
or single-address entries, the provider reassembles them only when they cover
the requested range exactly. The expanded range bounds remain available as
computed state.

The FTP resource manages cPanel virtual FTP accounts. Its home directory is
relative to the cPanel account home, and Terraform preserves that directory by
default when deleting the account. cPanel SFTP access uses the main cPanel
system account and does not have an independent account lifecycle API, so the
provider does not advertise a separate SFTP resource.

Remote MySQL host mutations and notes use UAPI `Mysql::add_host`,
`Mysql::add_host_note`, and `Mysql::delete_host`. The authoritative host
inventory uses the remaining API 2 `MysqlFE::listhosts` function because UAPI
only returns notes and omits authorized hosts that have no note. Host and note
changes replace the authorization. This avoids the unreliable in-place note
update observed on the certified cPanel 134 environment. IPv4 addresses, IPv4
CIDR prefixes, cPanel percent-wildcard IPv4 patterns, and hostnames are
normalized before comparison.

Per-domain ModSecurity reads and mutations use UAPI
`ModSecurity::list_domains`, `ModSecurity::enable_domains`, and
`ModSecurity::disable_domains`. The provider deliberately does not call the
account-wide enable or disable functions. cPanel can report related domains
that a change also affects; Terraform exposes both those dependencies and the
complete affected-domain set. Removing the resource restores the enabled
state without deleting the domain.

Account locale reads use UAPI `Locale::get_attributes` and
`Locale::list_locales`. Mutations use `Locale::set_locale` over POST and are
verified against a fresh read. Terraform stores the locale observed when it
first takes ownership and restores that value when the resource is removed.
Import uses the singleton identifier `account`; destroying an unchanged import
therefore leaves the account locale unchanged.

Cron operations currently use cPanel API 2 because cPanel does not provide UAPI
equivalents for the required cron functions. API 2 is deprecated, so each
provider release must run the cron acceptance suite against the certified
cPanel environment.

Addon-domain, domain-alias, and web-subdomain creation and deletion, plus
document-root changes, use cPanel API 2 because cPanel does not provide
equivalent account-level UAPI mutations. A domain alias always targets the
account main domain and shares its `public_html` document root; Terraform never
deletes that shared directory. The complete read-only domain inventory uses
UAPI `DomainInfo::list_domains`, maps parked domains to the `alias` category,
normalizes names, sorts the combined result, and rejects a domain reported in
multiple categories. It exposes no document roots or internal domain keys.
Every release must repeat the domain acceptance suites against the certified
environment.

## Concurrency

The provider serializes all cPanel requests through its shared client. Users do
not need to disable Terraform parallelism. Email filters additionally lock the
complete read, mutate, verify, and rollback sequence per mailbox so parallel
filter resources cannot overwrite one another between individual API calls.

Each HTTP request has a 90-second timeout and honors Terraform context
cancellation. The certified environment can take more than 30 seconds to
rebuild web-server and DNS configuration after a domain mutation. The provider
does not automatically retry API calls. cPanel mutations are not reliably
idempotent, so retrying after an interrupted response could apply an operation
twice. Terraform instead refreshes the remote state before deciding what must
be retried.

## Compatibility evidence

A release is compatible only when:

- unit tests pass without a cPanel account;
- acceptance tests pass on every supported Terraform minor release;
- the complete acceptance suite passes on the certified cPanel environment;
- a second refresh after the test run reports no unexpected state changes;
- the test account contains no residual resources after cleanup.

The release workflow repeats the complete acceptance suite on Terraform
`1.14.9` and `1.15.8` before it can sign and publish artifacts.
