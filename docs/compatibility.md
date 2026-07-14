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
| Certified o2switch environment | 134.0 build 44 |

The o2switch certification environment uses the cPanel account API over HTTPS
on port 2083. Certification covers authentication, full-access API tokens, cron
jobs, DNS records, Dynamic DNS domains, addon domains, domain aliases, web
subdomains, HTTP redirects, email accounts, forwarders and autoresponders, FTP
accounts, directory indexes and privacy, custom MIME types, Apache handlers,
Git repositories, website IP blocks, MySQL or MariaDB databases and users,
remote MySQL hosts, PostgreSQL databases and users, imports, drift detection,
per-domain ModSecurity status, and cleanup.

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

Cron operations currently use cPanel API 2 because cPanel does not provide UAPI
equivalents for the required cron functions. API 2 is deprecated, so each
provider release must run the cron acceptance suite against the certified
cPanel environment.

Addon-domain, domain-alias, and web-subdomain creation and deletion, plus
document-root changes, use cPanel API 2 because cPanel does not provide
equivalent account-level UAPI mutations. A domain alias always targets the
account main domain and shares its `public_html` document root; Terraform never
deletes that shared directory. Reads use the domain inventory exposed by
cPanel, and every release must repeat the domain acceptance suites against the
certified environment.

## Concurrency

The provider serializes all cPanel requests through its shared client. Users do
not need to disable Terraform parallelism.

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
