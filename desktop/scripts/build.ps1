#!/usr/bin/env pwsh
<#
.SYNOPSIS
    Builds and optionally runs the MGA Desktop client.

.PARAMETER Configuration
    Build configuration: Debug (default) or Release.

.PARAMETER Run
    If specified, launches the application after a successful build.

.PARAMETER Publish
    If specified, publishes a self-contained single-file executable to ./publish/.

.EXAMPLE
    .\scripts\build.ps1
    .\scripts\build.ps1 -Configuration Release -Publish
    .\scripts\build.ps1 -Run
#>
param(
    [ValidateSet('Debug','Release')]
    [string]$Configuration = 'Debug',

    [switch]$Run,
    [switch]$Publish
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$SlnPath = Join-Path $PSScriptRoot "..\MGA.Desktop.sln"

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

Write-Host "Building $Configuration…" -ForegroundColor Cyan
dotnet build $SlnPath -c $Configuration --nologo
if ($LASTEXITCODE -ne 0) { throw "Build failed." }

# ---------------------------------------------------------------------------
# Publish (optional)
# ---------------------------------------------------------------------------

if ($Publish) {
    $PublishDir = Join-Path $PSScriptRoot "..\publish"
    Write-Host "Publishing to $PublishDir …" -ForegroundColor Cyan
    dotnet publish "$PSScriptRoot\..\src\MGA.Desktop\MGA.Desktop.csproj" `
        -c Release `
        --self-contained true `
        -r win-x64 `
        -p:PublishSingleFile=true `
        -p:PublishTrimmed=false `
        -o $PublishDir `
        --nologo
    if ($LASTEXITCODE -ne 0) { throw "Publish failed." }
    Write-Host "Published to: $PublishDir" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Run (optional)
# ---------------------------------------------------------------------------

if ($Run) {
    Write-Host "Launching…" -ForegroundColor Cyan
    dotnet run --project "$PSScriptRoot\..\src\MGA.Desktop\MGA.Desktop.csproj" `
        -c $Configuration `
        --no-build
}

Write-Host "Done." -ForegroundColor Green
