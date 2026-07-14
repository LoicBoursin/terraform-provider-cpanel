# Terraform Provider cPanel

## Project Status

Version `0.1.0` is an experimental release from February 2024. The current
`v1.0` effort is focused on making the existing cron and PostgreSQL resources
production-ready and certifying them against o2switch cPanel 134.

The supported-version policy is documented in
[`docs/compatibility.md`](docs/compatibility.md).

## Available Resources

The v1.0 scope contains:

- Cron Jobs
- PostgreSQL Databases & Users

Other cPanel resource families are outside the v1.0 scope and can be added in
later releases.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.4
- [Go](https://golang.org/doc/install) >= 1.20

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the provider

You **MUST** disable parallelism when using the provider, because cPanel's API does not support concurrent requests. 
To do so, you can run the defined commands:

```shell
make plan
```

```shell
make apply
```


Or you can manually:
- Set an environment variable: `export TF_CLI_ARGS_apply="-parallelism=1"`
- Set a CLI flag: `terraform apply -parallelism=1`

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run:

```shell
make generate-documentation
```

To lint the code, run:

```shell
make lint
```

In order to run the full suite of Acceptance tests, run:

```shell
make test
```

*Note:* Acceptance tests create real resources, and requires a real cPanel account to run. Please be aware of the costs associated with running acceptance tests. For more information, refer to the [Acceptance Testing](https://www.terraform.io/docs/extend/testing/acceptance-tests/index.html) documentation.
