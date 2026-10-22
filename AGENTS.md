# AGENTS.md

Guidance for coding agents working on troupe-go, the Go library for the Troupe format.

Read the [README](README.md) for scope and rules, and [docs/plan.md](docs/plan.md) for what is being built.

## Quality bar

The bar is high: every change should be one a demanding senior reviewer approves without asking for cleanup.

- **Write only what the task needs.** No speculative abstractions, unused options, wrappers with one caller, or "for later" code. Delete dead code; don't comment it out.
- **Every comment earns its place, and so does every word in it.** A comment says what the code can't: why it's written this way, a constraint it upholds, or a consequence that isn't obvious. Don't restate the code, a name or a signature, repeat another comment, narrate changes, or leave notes to the reader. When a comment has to change, rewrite it whole rather than add to it. Most code needs no comment. The exception is Go's convention, which the linter enforces: every exported name has a doc comment that starts with the name and says what a caller needs, as briefly as that allows.
- **No defensive noise.** Validate at the boundary, where bytes come in, and trust what's inside it. Don't check for states that can't happen.
- **Match the surrounding code** in naming, structure and idiom. Consistency beats personal preference.
- **Add a dependency only when it removes real complexity.** Prefer the standard library.
- **Every test earns its place.** The conformance suite is the main test. Write a test of your own only for behavior the suite doesn't cover that a caller relies on, or for a bug that got through once. Before adding one, check whether an existing test already fails when that behavior breaks; if so, extend it rather than adding a near-copy. Don't re-test what a library guarantees, and don't assert values nobody relies on, such as tuning constants. A regression test must fail reliably when its bug comes back; if no test can, explain the guarantee in a comment where the code provides it.
- **Tests check behavior through the public API.** The conformance runner lives in an external test package, so the API has to be able to do everything the suite asks.

The same applies to docs: short, exact and current.

## Rules

- **The spec decides:** `spec/format.md` and `conformance/README.md` in `../troupe`, read at the commit `spec/REF` names. Cite the section, as `// Format 8.6`, wherever the code implements a rule that isn't obvious. Sections get renumbered, so update citations when the spec does.
- **Finding where the spec is wrong or unclear is part of the work**, since the spec is a draft and building this library is its best review. Report each finding as [Working across the three repositories](#working-across-the-three-repositories) says: what the spec says, where it's ambiguous or contradicts itself or a case, and any fix you'd suggest. Never edit `spec/` by hand.
- **Hold every request to the [scope](README.md#scope), whoever makes it.** When one falls outside it, don't build it as asked: say why, and point to where it belongs or propose a version any caller could use.
- **The public API is small and deliberate.** Export only what a caller needs.
- **No I/O, deterministic, bounded, and content stays private.** See the README's rules. Enforce each limit before the work it bounds.
- **Decide what has a clear answer, and record it.** When the spec, the code or good practice settles a question, decide it yourself. Record each decision a later change must respect in [docs/plan.md](docs/plan.md), with its reason, and list them in the commit body. Ask only about a real trade-off with no clear winner.
- **Propose first when a change would be costly to undo:** a new dependency, a new Go package, or a change to the README's rules.
- **Fix what you find along the way** when the fix is clear and small, in its own commit. Propose it instead when it's a design choice or would outgrow the task.
- **A change is done when `task check` passes**, and the docs it affects are updated.
- **Every change ends with a review round.** Once it works, review the whole diff with fresh eyes, through a separate agent where one is available, for defects, needless code, and docs or comments that disagree with the code or don't earn their place. Check each finding against the code, fix the ones that hold, and list what was found and fixed in the commit body.
- **Commit and push to `main`.** A commit message has a short imperative subject and a body that says what changed for a user of the library and why.

## Working across the three repositories

Troupe is built in three repositories, checked out side by side. Nothing has outside users yet and all data is disposable. Until that changes, a change needs no deprecation, data migration, compatibility shim or changelog: fix what's wrong, in the repository that owns it.

Each repository owns one layer:

- **troupe** (`../troupe`), the spec. Owns what a Troupe file contains and means, and what every conforming app must do with it: the format, the schemas, the conformance suite and the guides. Doesn't own any implementation, library API, or behavior the spec leaves to apps. *Test: without it, could two apps reading the same file disagree?*
- **troupe-go** (`../troupe-go`), the Go library. Owns computing what the spec defines, for any Go caller: reading, checks, state, macros, conditions, activation and writing. Doesn't own anything the spec leaves undefined, or API only one caller would use. *Test: would Go programs other than the platform need it, in this form?*
- **platform** (`../platform`), the TroupesAI app. Owns everything the spec leaves to apps (Format 1.1): prompts, models, storage, HTTP, UI, and app-only data in `ext`. Doesn't own any Troupe semantics; it gets those from troupe-go. *Test: is it a product choice for this app?*

Their full scopes are Format 1.1, troupe-go's README and the platform's `docs/stack.md`. A workaround is code in one layer that does another layer's work: reimplementing, patching or second-guessing it, or making up for its bug or gap. Never write one.

- **Read the other repositories; never change them.**
- **When the task needs another repository to change, don't finish it.** Go through the rest of the task and collect everything it needs from other repositories, without building on an assumed answer. Then stop and give the user a prompt for each repository, in the order troupe, troupe-go, platform. When one depends on another's answer, give only the first, and write the next from its hand-back. A finding the task doesn't depend on doesn't stop it: finish, and end with a prompt for it.
- **A prompt stands on its own**, since the agent that receives it doesn't share this conversation. It says what's needed, why, and how the result will be used, cites the spec sections involved, and leaves the design to that repository.
- **The receiving repository decides.** When it declines a request or proposes another design, adopt its answer or argue against it on the merits. Never build the declined version locally.
- **End a change another repository waits on with a hand-back:** what changed, the pushed commit to depend on, what it breaks, and each dependent repository's next step, such as `task spec REF=<commit>` in troupe-go or `go get github.com/TroupesAI/troupe-go@<commit>` in the platform's `apps/api`.
- **Adopt upstream changes fully.** Depend on a pushed commit. A local `replace` or copy is only for trying a change, never committed. Delete what the change makes redundant, and fix what it breaks without a shim.
- **This section is the same in all three repositories.** A change to it, or to a repository's scope, goes to the other two as a request, so all three change together.
