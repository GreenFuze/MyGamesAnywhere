# Agent Responsibility and Escalation Boundary

- **Status:** Required operating policy
- **Scope:** Every MGA coding session, regardless of agent vendor or model

## Purpose

MGA separates architectural decisions from implementation execution. Model cost
is only a practical signal: the important distinction is whether an agent is
authorized and capable of resolving cross-system product, security, persistence,
and compatibility decisions.

- A **regular implementation agent** (for example Composer 2.5 or an equivalent
  coding model) executes decisions that are already recorded.
- An **architecture-capable agent** (for example GPT-5.6 Sol or an equivalent
  high-reasoning model) resolves and records decisions before implementation.

## Regular implementation agent

A regular agent may:

- implement a bounded task whose behavior and acceptance criteria are explicit;
- follow existing ADRs, protocol contracts, migrations, and handoffs;
- add focused tests, generated contracts, packaging, and end-to-end verification;
- fix a local defect when the intended behavior is already unambiguous;
- refactor internals without changing public behavior or persistence.

It must not choose among unresolved options or silently establish precedent. It
must stop before changing:

- SQLite tables, persisted JSON/config, migration/default/rollback policy;
- protocol messages, command families, capabilities, authorization, or access;
- credentials, network trust, remote execution, elevation/UAC, or sandboxing;
- game identity, edition/copy/DLC/save compatibility, or reconciliation meaning;
- destructive filesystem boundaries, uninstall ownership, or external-removal
  semantics;
- bundled executables/libraries, licensing, update/signing, or prerequisites;
- player-facing defaults, precedence rules, automation, or destructive UX;
- cross-component ownership between server, web UI, client, and plugins.

When uncertain, the regular agent does not infer permission from nearby code,
an old roadmap, or a partially implemented branch.

## Architecture-capable agent

The architecture agent:

1. reads the current authoritative handoff, relevant ADRs, protocol, code, and
   migrations;
2. states the user outcome and security/ownership boundaries;
3. compares viable options and recommends one with concrete trade-offs;
4. records the accepted decision in an ADR/protocol/plan;
5. states migration or `NO_MIGRATION_NEEDED`, compatibility, rollback, and
   failure behavior;
6. creates a bounded implementation packet with exact acceptance criteria,
   required tests, packaging, and end-to-end evidence;
7. reviews deviations or genuinely ambiguous failures escalated by the regular
   agent.

The architecture agent may implement difficult cross-layer work, but should
still hand mechanical follow-through to a regular agent when that saves cost
without weakening decisions.

## Escalation packet

Before stopping, a regular agent writes:

```text
Decision required:
Current behavior/evidence:
Why existing docs do not decide it:
Options and trade-offs:
Recommended option:
Persistence/migration impact:
Security/destructive impact:
Tests and compatibility affected:
Work safely completed so far:
```

It must not modify the decision-sensitive surface while waiting.

## Session and model policy

Use separate sessions by default:

1. The architecture agent makes and records the decision.
2. It updates the authoritative handoff with a narrow implementation packet.
3. A fresh regular-agent session reads only the handoff, required ADRs, and
   relevant files, then implements that packet.
4. Escalate back to an architecture agent only when a stop condition is reached.

A fresh implementation session reduces inherited noise, token cost, and scope
drift. Switching a long architecture session to a cheaper model is acceptable
only for a short, fully specified mechanical action. It is not the default for
a multi-file feature.

## Current MGA boundary

The completed ZIP/7z/RAR slice is an implementation task and may be reviewed,
committed, or maintained by a regular agent.

The next EXE/BIN installer and prerequisite slice begins with architecture-agent
work because it requires decisions about typed execution, interactivity,
elevation, cancellation, rollback, shared prerequisites, and destructive
ownership. A regular agent may implement it only after those decisions and
acceptance tests are recorded.
