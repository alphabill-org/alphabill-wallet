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

## Logging configuration

Logging can be configured through a yaml configuration file. See `cli/alphabill/config/logger-config.yaml` for example.

Default location of the logger configuration file is `$AB_HOME/logger-config.yaml`

The location can be changed through `--logger-config` configuration key. If it's relative URL, then it's relative
to `$AB_HOME`. Some logging related parameters can be set via command line parameters too - run `alphabill -h`
for more.

## Tracing tests

To enable trace exporter for test the `AB_TEST_TRACER` environment variable has to be set
to desired exporter name, ie

```sh
AB_TEST_TRACER=otlptracehttp go test ./...
```

The test tracing will pick up the same OTEL environment variables linked above except that
some parameters are already "hardcoded":

- "always_on" sampler is used (`OTEL_TRACES_SAMPLER`);
- the `otlptracehttp` exporter is created with "insecure client transport"
  (`OTEL_EXPORTER_OTLP_INSECURE`);

## Tracing wallet commands

It is possible to collect traces from wallet command by setting the `AB_TRACING` and `OTEL_EXPORTER_OTLP_ENDPOINT`
environment variables, ie:

```sh
AB_TRACING=otlptracehttp OTEL_EXPORTER_OTLP_ENDPOINT=https://apmserver.abdev1.guardtime.com alphabill wallet ...
```
will send the traces into the devnet APM backend.


# CI setup

See gitlab-ci.yml for details.

GitLab runs the CI job inside docker container defined in `alphabill/gitlab-ci-image`.
