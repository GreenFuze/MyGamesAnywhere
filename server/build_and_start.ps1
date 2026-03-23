# Full build (server + plugins + frontend → bin) then start.ps1 (binary from bin, no npm).
$ErrorActionPreference = "Stop"
& "$PSScriptRoot/build.ps1"
& "$PSScriptRoot/start.ps1" @args
