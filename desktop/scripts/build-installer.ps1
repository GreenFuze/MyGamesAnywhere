# Build the MGA Desktop installer using Inno Setup.
# Usage: .\build-installer.ps1 [-Configuration Release] [-Runtime win-x64]
#
# Prerequisites:
#   - Inno Setup 6.x installed (iscc.exe on PATH or at the default location)
#   - .NET 9 SDK

param(
    [string]$Configuration = "Release",
    [string]$Runtime       = "win-x64",
    [string]$Version       = ""      # If empty, reads from AssemblyInformationalVersion
)

$ErrorActionPreference = "Stop"
$root      = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$desktop   = Join-Path $root "desktop"
$projFile  = Join-Path $desktop "src\MGA.Desktop\MGA.Desktop.csproj"
$publishDir = Join-Path $desktop "src\MGA.Desktop\bin\$Configuration\net9.0\$Runtime\publish"
$issFile   = Join-Path $desktop "installer\MGA-Setup.iss"

# ── Step 1: dotnet publish ────────────────────────────────────────────────
Write-Host "Publishing MGA.Desktop ($Configuration / $Runtime)..." -ForegroundColor Cyan
dotnet publish $projFile `
    --configuration $Configuration `
    --runtime $Runtime `
    --self-contained false `
    --output $publishDir

if ($LASTEXITCODE -ne 0) { throw "dotnet publish failed" }

# ── Step 2: Read version from assembly if not provided ────────────────────
if (-not $Version) {
    $exePath = Join-Path $publishDir "MGA.Desktop.exe"
    if (Test-Path $exePath) {
        $Version = [System.Diagnostics.FileVersionInfo]::GetVersionInfo($exePath).FileVersion
    }
    if (-not $Version) { $Version = "0.0.0" }
}
Write-Host "Version: $Version" -ForegroundColor Green

# ── Step 3: Find iscc.exe ─────────────────────────────────────────────────
$iscc = Get-Command iscc.exe -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source
if (-not $iscc) {
    $default = "C:\Program Files (x86)\Inno Setup 6\iscc.exe"
    if (Test-Path $default) {
        $iscc = $default
    } else {
        throw "iscc.exe not found. Install Inno Setup 6 from https://jrsoftware.org/isinfo.php"
    }
}

# ── Step 4: Run Inno Setup ────────────────────────────────────────────────
Write-Host "Building installer with Inno Setup..." -ForegroundColor Cyan
& $iscc $issFile `
    /dAppVersion=$Version `
    /dSourceRoot=$publishDir

if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed" }

$outputDir = Join-Path $desktop "installer\Output"
Write-Host "Installer built: $outputDir\MGA-Setup-$Version.exe" -ForegroundColor Green
