# ScummVM Harness

Standalone ScummVM web-runtime harness for debugging outside the MGA app flow.

It serves:

- the existing ScummVM runtime from `server/frontend/public/runtimes/scummvm`
- a live HTTP manifest of `\\tv2\Games\ScummVM\Island of Dr. Brain (Floppy DOS)`
- the game files themselves with range-request support

## Run

From the repository root:

```powershell
node .\tools\scummvm-harness\server.mjs
```

Then open:

```text
http://127.0.0.1:43123/
```

## Overrides

You can override the target game path or port with environment variables:

```powershell
$env:SCUMMVM_HARNESS_GAME_ROOT='\\tv2\Games\ScummVM\Island of Dr. Brain (Floppy DOS)'
$env:SCUMMVM_HARNESS_PORT='43123'
node .\tools\scummvm-harness\server.mjs
```

## Purpose

This is intentionally outside MGA's browser-play session code so the ScummVM runtime can be debugged in isolation against a real game tree.
