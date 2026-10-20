# troupe-go

A Go library for [Troupe](https://github.com/TroupesAI/troupe), an open file format for AI roleplay. It reads Troupe files, checks them, and computes what they mean: the state at a message, how macros render, how conditions evaluate and who knows what.

**Status:** in development, targeting Troupe 0.14, the spec's current draft. It passes the whole conformance suite; the [plan](docs/plan.md) says what is still to come.

## Scope

In scope: everything the [spec](https://github.com/TroupesAI/troupe/blob/main/spec/format.md) defines.

- Reading bundles, packages and sessions: I-JSON, the limits, versions, the schemas and unknown fields (Format 2.2, 11, 12, 13, 14).
- The checks and their codes (Format 15.3).
- The state at a message (Format 8.6).
- Macros (Format 9) and conditions in CEL (Format 10).
- Entry activation (Format 7.2) and knowledge (Format 7.4, 11.5).
- Writing: roots made from openings (Format 11.1), escaping (Format 9), files written back with every `ext` unchanged (Format 13), and IDs checked as Format 2.3 requires and renamed with their references.

Out of scope: everything the spec leaves to apps (Format 1.1). That includes prompt assembly, choosing which active entries reach a model, model calls, storage and UI.

## Rules

- **The spec decides.** When the code and the spec disagree, the code is wrong. When the spec is wrong or unclear, it's fixed upstream, never worked around here.
- **No I/O.** The library doesn't open files or network connections, and it doesn't read the environment. Callers pass the data in.
- **Deterministic.** Randomness, such as `{{pick}}` and `chance`, comes from a source the caller supplies, so every result can be reproduced.
- **Bounded.** The limits of Format 14 are enforced while reading, before any expensive work, so hostile input can't exhaust memory or CPU.
- **Content stays private.** Errors and diagnostics carry codes, locations and IDs, never text from a document, since apps log them.

## Conformance

The spec's [conformance suite](https://github.com/TroupesAI/troupe/tree/main/conformance) is the main test. The suite, the schemas and the examples some cases read are copied into this repository from a pinned spec commit. A runner executes every case through the public API. CI fails when a case fails, or when the copy no longer matches its commit.

## Versioning

Before 1.0 there are no releases: callers require a pushed commit as a pseudo-version. From 1.0, releases are tagged `v<spec major>.<spec minor>.<patch>`, so `v1.0.2` reads Troupe 1.0.

## Development

The tools are pinned in `mise.toml`. Run `mise install`, then:

| Task | Does |
| --- | --- |
| `task lint` | Lints the code and checks dependencies for vulnerabilities |
| `task test` | Runs the tests, the conformance suite included |
| `task check` | Runs what CI runs: lint, tests and the drift check |
| `task spec REF=<tag or commit>` | Replaces the copy in `spec/` with that commit's schemas, suite and examples |
| `task spec:local` | Copies them from a checkout at `../troupe`, to try a spec change before it's pushed. The drift check fails until the copy comes from a pushed commit |

Never edit `spec/` by hand. `spec/REF` names the commit it comes from.

## License

[Apache-2.0](LICENSE), apart from `spec/examples/`, the spec's worked examples copied from the spec repository, which are [CC BY 4.0](https://github.com/TroupesAI/troupe/blob/main/spec/LICENSE) as they are there.
