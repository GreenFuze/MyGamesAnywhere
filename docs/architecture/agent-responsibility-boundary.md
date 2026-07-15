# Decision Responsibility and Escalation Boundary

- **Status:** Required operating policy
- **Scope:** Every MGA coding session, regardless of agent vendor or model

## Purpose

MGA separates unresolved architectural decisions from implementation, but those
activities do not require different agents or sessions. Model cost is only a
practical signal: the important distinction is whether the active agent is
authorized and capable of resolving cross-system product, security, persistence,
and compatibility decisions.

- A **regular implementation agent** (for example Composer 2.5 or an equivalent
  coding model) executes decisions that are already recorded.
- An **architecture-capable agent** (for example GPT-5.6 Sol or an equivalent
  high-reasoning Codex/model) may resolve, record, and implement decisions in
  one continuous session.

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

The architecture agent may implement all follow-through itself. Delegating
mechanical work to a bounded agent is an optional cost/parallelism choice, not a
required boundary.

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

When one architecture-capable agent owns the work, it may stay in the same
session:

1. inspect evidence and identify the unresolved decision;
2. record the decision, migration/compatibility/security policy, and acceptance
   criteria in the authoritative docs;
3. implement and verify that recorded decision;
4. update the handoff with exact evidence.

It does not stop merely because work crosses architecture, protocol, migration,
security, or implementation layers. It stops only for a genuine user/product
choice, unavailable authority/credentials, destructive ambiguity not safely
resolvable from policy, or contradictory evidence requiring user direction.

Use a separate session when deliberately delegating to an implementation-only
agent, changing to a weaker model, or reducing excessive context. In that case,
the architecture-capable agent first writes the bounded packet and the delegated
agent follows the stop conditions above.

## Current MGA boundary

The completed ZIP/7z/RAR slice is an implementation task and may be reviewed,
committed, or maintained by a regular agent.

ADR-0007 now records the first bounded EXE/BIN packet: web-authorized signed GOG
Inno Setup installation, exact post-success crash acceptance, and marked
failed-install cleanup/Ignore. A regular agent may implement only that packet.
Another publisher/family, generic execution, standalone prerequisite, changed
elevation/cancellation/rollback behavior, or destructive ownership decision
must be escalated to an architecture-capable agent.

ADR-0008 separately records the bounded device-selected Installed Games Play
shelf. A regular agent may implement it without changing device association,
authorization, installation-state, or other shelf semantics beyond that ADR.
