# Development Container Workflow

This repository uses one Dev Container configuration for local development.
It is intended to keep setup simple while still matching CI toolchains.

## What it uses

- Configuration file: `.devcontainer/devcontainer.json`
- Build source: top-level `Dockerfile`
- Build target: `devbase`

The `devbase` stage contains build dependencies for Go and UI work, without running the full app build during image creation.

## Open the project in container

1. Open this repository in VS Code.
2. Run Command Palette:
   - Dev Containers: Reopen in Container
3. Wait for initial setup to finish.

On first create, VS Code runs:

- `postCreateCommand`: installs UI dependencies

On each container start, VS Code runs:

- `postStartCommand`: runs a UI build smoke check

## Build inside container

From repository root:

- Build UI only:
  - `make frontend`
- Build backend binary only:
  - `make homer-only`
- Build full release flow (UI + backend):
  - `make all`

## Run tests inside container

Backend tests:

- `cd src && go test ./...`

UI tests:

- `cd src/ui && npm run test`

Targeted UI tests for call flow work:

- `cd src/ui && npm run test -- flow-data.test.ts FlowItem.test.tsx`

## Common troubleshooting

If dependencies are missing or hooks were interrupted:

- `cd src/ui && npm ci`

If the container needs a full rebuild after Dockerfile or devcontainer changes:

1. Command Palette:
   - Dev Containers: Rebuild and Reopen in Container

If build tools are unexpectedly missing, verify you are inside the container terminal:

- `go version`
- `node --version`
- `npm --version`
