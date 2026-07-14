# Development

## Toolchain

Use the Go and Terraform versions listed in
[`compatibility.md`](compatibility.md). The validation scripts use tools from
`PATH` and fail early when a required tool is unavailable.

## cPanel acceptance environment

Acceptance tests create and delete real cron jobs, PostgreSQL users, databases,
and grants. Use a dedicated cPanel test account.

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
when any credential is missing or when the server does not expose Cron and
PostgreSQL.

## Acceptance test naming

Tests must derive resource names from `CPANEL_USERNAME` and append a unique,
short suffix. Tests must not depend on resources created outside the current
test case, and every test case must verify remote cleanup.
