param(
    [string]$Version,
    [string]$OutputDir = (Join-Path $PSScriptRoot "release"),
    [string]$ReleaseBaseUrl = "https://github.com/GreenFuze/MyGamesAnywhere/releases/latest/download",
    [string]$ISCCPath,
    [switch]$SkipBuild,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"

function Require-WindowsAmd64 {
    if ($env:OS -ne "Windows_NT") {
        throw "Installer packaging is currently supported only on Windows."
    }
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    if ($arch -ne "AMD64") {
        throw "Installer packaging currently supports only Windows amd64 hosts. Current architecture: $arch"
    }
}

function Resolve-Version {
    param([string]$RootDir, [string]$ExplicitVersion)
    if ($ExplicitVersion) { return $ExplicitVersion.TrimStart("v") }
    $versionFile = Join-Path (Split-Path $RootDir -Parent) "VERSION"
    if (-not (Test-Path $versionFile)) {
        throw "Missing VERSION file at $versionFile"
    }
    return (Get-Content $versionFile -Raw).Trim().TrimStart("v")
}

function Resolve-ISCC {
    param([string]$Explicit)
    if ($Explicit) {
        if (-not (Test-Path $Explicit)) { throw "ISCC.exe not found: $Explicit" }
        return $Explicit
    }
    $cmd = Get-Command "iscc.exe" -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    foreach ($candidate in @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
    )) {
        if ($candidate -and (Test-Path $candidate)) { return $candidate }
    }
    throw "Inno Setup compiler (ISCC.exe) was not found. Install Inno Setup 6 or pass -ISCCPath."
}

function Get-FileHashEntry {
    param([string]$Path)
    $file = Get-Item $Path
    [ordered]@{
        name = $file.Name
        sha256 = (Get-FileHash -Algorithm SHA256 $file.FullName).Hash.ToLowerInvariant()
        size = $file.Length
    }
}

Require-WindowsAmd64

$rootDir = $PSScriptRoot
$binDir = Join-Path $rootDir "bin"
$stageDir = Join-Path $OutputDir "installer-stage"
$issPath = Join-Path $rootDir "packaging\windows\mga.iss"
$resolvedVersion = Resolve-Version -RootDir $rootDir -ExplicitVersion $Version

if ($resolvedVersion -notmatch '^\d+\.\d+\.\d+$') {
    throw "VERSION must be in X.Y.Z format. Got '$resolvedVersion'."
}

if (-not $SkipBuild) {
    $buildArgs = @{ FrontendInstallMode = "Clean" }
    if ($SkipFrontend) { $buildArgs.SkipFrontend = $true }
    & (Join-Path $rootDir "build.ps1") @buildArgs
    if ($LASTEXITCODE -ne 0) { throw "build.ps1 failed with exit code $LASTEXITCODE" }
}

$required = @(
    (Join-Path $binDir "mga_server.exe"),
    (Join-Path $binDir "mga_tray.exe"),
    (Join-Path $binDir "mga.ico"),
    (Join-Path $binDir "plugins"),
    (Join-Path $binDir "frontend\dist\index.html"),
    $issPath,
    (Join-Path $rootDir "packaging\windows\install-config.ps1"),
    (Join-Path $rootDir "packaging\windows\service.ps1"),
    (Join-Path $rootDir "packaging\windows\firewall.ps1")
)
foreach ($path in $required) {
    if (-not (Test-Path $path)) { throw "Required installer input is missing: $path" }
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
if (Test-Path $stageDir) { Remove-Item $stageDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $stageDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $stageDir "frontend") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $stageDir "packaging\windows") | Out-Null

Copy-Item (Join-Path $binDir "mga_server.exe") -Destination $stageDir -Force
Copy-Item (Join-Path $binDir "mga_tray.exe") -Destination $stageDir -Force
Copy-Item (Join-Path $binDir "mga.ico") -Destination $stageDir -Force
Copy-Item (Join-Path $binDir "config.json") -Destination $stageDir -Force -ErrorAction SilentlyContinue
Copy-Item (Join-Path $binDir "plugins") -Destination $stageDir -Recurse -Force
Copy-Item (Join-Path $binDir "frontend\dist") -Destination (Join-Path $stageDir "frontend") -Recurse -Force
Copy-Item (Join-Path $rootDir "packaging\windows\*") -Destination (Join-Path $stageDir "packaging\windows") -Force

foreach ($doc in @("LICENSE.md", "NOTICE", "README.md")) {
    $src = Join-Path (Split-Path $rootDir -Parent) $doc
    if (Test-Path $src) { Copy-Item $src -Destination $stageDir -Force }
}

Get-ChildItem (Join-Path $stageDir "plugins") -Recurse -File | Where-Object {
    $_.Name -in @("config.json", "tokens.json")
} | ForEach-Object {
    Remove-Item $_.FullName -Force
}

$iscc = Resolve-ISCC -Explicit $ISCCPath
& $iscc $issPath "/DMyAppVersion=$resolvedVersion" "/DSourceDir=$stageDir" "/DOutputDir=$OutputDir"
if ($LASTEXITCODE -ne 0) { throw "Inno Setup compiler failed with exit code $LASTEXITCODE" }

$installerPath = Join-Path $OutputDir "mga-v$resolvedVersion-windows-amd64-installer.exe"
if (-not (Test-Path $installerPath)) {
    throw "Installer build did not produce expected output: $installerPath"
}

$installerHash = Get-FileHashEntry -Path $installerPath
$portableName = "mga-v$resolvedVersion-windows-amd64-portable.zip"
$portablePath = Join-Path $OutputDir $portableName
$assets = @(
    [ordered]@{
        os = "windows"
        arch = "amd64"
        type = "installer"
        name = $installerHash.name
        url = "$ReleaseBaseUrl/$($installerHash.name)"
        sha256 = $installerHash.sha256
        size = $installerHash.size
    }
)
if (Test-Path $portablePath) {
    $portableHash = Get-FileHashEntry -Path $portablePath
    $assets += [ordered]@{
        os = "windows"
        arch = "amd64"
        type = "portable"
        name = $portableHash.name
        url = "$ReleaseBaseUrl/$($portableHash.name)"
        sha256 = $portableHash.sha256
        size = $portableHash.size
    }
}

$manifest = [ordered]@{
    version = $resolvedVersion
    release_notes_url = "https://github.com/GreenFuze/MyGamesAnywhere/releases/tag/v$resolvedVersion"
    minimum_supported_updater_version = "1"
    assets = $assets
}
$manifestPath = Join-Path $OutputDir "mga-update.json"
$manifest | ConvertTo-Json -Depth 8 | Set-Content -Path $manifestPath -Encoding UTF8

$sumLines = @()
foreach ($asset in $assets) {
    $sumLines += ("{0} *{1}" -f $asset.sha256, $asset.name)
}
$manifestHash = Get-FileHashEntry -Path $manifestPath
$sumLines += ("{0} *{1}" -f $manifestHash.sha256, $manifestHash.name)
Set-Content -Path (Join-Path $OutputDir "SHA256SUMS.txt") -Value ($sumLines -join [Environment]::NewLine) -Encoding ASCII

Write-Host ""
Write-Host "Installer package created:" -ForegroundColor Green
Write-Host "  EXE:      $installerPath"
Write-Host "  Manifest: $manifestPath"
