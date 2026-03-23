# Runs the built server binary from server\bin (no npm, no go run).
# Working directory is bin so config.json, plugins/, and frontend/dist resolve like production.
$ErrorActionPreference = "Stop"
$RootDir = $PSScriptRoot
$BinDir  = Join-Path $RootDir "bin"
$ext     = if ($IsLinux -or $IsMacOS) { "" } else { ".exe" }
$server  = Join-Path $BinDir "mga_server$ext"

if (-not (Test-Path $server)) {
    Write-Error "Missing $server — run build.ps1 first."
    exit 1
}

Set-Location $BinDir
Write-Host "Running $server" -ForegroundColor Cyan
& $server @args
exit $LASTEXITCODE
