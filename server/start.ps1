# Starts the packaged server from server\bin (same as run.ps1).
# Use after build.ps1 so plugins, frontend dist, and mga_server.exe are in bin.
& "$PSScriptRoot/run.ps1" @args
cd ..