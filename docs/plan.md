# Plan

How troupe-go gets built: what's decided, what's still open, and the order of the work. Update this document when a decision changes, and delete each phase's section once it's done and its decisions are recorded under [Decisions](#decisions).

## Why this repository exists

The engine was first planned as `packages/troupe` inside the platform's monorepo. It moved here, before any code was written, for three reasons:

- The spec is public and the platform is private. An open format gains from an open implementation that anyone can run against the suite, and running it is also how the suite itself gets tested.
- The engine already had to stay free of the platform. A repository boundary enforces that.
- Its versions follow the spec's, not the platform's.

The cost is that a change touching both the engine and the platform can't land in one commit. Go keeps the cost small: the platform can require any commit of this module as a pseudo-version, so it needs no release for each change.

## Decisions

| Decision | Instead of | Why |
| --- | --- | --- |
| Module `github.com/TroupesAI/troupe-go`, package `troupe` | A path inside the spec repository | The spec repository holds only the spec, so it stays language-neutral |
| A public repository, `TroupesAI/troupe-go`, default branch `main` | Private until usable | Anyone can run it against the suite. The user creates it when phase 0 is ready to push |
| Apache-2.0 | Another license | It's the license of the spec repository's code, so the copied schemas and suite need nothing extra, and it grants patent rights |
| Target the 0.14 draft, then each tag | Pinning `v0.13` | The spec now changes in response to what implementing it turns up. Problems found here go into the next minor version |
| Copy the schemas and the suite from a pinned spec commit | A git submodule | Go module zips leave out submodules, so embedded schemas would be missing for anyone who imports the module. A copy with a drift check is pinned just as tightly |
| Copy the spec's `examples/` too, into `spec/examples` | Skipping the cases that read them | Four cases read their bundle and session from `../../../examples/`, so the same layout keeps those paths working. The examples are CC BY 4.0, as the README's License section says |
| The first copy comes from `a867d85` | Waiting for a 0.14 tag | It's the commit that starts the 0.14 draft |
| The drift check downloads `spec/REF` into a temporary directory and diffs it against `spec/` | Overwriting `spec/` and asking git | It never touches the working tree, and it catches added and removed files |
| Tags `v<spec version>.<patch>`, as `v0.14.2` | Versions of its own | The version shows which spec it reads. Revisit at spec 1.0: an API break during 1.x would then need a `/v2` module path |
| `go 1.27.0` in `go.mod` | `go 1.27.2` | It's the minimum for everyone who imports the module, and 1.27 is what `encoding/json/v2` needs. mise pins 1.27.2 for development |
| No clock interface | Passing in the current time | Every time a rule reads comes from a message's `at` (Format 10.1), so nothing reads the clock |
| `encoding/json/jsontext` for the strict pass, `encoding/json/v2` with `WithUnmarshalers` for the typed model | `encoding/json` v1, or a third-party decoder | Stable since Go 1.27. `jsontext` rejects duplicate names, invalid UTF-8 and unpaired surrogates by default, and `StackPointer` locates failures without document text. Caller-supplied unmarshalers handle `8.0` and wrong-typed values without changing exported types. The cost: a program built with `GOEXPERIMENT=nojsonv2` can't import the module |
| Bundles and sessions are read from an `io.Reader`, sessions line by line; packages from an `io.ReaderAt` and a size | `[]byte` | One shape across the readers. A 256 MiB session isn't held as bytes and as a model at once, a package's documents stream from its ZIP entries, and size limits are counted while reading. Readers return an `error` only when the reader fails, so an I/O failure is never reported as `PARSE` |
| [`cel-go`](https://github.com/google/cel-go) for conditions | Our own CEL | It's the reference implementation. Format 10.2 narrows it, and that's done with validators, not a fork |
| [`santhosh-tekuri/jsonschema`](https://github.com/santhosh-tekuri/jsonschema) v6 | Other validators | It supports JSON Schema 2020-12, asserts `format`, and reports instance locations, which diagnostics need |
| Schemas compiled once, on first use, with `sync.OnceValue` | At `init` | Programs that import the module but never read a document don't pay for it |
| A regex parser of our own, emitting RE2 | Compiling keys directly with `regexp` | Format 14 defines a grammar that has to be validated exactly, and keys mean what they mean in ECMAScript, which differs from RE2 in places (see [Pitfalls](#pitfalls)) |
| Tools pinned with mise, tasks in Task, `golangci-lint` v2 | Whatever is installed | The same setup as the platform, so one toolchain works in both |
| The platform's lint config without `bodyclose`, `noctx` and `rowserrcheck` | An identical copy | Those linters only check HTTP and database code, which this library has none of |

## Open decisions

Settle these with the user before the phase that needs them. None are open now.

## Layout

A starting point. Change it once the code argues for something else.

```text
troupe.go, *.go     package troupe: the public API
internal/ijson/     strict reading: I-JSON, depth, number text (Format 2.2, 14)
internal/regex/     regex keys: grammar, limits, translation to RE2 (Format 14)
internal/cel/       the condition environment and the Format 10.2 validators
internal/macro/     macro parsing and rendering (Format 9)
spec/               copied from the spec repository; never edited by hand
  REF               the spec commit the copy comes from
  schemas/          embedded, and compiled on first use
  conformance/      the suite: README, case.schema.json, cases/
  examples/         the spec's worked examples, which some cases read
conformance_test.go package troupe_test: the runner
```

## The public API

The API has to do two things: everything the conformance runner asks for, through exported names only, and everything the platform needs (see [What the platform needs](#what-the-platform-needs)). Phase 1's part is settled (see [Reading](#1-reading)). The rest is a sketch; its names and signatures get settled when each phase starts.

```go
// Checks (Format 15.3), on documents that read without PARSE.
func Check(b *Bundle, s *Session) []Diagnostic

// A story is a bundle played as one scenario, with a session or without one.
func NewStory(b *Bundle, s *Session) (*Story, error)
func (st *Story) StateAt(message string) (State, error)  // "" for the initial state
func (st *Story) KnownBy(message string, part int) []string
func (st *Story) Render(text string, at RenderContext) string
func (st *Story) Eval(expr string, at EvalContext) (bool, error)
```

The suite's operations map onto it like this (conformance README section 3):

| Test | Uses |
| --- | --- |
| `parse` | `ReadBundle`, `ReadSession`, `ReadPackage`, at the session-reader level |
| `check` | `Check`, once per scenario when there's no session |
| `state` | `StateAt`, flattened as section 3.1 says |
| `knows` | `KnownBy` |
| `macro` | `Render`, with `char`, `owner`, `narrator` and `at` |
| `cel` | `Eval`, with `char`, `self`, `scanDepth` and `at`; a compile error is a `CEL_ERROR` |

Flattening a `State` into the suite's keys is the runner's job, unless the platform turns out to need it too. A case whose `bundleFile` is a package's `bundle.json` is read by zipping its directory in memory and calling `ReadPackage`, since package paths are `PARSE` anywhere else.

## Phases

Each phase ends with its part of the suite passing, lint clean, and its decisions recorded above. The runner keeps a **pending list** of tests not expected to pass yet: each entry is a test type, such as `cel`, or one test, as `<case path>#<test index>`. A pending test that fails is skipped, and one that passes fails the run, so the list can't go stale. It shrinks to empty by the end of phase 7, and is then deleted.

### 1. Reading

The goal is `parse` tests passing at the session-reader level, since the platform plays sessions.

```go
// Version is the Troupe version this module reads.
const Version = "0.14"

func ReadBundle(r io.Reader) (*Bundle, []Diagnostic, error)
func ReadSession(r io.Reader) (*Session, []Diagnostic, error)
func ReadPackage(r io.ReaderAt, size int64) (*Package, []Diagnostic, error)

type Package struct {
	Bundle   *Bundle
	Sessions map[string]*Session // by path, such as "sessions/a.troupe-session.jsonl"
}

type Session struct {
	Header   SessionHeader
	Messages []Message // in line order
}

type Diagnostic struct {
	Code    Code
	File    string // the path within a package; "" outside one
	Line    int    // in a session, the line from 1, where 1 is the header; 0 in a bundle
	Message string // the message's ID, when its line has a valid one
	Pointer string // a JSON Pointer within the document
}

// String gives the suite's form: "/scenarios/0/when", "header/players", "message:3/changes/0".
func (d Diagnostic) String() string

type Code string // CodeParse, CodeUnsupportedVersion, CodeSchema, CodeUnknownField; later phases add theirs
```

What a read returns:

- When the reader fails, only the error.
- `nil` for a document that fails `PARSE` or `UNSUPPORTED_VERSION`, pending [spec feedback](#spec-feedback) 1. A package is `nil` when the archive or its `bundle.json` fails; a session that fails is left out of `Sessions`.
- The document otherwise, `SCHEMA` included, so `Check` still runs on it. A value of the wrong type is skipped, so its field reads exactly as if it were absent. In a list, a wrong-typed item makes the whole list absent, so every other item keeps its document index and diagnostics found later point at the right place. That hides diagnostics on the list's other items, such as an `UNKNOWN_CHARACTER` beside a wrong-typed `knownBy` item; revisit if a suite case needs them. A document with any diagnostic doesn't conform: callers use it only to report problems.
- Diagnostics in document order.

The typed model has one struct per spec interface: `Bundle`, `Character`, `Line`, `World`, `Location`, `Scenario`, `CastMember`, `Opening`, `Entry`, `Variable`, `SessionHeader`, `Limits`, `Message`, `Part` and `Change`. It mirrors the document, and defaults are applied by the operations that read them.

- Absence is the zero value where the spec makes the zero value impossible or the same as absence (Format 2.2): strings, lists, maps and booleans. Optional numbers where 0 is a value are pointers, and `Scenario.Worlds` is `*[]string`, since `[]` means no worlds and absence means all of them.
- `ext`, `initial` and the values in `set` are `jsontext.Value`, kept as written.
- Integers are `int` where the spec bounds them within int32, and `int64` where they can reach ±(2^53 − 1).
- Settled with the code: how the unions `Variable.of` and `Part` are represented, and whether the list of defined properties comes from the Go types or the schemas' `properties`. The schemas are preferred if they describe every interface cleanly, so the spec stays the one source.

How a document is read:

1. Size limits are counted while reading, which stops at the limit.
2. The strict pass (`internal/ijson`) checks the bytes and builds the tree the validator takes: UTF-8, no BOM, duplicate names, surrogates and noncharacters, nesting depth 64, number text, and one value per document or line (Format 2.2). Each failure carries its JSON Pointer.
3. Count limits, before any check that walks what they count (Format 14). Then `kind`, and versions (Format 13): a missing or malformed `troupe` is only `SCHEMA`.
4. Unknown fields are reported and removed, then the document is validated against its schema (Format 13, 15.3).
5. The typed model is decoded from what remains.
6. JSON Lines sessions (Format 11), and packages (Format 12.3), whose `bundle.json` and sessions stream from their ZIP entries.

### 2. Checks

The goal is `check` tests passing, apart from codes that need later phases (`CEL_ERROR`, `REGEX_ERROR`, `UNKNOWN_MACRO`): duplicates, references, scenarios, locations and cycles, openings, entries, variable keys and values, messages and players. Locations follow conformance README section 3.4 exactly, including the order in which duplicates are reported.

### 3. State at a message

The goal is `state` tests passing (Format 8.6): initial values, keyed variables, normalization and clamping, the scene, and the cast and its joiners (Format 11.2).

### 4. Knowledge

The goal is `knows` tests passing: the witness rule, `knownBy`, the narrator in both modes, and hidden messages (Format 7.4, 11.5).

### 5. Macros

The goal is `macro` tests passing, plus `UNKNOWN_MACRO`. That covers parsing in one left-to-right scan, rendering rules, `{{char}}` and `{{user}}`, which state a macro reads, and picks (Format 9).

### 6. Regex keys and key matching

The goal is `REGEX_ERROR`, and matching ready for `hit()`. Regex keys get the grammar and limits of Format 14, translated to RE2 with ECMAScript's meaning. Plain keys get whole-word matching with simple case folding, and the scripts that match anywhere (Format 7.2).

### 7. Conditions

The goal is `cel` tests passing, plus `CEL_ERROR`. That covers the environment (Format 10.1), the rules (Format 10.2), and `hit()` over the scanned text. At the end of this phase the whole suite passes and the pending list is gone.

### 8. Beyond the suite

These are what the platform needs that the suite doesn't test. Build them in the order the platform asks for them, with tests of our own, since the suite has no cases for them:

- Entry activation: keys, `when`, timing from `activated` records, `group`, `priority` and `chance` (Format 7.2). An activation test type was deliberately left out of the suite for now.
- Making a root from an opening: picks chosen once and escaped, and the opening's `set` with the players added to `scene.present` (Format 11.1).
- Escaping a player's or a model's words written into session characters, worlds or entries (Format 9).
- Checking one proposed change, key and value, before the platform records it (Format 8.5).
- Reading a package's other files, such as its images, which the platform copies to its own storage on import.
- Writing bundles, sessions and packages back, keeping every `ext` and a package's other files (Format 12.3, 13).
- Upgrading documents from the previous minor version. This starts mattering once the platform stores documents.

## What the platform needs

The platform's API (its architecture is in `docs/stack.md` of the platform repository) uses the library to:

- **Import** a single file or a package that a person uploads, show its diagnostics with their locations, and store it as JSONB. Fields it queries come from the typed model: name, rating, tags.
- **Start a session** from an opening.
- **Prepare each generation:** the state at the evaluation point, the active entries, macros rendered for the character being written for, and what each character knows. A path can be 100,000 messages long, so the state at a message must be computable from its parent's state and that message's changes. Replaying the whole path on every turn won't do.
- **Record a turn**, after checking the changes a player or model proposes.
- **Export** a package with its images and sessions.

Prompt assembly, the choice of which active entries reach the model, model calls and storage stay in the platform.

## Pitfalls

Each of these needs verifying against the suite when its phase starts.

- **`encoding/json` (v1) is too lenient.** It accepts duplicate names, silently replaces invalid UTF-8, and decodes numbers to `float64`, which can't tell whether `9007199254740993` is within ±(2^53 − 1). Hence the strict pass in phase 1.
- **`jsontext` isn't all of I-JSON.** It accepts noncharacters, written or escaped, numbers too large for a double such as `1e400`, any depth up to its own limit, and a second value after the first. The strict pass checks each.
- **v2 is stricter than Format 2.2 about integers, and stops at the first type error.** It refuses `8.0` and `8e0` for an `int64`, which Format 2.2 says are integers, and leaves the value already in the field when the type is wrong. Hence the caller-supplied unmarshalers.
- **Unknown fields can't simply be passed to the validator.** Several schema objects set `additionalProperties: false`, so an unknown field would come out as `SCHEMA` as well as `UNKNOWN_FIELD`. Remove unknown fields first, then validate (Format 13). A part of an unknown `type` is only `SCHEMA`.
- **Schema references must not reach the network.** The message and header schemas share definitions in `bundle.schema.json`. Register all three with the compiler under their `$id`, so nothing is ever fetched.
- **Schema error locations need care.** For an `anyOf` that nothing accepts, the suite wants the deepest location where a branch fails, among other rules (conformance README section 3.4). The validator's raw output won't match without work.
- **ECMAScript and RE2 differ.** Under the `u` flag, ECMAScript's `.` excludes `\r`, U+2028 and U+2029, and RE2's doesn't. ECMAScript's `\s` is Unicode whitespace plus U+FEFF, and RE2's is ASCII only. The `m` flag recognizes different line terminators. With `i` and `u` together, ECMAScript's `\w` and `\b` also match U+017F and U+212A. The translation must produce ECMAScript's meaning.
- **CEL's defaults are wider than Format 10.2.** cel-go's standard environment has many more functions and macros, as well as `double`, `uint`, `bytes` and `null`. Reject them with AST validators after checking, along with any use of `state`, `own` or `scene` other than a selection, and selecting `char` or `user` from a keyed map. `has()` follows Format 10.2, which isn't quite CEL's presence test. Bound expression size (4 KiB) before parsing.
- **`archive/zip` doesn't check what Format 12.3 requires.** Check every rule yourself: absolute paths, `..`, backslashes, symlinks, encryption, compression methods, case-insensitive duplicates, entry count, and unpacked size counted while decompressing, since headers can lie.
- **Timestamps keep their offset.** `hour` and `weekday` are read at the offset written (Format 10.1). Don't convert to UTC or to local time.

## Spec feedback

When the code shows the spec is wrong or unclear, write it up for the user: what the spec says, the case or input where it matters, and a proposed fix. The user decides, and changes the spec repository. Add a conformance case only when apps could disagree on the issue, not for coverage. Then sync the copy here with `task spec`.

Reported, awaiting the spec:

1. **Which checks run after `PARSE` or `UNSUPPORTED_VERSION`.** Format 15.3 says every check runs on the document as written, so a 0.13 file would also be judged against the 0.14 schema, giving `SCHEMA` and `UNKNOWN_FIELD` that mean nothing for it. No case tells the readings apart. Proposed: "A document that fails `PARSE` or `UNSUPPORTED_VERSION` is reported with that code only", with a case of a 0.13 bundle that also fails the 0.14 schema.
2. **Integer fields outside ±(2^53 − 1).** Format 2.2 lists the range under I-JSON, which makes `"max": 9007199254740993` a `PARSE`. Enforcing that would put the schema's knowledge of which fields are integers into the I-JSON pass, and the schemas set no bound on fields such as `max`, `depth` or `maxStep`. Values of `int` variables are already `INVALID_VALUE` (Format 8.5). Proposed: bound every integer field in the schemas to ±9007199254740991, so it's `SCHEMA`, have Format 2.2 refer to that, and add a case.

3. **The suite depends on `examples/`.** Four cases (`cel/environment`, `examples/first-card`, `examples/card-with-lore`, `examples/lantern-heist`) read files from `../../../examples/`, outside `conformance/`, but conformance README section 1 describes the suite as `conformance/` alone. Proposed: say in section 1 that `bundleFile` and `sessionFile` may point into `examples/`, which a library copying the suite copies too, under its CC BY 4.0 license. No case needed.

These open items were deliberately left out of 0.14 and may come back as findings: label keys outside `values`, the loose `language` pattern, codes for nested `null`, and the missing activation test type.
