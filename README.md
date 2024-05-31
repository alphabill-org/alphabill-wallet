## Alphabill Wallet

Client side software to interface with Alphabill network. This repository contains the following components:
* Money partition CLI wallet
* User Token partition CLI wallet
* EVM partition client

## Update Alphabill dependency

`go get github.com/alphabill-org/alphabill@<commit-id>`

## Building the source

* Requires `golang` version 1.21. (https://go.dev/doc/install)
* Requires `C` compiler, recent versions of [GCC](https://gcc.gnu.org/) are recommended. In Debian and Ubuntu repositories, GCC is part of the build-essential package. On macOS, GCC can be installed with [Homebrew](https://formulae.brew.sh/formula/gcc).

Run `make build` to build the application. Executable will be built to `build/abwallet`. 

## Configuration

It's possible to define the configuration values from (in the order of precedence):

* Command line flags (e.g. `--address="/ip4/127.0.0.1/tcp/26652"`)
* Environment (Prefix 'AB' must be used. E.g. `AB_ADDRESS="/ip4/127.0.0.1/tcp/26652"`)
* Configuration file (properties file) (E.g. `address="/ip4/127.0.0.1/tcp/26652"`)
* Default values

The default location of configuration file is `$AB_HOME/config.props`

The default `$AB_HOME` is `$HOME/.alphabill`

## Integration tests

Integration tests use Alphabill docker image to set up the test environment in containers,
and thus Docker must be installed to run those tests. It is possible to skip such tests with
the `nodocker` build tag:

```sh
go test ./... --tags=nodocker
```

Alphabill docker image used in tests can be configured with `AB_TEST_DOCKERIMAGE`
environment variable:

```sh
AB_TEST_DOCKERIMAGE=ghcr.io/alphabill-org/alphabill:cf4ff7151d7a7ebba65903b7d827b0740fc878a4 go test ./...
```

# CI setup

See gitlab-ci.yml for details.

GitLab runs the CI job inside docker container defined in `alphabill/gitlab-ci-image`.
