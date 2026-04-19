# Browser-Play Save Proof

This directory now holds both:

- the automated end-to-end proof runner for browser-play save import/export
- the checklist/output surface for manual follow-up when needed

Use it when you need deterministic save/import-export proof across:

- `EmulatorJS`
- `js-dos`
- `ScummVM`

## Automated E2E Proof

Build the frontend, install the proof runner dependencies, install Chromium for Playwright, then run the proof runner:

```bash
cd server/frontend
npm run build
cd ../../tools/browser-play
npm install
npx playwright install chromium
npm run proof:e2e
```

The automated runner:

- serves the built frontend and runtime assets from `server/frontend/dist`
- provides deterministic fake API responses for browser-play proof games
- exercises the real `GamePlayerPage` save/load UI and the real runtime bridge
- verifies:
  - EmulatorJS save import/export round-trip
  - bundle-backed js-dos save import/export round-trip
  - plain-file js-dos unsupported-state fast fail
- ScummVM save import/export round-trip
- writes `proof-report.md` into a timestamped `proof-output/` directory

## EmulatorJS

Automated proof now uses a repo-local NES fixture and the real runtime page/save bridge.

Checklist summary for manual follow-up:

1. Launch a supported EmulatorJS game from the game detail page.
2. Create or modify in-runtime save data.
3. Use the player-page Save action to export to the active save-sync integration.
4. Clear or overwrite the in-runtime state.
5. Use the player-page Load action to import the saved snapshot.
6. Confirm the runtime restores the exported state.

## js-dos

Automated proof now uses:

- a repo-local bundle-backed `.zip` fixture for the supported save path
- a repo-local plain `.exe` fixture for the unsupported fast-fail path

Important:

- plain-file js-dos sessions are intentionally treated as unsupported for save import/export
- the UI should show the explicit unsupported-state message for those sessions

Checklist summary for manual follow-up:

1. Launch a bundle-backed js-dos game.
2. Create or modify runtime state.
3. Save to the active save-sync integration.
4. Replace or clear local runtime state.
5. Load from the saved snapshot.
6. Confirm the restored state matches the exported snapshot.

## ScummVM

Automated proof now uses the real ScummVM runtime page plus repo-local proof fixtures for save-path verification.

`tools/scummvm-harness` still remains useful for manual launch-path inspection, but the save import/export proof no longer depends on an external game tree.

Checklist summary for manual follow-up:

1. Start the harness server documented in `tools/scummvm-harness/README.md` if you want to inspect launch behavior manually.
2. Verify export and import through the ScummVM runtime bridge.
3. Record any external game tree path only if you intentionally validate a real game launch outside the automated proof.

## Completion Rules

Do not mark save proof complete unless:

- the runtime bridge exposes both export and import commands
- the launch path used is compatible with save import/export for that runtime
- the restored runtime state is verified after a real save/load round trip
