param(
    [string]$BaseRef = $env:GITHUB_BASE_REF
)

$ErrorActionPreference = "Stop"

$repoRoot = (& git rev-parse --show-toplevel).Trim()
if (-not $repoRoot) {
    throw "Could not resolve repository root."
}
Push-Location $repoRoot
try {
    $changed = @()
    if ($BaseRef) {
        $base = "origin/$BaseRef"
        $changed += & git diff --name-only "$base...HEAD" 2>$null
    }
    if (-not $changed) {
    $changed += & git diff --name-only "HEAD~1...HEAD" 2>$null
    }
    $changed += & git diff --name-only --cached
    $changed += & git diff --name-only
    $changed += & git ls-files --others --exclude-standard -- AGENTS.md .github server/internal server/scripts server/packaging docs README.md roadmap.md
    $changed = @($changed | Where-Object { $_ } | Sort-Object -Unique)
    if ($changed.Count -eq 0) {
        Write-Host "Migration guard: no changed files detected."
        exit 0
    }

    $migrationChanged = $changed | Where-Object {
        $_ -match '^server/internal/db/migrations.*\.go$' -or
        $_ -match '^server/internal/db/migrations/'
    }

    $ownedPatterns = @(
        '^server/internal/db/.*\.go$',
        '^server/internal/core/entities\.go$',
        '^server/internal/sync/.*\.go$',
        '^server/plugins/.*/(plugin\.json|manifest\.json)$',
        '^server/plugins/.*/.*config.*\.go$'
    )

    $dbOwnedChanged = @()
    foreach ($path in $changed) {
        foreach ($pattern in $ownedPatterns) {
            if ($path -match $pattern) {
                $dbOwnedChanged += $path
                break
            }
        }
    }
    $dbOwnedChanged = @($dbOwnedChanged | Sort-Object -Unique)
    if ($dbOwnedChanged.Count -eq 0) {
        Write-Host "Migration guard: no DB-owned persisted files changed."
        exit 0
    }
    if ($migrationChanged) {
        Write-Host "Migration guard: migration change detected."
        exit 0
    }

    $noteFound = $false
    foreach ($path in $dbOwnedChanged) {
        if (Test-Path -LiteralPath $path) {
            $content = Get-Content -LiteralPath $path -Raw -ErrorAction SilentlyContinue
            if ($content -match 'NO_MIGRATION_NEEDED') {
                $noteFound = $true
                break
            }
        }
    }
    if ($noteFound) {
        Write-Host "Migration guard: NO_MIGRATION_NEEDED note detected."
        exit 0
    }

    $list = ($dbOwnedChanged -join "`n  - ")
    throw "DB-owned persisted files changed without a migration or NO_MIGRATION_NEEDED note:`n  - $list"
} finally {
    Pop-Location
}
