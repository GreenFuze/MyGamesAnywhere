# Full build (server + plugins + frontend -> bin) then start.ps1.
# The dev runtime is intentionally portable: server/bin is both app dir and data dir.
$ErrorActionPreference = "Stop"
& "$PSScriptRoot/build.ps1"
& "$PSScriptRoot/start.ps1" @args
