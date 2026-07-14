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
jobs, DNS records, addon domains, domain aliases, web subdomains, email
accounts, forwarders and autoresponders, FTP accounts, website IP blocks,
MySQL or MariaDB databases and users, PostgreSQL databases and users, imports,
drift detection, and cleanup.

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
