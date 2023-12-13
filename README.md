## Alphabill Wallet

Client side software to interface with Alphabill network. This repository contains the following components:
* Money partition indexing backend
* User Token partition indexing backend
* Money partition CLI wallet
* User Token partition CLI wallet
* EVM partition client

## Building the source

Requires `golang` version 1.21. (https://go.dev/doc/install)

Run `make build` to build the application. Executable will be built to `build/abwallet`. 

## Start the indexing backends

Requires Alphabill network to be operational. (https://github.com/alphabill-org/alphabill)

The startup scripts expect `alphabill` directory is sibling to `alphabill-wallet` directory 
to load system description record files.

1. `./start.sh -b money` to start money backend
2. `./start.sh -b tokens` to start tokens backend
3. `./start.sh -b money -b tokens` to start both money and tokens backends 

NB! EVM partition does not have an indexing backend.

`./stop.sh -a` to stop all backends.

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
AB_TEST_TRACER=otlptracegrpc go test ./...
```

The test tracing will pick up the same OTEL environment variables linked above except that
some parameters are already "hardcoded":

- "always_on" sampler is used (`OTEL_TRACES_SAMPLER`);
- the `otlptracehttp` and `otlptracegrpc` exporters are created with "insecure client transport"
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
