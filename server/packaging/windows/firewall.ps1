param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("add","remove")]
    [string]$Action,
    [string]$RuleName = "MyGamesAnywhere",
    [string]$Program
)

$ErrorActionPreference = "Stop"

function Invoke-Native {
    param(
        [Parameter(Mandatory=$true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments
    )

    $output = & $FilePath @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath $($Arguments -join ' ') failed with exit code $LASTEXITCODE. Output: $($output -join [Environment]::NewLine)"
    }
    return $output
}

if ($Action -eq "remove") {
    & netsh advfirewall firewall delete rule name="$RuleName" | Out-Null
    exit 0
}

if (-not $Program) {
    throw "Program is required when adding the firewall rule."
}

& netsh advfirewall firewall delete rule name="$RuleName" | Out-Null
Invoke-Native netsh advfirewall firewall add rule name="$RuleName" dir=in action=allow program="$Program" enable=yes profile=private | Out-Null
