[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Version,

    [switch]$Inc
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-Native {
    param(
        [Parameter(Mandatory = $true)][string]$Command,
        # Named so short native flags like `git tag -a` are not bound as -Arguments.
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$RemainingArguments
    )

    & $Command @RemainingArguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: $Command $($RemainingArguments -join ' ')"
    }
}

function Invoke-InDirectory {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][scriptblock]$Action
    )

    Push-Location $Path
    try {
        & $Action
    } finally {
        Pop-Location
    }
}

function Assert-CleanWorktree {
    param([Parameter(Mandatory = $true)][string]$Message)

    $changes = @(git status --porcelain=v1)
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to inspect the Git worktree."
    }
    if ($changes.Count -gt 0) {
        throw "$Message`n$($changes -join [Environment]::NewLine)"
    }
}

function Get-LatestStableVersion {
    $stableVersions = @(
        git tag --list "v*" |
            ForEach-Object {
                if ($_ -match '^v(\d+)\.(\d+)\.(\d+)$') {
                    [version]::new([int]$Matches[1], [int]$Matches[2], [int]$Matches[3])
                }
            }
    )
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to list Git tags."
    }
    if ($stableVersions.Count -eq 0) {
        return $null
    }
    return $stableVersions | Sort-Object -Descending | Select-Object -First 1
}

function Resolve-ReleaseVersion {
    param(
        [string]$ExplicitVersion,
        [bool]$Increment
    )

    if ($Increment) {
        $latest = Get-LatestStableVersion
        if ($null -eq $latest) {
            throw "--inc requires at least one stable vX.Y.Z Git tag."
        }
        return "{0}.{1}.{2}" -f $latest.Major, $latest.Minor, ($latest.Build + 1)
    }

    $resolved = $ExplicitVersion.Trim().TrimStart('v')
    if ($resolved -notmatch '^\d+\.\d+\.\d+$') {
        throw "Version must be a stable SemVer value such as 0.2.4 or v0.2.4."
    }
    return $resolved
}

function Get-HashLine {
    param([Parameter(Mandatory = $true)][string]$Path)

    $file = Get-Item -LiteralPath $Path
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    return "$hash *$($file.Name)"
}

if ($Version -eq '--inc') {
    if ($Inc) {
        throw "Use either --inc or -Inc, not both."
    }
    $Inc = $true
    $Version = ""
}

if ($Inc -and $Version) {
    throw "Provide a version or --inc, not both."
}
if (-not $Inc -and -not $Version) {
    throw "Provide a version (for example 0.2.4) or use --inc."
}

$repoRoot = $PSScriptRoot
$versionFile = Join-Path $repoRoot "VERSION"
$serverDir = Join-Path $repoRoot "server"
$clientDir = Join-Path $repoRoot "client"
$protocolDir = Join-Path $repoRoot "protocol"
$serverReleaseDir = Join-Path $serverDir "release"
$clientReleaseDir = Join-Path $clientDir "release"

foreach ($tool in @('git', 'gh', 'go', 'npm')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "Required command is unavailable: $tool"
    }
}
if ($env:OS -ne 'Windows_NT') {
    throw "MGA release packaging currently requires Windows."
}
if (-not (Test-Path -LiteralPath $versionFile)) {
    throw "Missing repository VERSION file: $versionFile"
}

$runningBuildProcesses = @(
    Get-CimInstance Win32_Process |
        Where-Object {
            $_.ExecutablePath -in @(
                (Join-Path $serverDir 'bin\mga_server.exe'),
                (Join-Path $serverDir 'bin\mga_tray.exe')
            )
        }
)
if ($runningBuildProcesses.Count -gt 0) {
    $runningSummary = $runningBuildProcesses | ForEach-Object { "$($_.Name) (PID $($_.ProcessId))" }
    throw "Stop the repository's packaged MGA server/tray before publishing so Windows can replace the binaries: $($runningSummary -join ', ')"
}

