param(
    [string]$Version = "",
    [string]$Commit = "",
    [string]$BuildDate = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $Version) {
    $Version = (Get-Content -LiteralPath (Join-Path $root "VERSION") -Raw).Trim()
}
if (-not $Commit) {
    $Commit = (git -C $root rev-parse --short HEAD).Trim()
}
if (-not $BuildDate) {
    $BuildDate = [DateTime]::UtcNow.ToString("o")
}
if (-not $Version -or -not $Commit -or -not $BuildDate) {
    throw "Version, commit, and build date are required."
}

$bin = Join-Path $root "bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
$output = Join-Path $bin "mga-client.exe"
$agentOutput = Join-Path $bin "mga-client-agent.exe"
$ldflags = "-s -w -X github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo.Version=$Version -X github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo.Commit=$Commit -X github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo.BuildDate=$BuildDate"
$agentLdflags = "-H=windowsgui $ldflags"

Push-Location $root
try {
    go build -trimpath -ldflags $ldflags -o $output ./cmd/mga-client
    if ($LASTEXITCODE -ne 0) { throw "MGA Client build failed." }
    go build -trimpath -ldflags $agentLdflags -o $agentOutput ./cmd/mga-client
    if ($LASTEXITCODE -ne 0) { throw "MGA Client agent build failed." }
} finally {
    Pop-Location
}

Write-Output $output
