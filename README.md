# box-dispatch

Box Dispatch is a browser-first workspace for configuring, validating, and deploying
Box-backed solution stacks. A single Go executable serves the embedded React application
and its loopback API; Go owns credentials, provider calls, package assembly, deployment,
and audit records.

The browser guides operators through choosing a solution, connecting Box and Salesforce,
configuring the deployment, validating provider state, applying changes, and reviewing
deployment history.

## Prerequisites

- Go 1.26
- Bun, when rebuilding or developing the React application
- Provider credentials for live Box or Salesforce operations

## Configure

Copy the sample environment file and add the provider values needed for the systems you
will use:

```bash
cp .env.sample .env
```

Real `.env` files and tokens are gitignored and must never be committed. Box browser login
uses callback port `4400`; Salesforce browser login uses callback port `1717`. Dispatch
reports a remediation message when either port is unavailable.

Connection selections and workspace defaults are saved locally as owner-only BCL documents
under `.dispatch/`.

## Run

Start the complete embedded application:

```bash
go run ./cmd/box-dispatch
```

Dispatch listens on `127.0.0.1:8787` and opens the browser. To keep the browser closed or
choose another loopback port:

```bash
go run ./cmd/box-dispatch --no-open
go run ./cmd/box-dispatch --port 8790
```

## Build

Build the React application and Go executable together:

```bash
make build
./box-dispatch
```

The executable contains the compiled browser UI and starts both the UI and local API.

## Documentation

- [Development and verification](docs/DEVELOPMENT.md)
- [Command reference](docs/COMMAND_REFERENCE.md)
- [Web application architecture](docs/WEB_APP_ARCHITECTURE.md)
- [BCL artifact contract](BCL_ARTIFACT_CONTRACT.md)
- [Public API capability gaps](docs/PUBLIC_API_GAPS.md)
- [Roadmap](docs/ROADMAP.md)