Invoke-InDirectory $repoRoot {
    Invoke-Native gh auth status
    Assert-CleanWorktree "Commit or discard every change before publishing a release."
    Invoke-Native git fetch origin --tags --prune

    $defaultBranch = (gh repo view --json defaultBranchRef --jq '.defaultBranchRef.name').Trim()
    if ($LASTEXITCODE -ne 0 -or -not $defaultBranch) {
        throw "Unable to determine the GitHub default branch."
    }
    if ($defaultBranch -notin @('main', 'master')) {
        throw "The GitHub default branch must be main or master. Found '$defaultBranch'."
    }

    $currentBranch = (git branch --show-current).Trim()
    if ($LASTEXITCODE -ne 0 -or $currentBranch -ne $defaultBranch) {
        throw "Release publishing must run from '$defaultBranch'. Current branch: '$currentBranch'."
    }

    $localHead = (git rev-parse HEAD).Trim()
    $remoteHead = (git rev-parse "origin/$defaultBranch").Trim()
    if ($localHead -ne $remoteHead) {
        throw "Local $defaultBranch must exactly match origin/$defaultBranch before release preparation. Push or reconcile it first."
    }

    $resolvedVersion = Resolve-ReleaseVersion -ExplicitVersion $Version -Increment ([bool]$Inc)
    $tag = "v$resolvedVersion"
    $latestStable = Get-LatestStableVersion
    if ($null -ne $latestStable -and [version]$resolvedVersion -le $latestStable) {
        throw "$tag must be newer than the latest stable tag v$latestStable."
    }

    git show-ref --verify --quiet "refs/tags/$tag"
    if ($LASTEXITCODE -eq 0) {
        throw "Git tag already exists: $tag"
    }
    if ($LASTEXITCODE -ne 1) {
        throw "Unable to check local tag $tag."
    }
    $remoteTag = @(git ls-remote --tags origin "refs/tags/$tag")
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to check remote tag $tag."
    }
    if ($remoteTag.Count -gt 0) {
        throw "Remote Git tag already exists: $tag"
    }

    gh release view $tag *> $null
    if ($LASTEXITCODE -eq 0) {
        throw "GitHub release already exists: $tag"
    }

    $currentVersion = (Get-Content -LiteralPath $versionFile -Raw).Trim().TrimStart('v')
    if ($currentVersion -ne $resolvedVersion) {
        $encoding = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($versionFile, "$resolvedVersion$([Environment]::NewLine)", $encoding)
        Invoke-Native git add -- VERSION
        Invoke-Native git commit -m "chore: prepare $tag release"
    } else {
        Write-Host "VERSION already contains $resolvedVersion; no version commit is needed." -ForegroundColor DarkGray
    }

    Write-Host "Running release verification for $tag..." -ForegroundColor Cyan
    & (Join-Path $serverDir "scripts\check-migration-guard.ps1")
    if ($LASTEXITCODE -ne 0) {
        throw "Migration guard failed with exit code $LASTEXITCODE."
    }

    Invoke-InDirectory $protocolDir { Invoke-Native go test ./... }
    Invoke-InDirectory $clientDir { Invoke-Native go test ./... }
    Invoke-InDirectory $serverDir { Invoke-Native go test ./... }

    $standaloneModules = Get-ChildItem -Path (Join-Path $serverDir 'plugins') -Filter go.mod -Recurse -File
    foreach ($module in $standaloneModules) {
        Invoke-InDirectory $module.Directory.FullName { Invoke-Native go test ./... }
    }

    Invoke-InDirectory (Join-Path $serverDir 'frontend') {
        Invoke-Native npm run generate:api-contracts
        Invoke-Native npm run test:unit
    }
    Assert-CleanWorktree "Verification changed tracked files. Review and commit the generated changes before releasing."

    Write-Host "Building release artifacts for $tag..." -ForegroundColor Cyan
    & (Join-Path $clientDir 'package-installer.ps1') -Version $resolvedVersion
    if ($LASTEXITCODE -ne 0) { throw "MGA Client packaging failed with exit code $LASTEXITCODE." }

    & (Join-Path $serverDir 'package-portable.ps1') -Version $resolvedVersion
    if ($LASTEXITCODE -ne 0) { throw "Portable server packaging failed with exit code $LASTEXITCODE." }

    $releaseBaseUrl = "https://github.com/GreenFuze/MyGamesAnywhere/releases/download/$tag"
    & (Join-Path $serverDir 'package-installer.ps1') -Version $resolvedVersion -SkipBuild -ReleaseBaseUrl $releaseBaseUrl
    if ($LASTEXITCODE -ne 0) { throw "Server installer packaging failed with exit code $LASTEXITCODE." }

    $assets = @(
        (Join-Path $serverReleaseDir "mga-v$resolvedVersion-windows-amd64-installer.exe"),
        (Join-Path $serverReleaseDir "mga-v$resolvedVersion-windows-amd64-portable.zip"),
        (Join-Path $clientReleaseDir 'mga-client-windows-amd64-installer.exe'),
        (Join-Path $serverReleaseDir 'mga-update.json')
    )
    foreach ($asset in $assets) {
        if (-not (Test-Path -LiteralPath $asset)) {
            throw "Expected release asset is missing: $asset"
        }
    }

    $checksumPath = Join-Path $serverReleaseDir 'SHA256SUMS.txt'
    $checksumLines = $assets | ForEach-Object { Get-HashLine -Path $_ }
    Set-Content -LiteralPath $checksumPath -Value $checksumLines -Encoding ASCII
    $assets += $checksumPath

    $manifest = Get-Content -LiteralPath (Join-Path $serverReleaseDir 'mga-update.json') -Raw | ConvertFrom-Json
    if ($manifest.version -ne $resolvedVersion) {
        throw "Update manifest version '$($manifest.version)' does not match '$resolvedVersion'."
    }

    Assert-CleanWorktree "Packaging changed tracked files. Review those changes before publishing."

    Invoke-Native git push origin "HEAD:$defaultBranch"
    Invoke-Native git fetch origin
    $localHead = (git rev-parse HEAD).Trim()
    $remoteHead = (git rev-parse "origin/$defaultBranch").Trim()
    if ($localHead -ne $remoteHead) {
        throw "The release commit was not pushed exactly to origin/$defaultBranch."
    }

    Invoke-Native git tag -a $tag -m "MyGamesAnywhere $tag"
    Invoke-Native git push origin "refs/tags/$tag"

    $releaseArgs = @('release', 'create', $tag) + $assets + @(
        '--verify-tag',
        '--title', $tag,
        '--latest'
    )
    $notesPath = Join-Path $repoRoot "docs\releases\$tag.md"
    if (Test-Path -LiteralPath $notesPath) {
        $releaseArgs += @('--notes-file', $notesPath)
    } else {
        $releaseArgs += '--generate-notes'
    }
    Invoke-Native gh @releaseArgs

    $releaseUrl = (gh release view $tag --json url --jq '.url').Trim()
    if ($LASTEXITCODE -ne 0 -or -not $releaseUrl) {
        throw "Release was created, but its GitHub URL could not be verified."
    }

    Write-Host "Published $tag from $defaultBranch as the latest GitHub release." -ForegroundColor Green
    Write-Host $releaseUrl
}
