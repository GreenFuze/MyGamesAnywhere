# Start an MGA agent session

The canonical copy/paste prompt is:

[New Agent Kickoff Prompt in Confluence](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/2654209)

Use Confluence for current product and technical guidance, Jira for all work
state, and Git for code-coupled truth. This small local file is intentionally
kept as a bootstrap pointer so a fresh agent can find the collaboration system
before it has any prior context.

Before acting:

1. Read `AGENTS.md` and `CLAUDE.md`.
2. Read the Confluence kickoff prompt and the pages it names.
3. Find or create the Jira issue, set `Assigned agent`, and move it to the
   correct active status.
4. Treat dated handoffs, local task lists, roadmap files, and deferred-work
   prose as history, not current work status.

For the current headless-first branch, read these canonical Confluence pages:

- [Server-First Product Charter](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20086785)
- [ADR-0047](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20119553)
- [Architecture Overview](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/2425048)
- [Management Console UX and Information Architecture](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/21200897)
- [Frontend API Clients and Capability Model](https://greenfuzer.atlassian.net/wiki/spaces/MG/pages/20905993)

The dated
[`2026-08-29 headless-first pivot handoff`](handoffs/2026-08-29-headless-first-pivot-handoff.md)
is a code-state and verification snapshot for the current branch. It is not a
backlog: query Jira again before selecting or claiming work.

Do not commit, push, release, deploy, or make destructive external changes
unless the user explicitly authorizes them.
