# MAM Backend

Go service skeleton for the MVP backend.

## Scope

- HTTP entrypoint
- Health and readiness endpoints
- Bootstrap endpoint for frontend/admin consumers
- SQLite-backed local persistence for B1/B2/B3/B4 domain tables
- Config model for PostgreSQL, Redis, OpenSearch, RabbitMQ, MinIO, FFmpeg, and AI service
- Module registry for asset, storage, sync, import, search, task, and media worker services

## Persistence

The current backend skeleton uses a local SQLite catalog to make B1/B2/B3/B4 executable and testable without requiring PostgreSQL during bootstrap. The schema maps directly to the planned transactional domain:

- `storage_endpoints`
- `assets`
- `replicas`
- `replica_versions`
- `tasks`

PostgreSQL remains the target transactional database for the full service deployment.

## Run

Use the local Go SDK in the workspace:

```powershell
$env:HTTP_PROXY='http://127.0.0.1:7890'
$env:HTTPS_PROXY='http://127.0.0.1:7890'
$env:ALL_PROXY='socks5://127.0.0.1:7890'
.\.tools\go\bin\go.exe run .\backend\cmd\server
```

## Hot Reload

Air has been wired into the workspace for local backend development.

Start it from the repo root:

```powershell
cmd /c npm.cmd run backend:dev
```

Or run the backend wrapper directly:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\backend\dev-air.ps1
```

The wrapper:

- uses the workspace Go toolchain in `.tools\go\bin`
- uses the workspace-local Air binary in `.tools\bin\air.exe`
- keeps Go build cache inside `.cache\go-build`
- watches Go source under `backend\` and rebuilds `cmd\server`
- restarts the backend process automatically after changes

Air configuration lives in `backend/.air.toml`.

## Validate

```powershell
.\.tools\go\bin\go.exe test .\backend\...
```
