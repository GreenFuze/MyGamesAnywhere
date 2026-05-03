param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("add","remove")]
    [string]$Action,
    [string]$RuleName = "MyGamesAnywhere",
    [string]$Program
)

$ErrorActionPreference = "Stop"

if ($Action -eq "remove") {
    & netsh advfirewall firewall delete rule name="$RuleName" | Out-Null
    exit 0
}

if (-not $Program) {
    throw "Program is required when adding the firewall rule."
}

& netsh advfirewall firewall delete rule name="$RuleName" | Out-Null
& netsh advfirewall firewall add rule name="$RuleName" dir=in action=allow program="$Program" enable=yes profile=private | Out-Null
