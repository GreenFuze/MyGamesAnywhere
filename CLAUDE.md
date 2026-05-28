# MyGamesAnywhere — Project Instructions

## SuitCode is mandatory

SuitCode is a CLI tool that provides authoritative import-graph intelligence for this repository.
Run `suitcode --help` for the full command reference. Run `suitcode . <command> --help` for per-command details.

**Non-negotiable workflow:**

1. **Session start** — run `suitcode . warmup` once before any other command.
2. **Before reading or editing any code file** — run `suitcode . context --files <files>` first.
   Do not open files with Read/Grep/cat until you have run context on them.
3. **After every context/explain-file/impact call** — run `suitcode . feedback good|bad`.
   This is mandatory, not optional. Check `suitcode . status` to see how many calls are unrated.
4. **After session compaction** — re-anchor immediately with `suitcode . context --from <ref>`.

`suitcode . status` shows current readiness and how many calls are awaiting feedback.
`suitcode . metrics summary` shows the session overview (feedback rate, errors, compression).

Do not use Read/Grep/cat to explore code you haven't run `context` on first.
SuitCode output is the authoritative source; manual file reads are the fallback when SuitCode explicitly reports a limitation.
