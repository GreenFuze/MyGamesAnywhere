# MGA documentation

## Current sources of truth

- [Jira MGA backlog](https://greenfuzer.atlassian.net/jira/software/c/projects/MGA/boards/69/backlog)
  owns all open work, priorities, assignments, and progress.
- [Confluence MGA](https://greenfuzer.atlassian.net/wiki/spaces/MG/overview)
  owns current product, UX, architecture, security, and operating guidance.
- Git owns executable code, immutable database migrations, tests, release
  artifacts, and code-coupled rules.

Start a new human or agent session with
[`agent-bootstrap.md`](agent-bootstrap.md).

Local ADRs and protocols remain close to the code as decision evidence and
implementation contracts. Dated handoffs, old task lists, and old release
planning files are historical evidence only. Never infer current status from
them; check Jira.

## Documentation changes

Update Confluence when current behavior, architecture, product policy, UX
language, security, or operating guidance changes. Update Jira when work is
discovered, claimed, reprioritized, blocked, or completed. Update local files
only where the information must version with code.
