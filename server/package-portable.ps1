param(
    [string]$Version,
    [string]$OutputDir = (Join-Path $PSScriptRoot "release"),
    [switch]$SkipBuild,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"

function Require-WindowsAmd64 {
    if ($env:OS -ne "Windows_NT") {
        throw "Portable packaging is currently supported only on Windows."
    }

    $arch = if ($env:PROCESSOR_ARCHITEW6432) {
        $env:PROCESSOR_ARCHITEW6432
    } else {
        $env:PROCESSOR_ARCHITECTURE
    }

    if ($arch -ne "AMD64") {
        throw "Portable packaging currently supports only Windows amd64 hosts. Current architecture: $arch"
    }
}

function Resolve-Version {
    param([string]$RootDir, [string]$ExplicitVersion)

    if ($ExplicitVersion) {
        return $ExplicitVersion.TrimStart("v")
    }

    $versionFile = Join-Path (Split-Path $RootDir -Parent) "VERSION"
    if (-not (Test-Path $versionFile)) {
        throw "Missing VERSION file at $versionFile"
    }

    return (Get-Content $versionFile -Raw).Trim()
}

Require-WindowsAmd64

$rootDir = $PSScriptRoot
$binDir = Join-Path $rootDir "bin"
$packageTemplates = Join-Path $rootDir "packaging\windows"
$configTemplate = Join-Path $rootDir "config.json.example"
$resolvedVersion = Resolve-Version -RootDir $rootDir -ExplicitVersion $Version

if ($resolvedVersion -notmatch '^\d+\.\d+\.\d+$') {
    throw "VERSION must be in X.Y.Z format. Got '$resolvedVersion'."
}

$artifactStem = "mga-v$resolvedVersion-windows-amd64"
$artifactName = "$artifactStem-portable.zip"
$stageDir = Join-Path $OutputDir $artifactStem
$zipPath = Join-Path $OutputDir $artifactName
$checksumPath = Join-Path $OutputDir "SHA256SUMS.txt"

if (-not $SkipBuild) {
    $buildArgs = @("-FrontendInstallMode", "Clean")
    if ($SkipFrontend) {
        $buildArgs += "-SkipFrontend"
    }
    & (Join-Path $rootDir "build.ps1") @buildArgs
    if ($LASTEXITCODE -ne 0) {
        throw "build.ps1 failed with exit code $LASTEXITCODE"
    }
}

$requiredPaths = @(
    (Join-Path $binDir "mga_server.exe"),
    $configTemplate,
    (Join-Path $binDir "plugins"),
    (Join-Path $binDir "frontend\dist\index.html"),
    (Join-Path $packageTemplates "Start MGA.cmd"),
    (Join-Path $packageTemplates "Start MGA.ps1"),
    (Join-Path $packageTemplates "verify-mga.ps1")
)

foreach ($path in $requiredPaths) {
    if (-not (Test-Path $path)) {
        throw "Required packaging input is missing: $path"
    }
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
if (Test-Path $stageDir) {
    Remove-Item $stageDir -Recurse -Force
}
if (Test-Path $zipPath) {
    Remove-Item $zipPath -Force
}

New-Item -ItemType Directory -Force -Path $stageDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $stageDir "data") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $stageDir "media") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $stageDir "frontend") | Out-Null

Copy-Item (Join-Path $binDir "mga_server.exe") -Destination $stageDir -Force
Copy-Item $configTemplate -Destination (Join-Path $stageDir "config.json") -Force
Copy-Item (Join-Path $binDir "plugins") -Destination $stageDir -Recurse -Force
Copy-Item (Join-Path $binDir "frontend\dist") -Destination (Join-Path $stageDir "frontend") -Recurse -Force
Copy-Item (Join-Path $packageTemplates "*") -Destination $stageDir -Force

Get-ChildItem (Join-Path $stageDir "plugins") -Recurse -File | Where-Object {
    $_.Name -in @("config.json", "tokens.json")
} | ForEach-Object {
    Remove-Item $_.FullName -Force
}

Compress-Archive -Path $stageDir -DestinationPath $zipPath -CompressionLevel Optimal -Force

$hash = (Get-FileHash -Algorithm SHA256 $zipPath).Hash.ToLowerInvariant()
Set-Content -Path $checksumPath -Value ("{0} *{1}" -f $hash, (Split-Path $zipPath -Leaf)) -NoNewline

Write-Host ""
Write-Host "Portable package created:" -ForegroundColor Green
Write-Host "  ZIP:      $zipPath"
Write-Host "  SHA256:   $checksumPath"
