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
    # Launch the built .exe directly — NOT via "dotnet run".
    # dotnet.exe is a console-subsystem process, so using "dotnet run" always
    # opens a terminal window even though the app is WinExe (no console subsystem).
    # Launching the .exe directly respects the WinExe subsystem and shows no terminal.
    $ExePath = Join-Path $PSScriptRoot "..\src\MGA.Desktop\bin\$Configuration\net9.0\MGA.Desktop.exe"
    if (-not (Test-Path $ExePath)) {
        throw "Executable not found at '$ExePath'. Build may have failed."
    }
    Write-Host "Launching $ExePath …" -ForegroundColor Cyan
    Start-Process -FilePath $ExePath
}

Write-Host "Done." -ForegroundColor Green
