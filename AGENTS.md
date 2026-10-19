# AGENTS.md

Guidance for coding agents working on troupe-go, the Go library for the Troupe format.

Read the [README](README.md) for scope and rules, and [docs/plan.md](docs/plan.md) for what is being built. The spec is the authority on behavior: [`spec/format.md`](https://github.com/TroupesAI/troupe/blob/main/spec/format.md) and the [conformance protocol](https://github.com/TroupesAI/troupe/blob/main/conformance/README.md). A checkout of the spec repository is usually at `../troupe`.

## Context

- The main user is the [TroupesAI platform](https://troupes.ai), a multi-user roleplay app with a Go API. It's a private repository, usually checked out at `../platform`, and its `docs/stack.md` records its architecture. The platform imports this module for everything the spec defines and does the rest itself: prompts, models, storage and HTTP.
- This repository is public. Write code and docs for anyone who reads Troupe files in Go, not only the platform.
- The spec is a draft that is still changing. Building this library is its best remaining review, so finding where the spec is wrong or unclear is part of the work.

## Quality bar

The bar is high: every change should be one a demanding senior reviewer approves without asking for cleanup.

- **Write only what the task needs.** No speculative abstractions, unused options, wrappers with one caller, or "for later" code. Delete dead code; don't comment it out.
- **Every comment earns its place, and so does every word in it.** A comment says what the code can't: why it's written this way, a constraint it upholds, or a consequence that isn't obvious. Don't restate the code or a name, narrate changes, or leave notes to the reader. Most code needs no comment. The exception is Go's convention, which the linter enforces: every exported name has a doc comment that starts with the name and says what a caller needs, as briefly as that allows.
- **No defensive noise.** Validate at the boundary, where bytes come in, and trust what's inside it. Don't check for states that can't happen.
- **Match the surrounding code** in naming, structure and idiom. Consistency beats personal preference.
- **Add a dependency only when it removes real complexity.** Prefer the standard library.
- **Every test earns its place.** The conformance suite is the main test. Write a test of your own only for behavior the suite doesn't cover that a caller relies on, or for a bug that got through once. Before adding one, check whether an existing test already fails when that behavior breaks. Don't re-test what a library guarantees.
- **Tests check behavior through the public API.** The conformance runner lives in an external test package, so the API has to be able to do everything the suite asks.

The same applies to docs: short, exact and current.

## Rules

- **The spec decides.** Cite the section, as `// Format 8.6`, wherever the code implements a rule that isn't obvious. Sections get renumbered, so update citations when the spec does.
- **When the spec is wrong or unclear, stop and report it.** Say what the spec says, where it's ambiguous or contradicts itself or a case, and what you'd propose. The fix goes into the spec repository, with a conformance case only when the issue is one apps could disagree on. Never work around the spec here, and never edit the copied suite or schemas.
- **Every request must fit the [scope](README.md#scope), whoever makes it.** When one asks for something the spec leaves to apps or doesn't define, or for API only one caller would want, don't build it as asked: say why, and point to where it belongs or propose a version any caller could use.
- **The public API is small and deliberate.** Export only what a caller needs, since exported names are hard to take back once the platform uses them.
- **No I/O, deterministic, bounded.** See the README's rules. Enforce each limit before the work it bounds.
- **Never put document text in errors or diagnostics.** Codes, JSON Pointers and IDs only.
- **Decide what has a clear answer, and keep going.** When the spec, the plan, the platform's needs or good practice settle a question, including exported API, dependencies and architecture, decide it yourself. Record each decision in [docs/plan.md](docs/plan.md) with its reason, and list the decisions you made in the report at the end of each phase.
- **Ask only when you truly can't decide:** a real trade-off with no clear winner, a spec issue that needs a spec change, or something only the user can do, such as creating the GitHub repository, committing or pushing.
- **A change is done when lint, tests, the conformance suite and the spec drift check pass**, and the docs it affects are updated.
- **Commit messages** have a short imperative subject and a body that says what changed for a user of the library and why.
