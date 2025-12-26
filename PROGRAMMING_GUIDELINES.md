# Style Guide

This repo prefers **maintainable, explicit, fail-fast code**. Optimize for correctness, clarity, and long-term evolution.

## 0) Non-negotiables

- **Fail-fast always**: detect problems early, raise loudly, never silently “fix” or hide issues.
- **Prefer OOP**: model the domain with objects and responsibilities. Avoid “bag of functions” designs unless the language/task strongly suggests otherwise.
- **Small functions/methods**: if it’s getting big, refactor.
- **RAII / deterministic cleanup**: resources must be acquired/owned and reliably released.
- **Minimal duplication**: keep code DRY, but don’t over-abstract trivial logic.

---

## 1) OOP by default

Use objects to represent:
- **Domain entities/value objects** (data + invariants)
- **Services** (orchestrate domain operations)
- **Repositories/Adapters** (IO boundaries: DB, filesystem, network)
- **Use-cases/Commands** (one operation per object, easy to test)

Guidelines:
- Prefer **composition over inheritance**.
- Keep classes cohesive: **one responsibility**.
- Keep object state valid: enforce invariants in constructors/factories.

When *not* to force OOP:
- A tiny pure utility where a class would add ceremony.
- Language ecosystems where idiomatic structure is not class-centric (still: keep structure and boundaries).

---

## 2) Keep functions/methods small

A method should do **one thing**. If you feel the urge to add “and also…”, split.

Rules of thumb:
- If you need many comments to explain a method, it’s probably too big.
- If you have deep nesting, extract helpers.
- Prefer early returns / guard clauses over pyramids of `if`.

Refactor triggers:
- Many parameters (consider a parameter object).
- Repeated blocks (extract a helper or strategy).
- Mixed concerns (validation + IO + business logic in one method).

---

## 3) RAII / deterministic resource management

Any resource (file handle, socket, lock, subprocess, temp dir, DB connection, etc.) must have:
- a clear **owner**
- a clear **lifetime**
- a guaranteed **cleanup path**

Rules:
- No “fire-and-forget” resources.
- Ensure cleanup on exceptions/errors.
- Use the language’s RAII mechanism:
  - C++: destructors, smart pointers
  - Rust: Drop, ownership/borrowing
  - Python: context managers (`with`), `try/finally`
  - JS/TS: explicit `dispose()` + `try/finally` (or a scoped helper)

---

## 4) DRY without over-abstraction

Goal: reduce meaningful duplication, not remove all repetition.

Guidelines:
- Prefer removing duplication via **better decomposition** (objects, helpers, strategies).
- Avoid premature abstraction:
  - If logic appears once, keep it local.
  - If it appears twice but is small, consider keeping it duplicated if abstraction hurts readability.
  - If it appears 3+ times or changes together, extract.

Trivial methods:
- Don’t create functions/classes that only rename a single call unless it buys clarity or enforces an invariant.

---

## 5) Fail-fast policy (no silent fallback)

Rules:
- Validate inputs at boundaries (public APIs, parsing, IO).
- Assert invariants internally (this should “never happen”).
- Prefer exceptions/errors that **stop execution** over “graceful” partial success.
- Never swallow errors. If you catch, you must:
  1) add context, and
  2) rethrow/propagate, unless a clearly defined recovery boundary exists.

Recovery boundaries (allowed):
- At the *outermost layer* (CLI/UI/API handler), convert internal exceptions into user-facing errors.
- Still log/report the failure with actionable detail.

---

## 6) Typing, schemas, and validation

- Prefer **strong typing** where available.
- Use **enums** over string literals.
- Use schema validation for external data (config files, HTTP payloads, DB rows):
  - Validate early, store validated objects, avoid passing raw dicts/maps around.
- Be explicit with nullability/optionality.

---

## 7) Errors and diagnostics

Error messages should be:
- specific (what failed)
- contextual (which input/operation)
- actionable (what to do next)

Rules:
- Don’t lose the root cause: preserve the original exception/error as the cause.
- Avoid generic `Exception("failed")`.
- Prefer dedicated exception types for domain errors vs. infrastructure errors.

---

## 8) Structure and dependencies

- Keep a clean layering:
  - **Domain** (pure logic) should not depend on infrastructure.
  - **Infrastructure** depends on domain interfaces/adapters.
  - **Entry points** (CLI/web handlers) wire everything.

- Dependency direction should flow inward.
- Prefer dependency injection (explicit constructor params) over hidden globals/singletons.

---

## 9) Testing expectations (style-related)

- Tests are first-class: new behavior should come with tests.
- Prefer deterministic tests (no sleeps, no flaky timing).
- Test the public behavior of a component, not its private internals.
- If a bug is fixed, add a regression test.

---

## 10) Code review checklist

Before merging:
- Does it fail fast on invalid inputs?
- Is ownership/lifetime of resources clear (RAII)?
- Are methods small and single-purpose?
- Any meaningful duplication?
- Are types/enums used consistently?
- Do errors carry useful context?
- Are tests present and meaningful?

---

## Suggested extra rules (optional, but recommended)

1) **Prefer pure domain logic**: keep business rules free of IO and side effects.  
2) **No hidden magic**: avoid reflection/dynamic dispatch unless it clearly pays off.  
3) **Explicit configuration**: config should be validated and immutable after load.  
4) **Time & retries**: if you do network/IO, enforce timeouts; retries must be explicit and bounded.  
5) **No global state** (or keep it minimal and behind interfaces).  
6) **Consistency over cleverness**: simpler is better, even if slightly longer.
