$ErrorActionPreference = "Stop"

$backendRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$workspaceRoot = Split-Path -Parent $backendRoot
$goBinDir = Join-Path $workspaceRoot ".tools\go\bin"
$toolBinDir = Join-Path $workspaceRoot ".tools\bin"
$airExe = Join-Path $toolBinDir "air.exe"
$goCacheDir = Join-Path $workspaceRoot ".cache\go-build"

if (-not (Test-Path $toolBinDir)) {
  New-Item -ItemType Directory -Path $toolBinDir | Out-Null
}

if (-not (Test-Path $goCacheDir)) {
  New-Item -ItemType Directory -Path $goCacheDir | Out-Null
}

$env:PATH = "$toolBinDir;$goBinDir;$env:PATH"
$env:GOBIN = $toolBinDir
$env:GOCACHE = $goCacheDir

if (-not (Test-Path $airExe)) {
  Write-Host "Air not found in workspace, installing to $airExe ..."
  & (Join-Path $goBinDir "go.exe") install github.com/air-verse/air@latest
}

Push-Location $backendRoot
try {
  & $airExe -c ".air.toml"
}
finally {
  Pop-Location
}
