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

Do not commit, push, release, deploy, or make destructive external changes
unless the user explicitly authorizes them.
