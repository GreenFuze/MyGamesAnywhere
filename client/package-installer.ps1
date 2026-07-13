param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $Version) {
    $Version = (Get-Content -LiteralPath (Join-Path $root "VERSION") -Raw).Trim()
}
if (-not $Version) { throw "Client version is required." }

$clientExe = & (Join-Path $root "build.ps1") -Version $Version
$iscc = Get-Command ISCC.exe -ErrorAction SilentlyContinue
if ($iscc) {
    $isccPath = $iscc.Source
} else {
    $isccPath = @(
        (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe"),
        (Join-Path ${env:ProgramFiles(x86)} "Inno Setup 6\ISCC.exe"),
        (Join-Path $env:ProgramFiles "Inno Setup 6\ISCC.exe")
    ) | Where-Object { $_ -and (Test-Path -LiteralPath $_) } | Select-Object -First 1
}
if (-not $isccPath) {
    throw "Inno Setup compiler (ISCC.exe) was not found in PATH or a standard install location."
}
$release = Join-Path $root "release"
New-Item -ItemType Directory -Force -Path $release | Out-Null
$script = Join-Path $root "installer\mga-client.iss"
& $isccPath "/DAppVersion=$Version" "/DClientExe=$clientExe" "/DOutputDir=$release" $script
if ($LASTEXITCODE -ne 0) { throw "MGA Client installer build failed." }
