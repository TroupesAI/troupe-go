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
| Copy a commit of the 0.14 draft until it is tagged | Waiting for a tag | Fixes found here land in the draft as commits, and `spec/REF` names the one copied |
| The drift check downloads `spec/REF` into a temporary directory and diffs it against `spec/` | Overwriting `spec/` and asking git | It never touches the working tree, and it catches added and removed files |
| Tags `v<spec version>.<patch>`, as `v0.14.2` | Versions of its own | The version shows which spec it reads. Revisit at spec 1.0: an API break during 1.x would then need a `/v2` module path |
| `go 1.27.0` in `go.mod` | `go 1.27.2` | It's the minimum for everyone who imports the module, and 1.27 is what `encoding/json/v2` needs. mise pins 1.27.2 for development |
| No clock interface | Passing in the current time | Every time a rule reads comes from a message's `at` (Format 10.1), so nothing reads the clock |
| `encoding/json/jsontext` for the strict pass, `encoding/json/v2` with `WithUnmarshalers` for the typed model | `encoding/json` v1, or a third-party decoder | Stable since Go 1.27. `jsontext` rejects duplicate names, invalid UTF-8 and unpaired surrogates by default, and `StackPointer` locates failures without document text. Caller-supplied unmarshalers handle `8.0` and wrong-typed values without changing exported types. The cost: a program built with `GOEXPERIMENT=nojsonv2` can't import the module |
| Bundles and sessions are read from an `io.Reader`, sessions line by line; packages from an `io.ReaderAt` and a size | `[]byte` | One shape across the readers. A 256 MiB session isn't held as bytes and as a model at once, a package's documents stream from its ZIP entries, and size limits are counted while reading. Readers return an `error` only when the reader fails, so an I/O failure is never reported as `PARSE` |
| [`cel-go`](https://github.com/google/cel-go) for conditions, now at the module path `cel.dev/cel-go` | Our own CEL | It's the reference implementation. Format 10.2 narrows it, and that's done with validators, not a fork |
| [`santhosh-tekuri/jsonschema`](https://github.com/santhosh-tekuri/jsonschema) v6 | Other validators | It supports JSON Schema 2020-12, asserts `format`, and reports instance locations, which diagnostics need |
| Schemas compiled once, on first use, with `sync.OnceValue` | At `init` | Programs that import the module but never read a document don't pay for it |
| Each value of the wrong type reads as absent, and a list holding an item of the wrong type reads as absent as a whole | Zero values, or dropping the item | Later checks see the document as written, and every other item keeps its index, so their diagnostics point at the right place. It hides diagnostics on the list's other items, such as an `UNKNOWN_CHARACTER` beside a wrong-typed `knownBy` item; revisit if a suite case needs them |
| The typed model mirrors the document: absence is the zero value, with pointers where 0 is a value and `*[]string` for `Scenario.Worlds`, whose `[]` means no worlds | Filling in defaults | Format 2.2 makes absence and the zero value the same nearly everywhere, and defaults belong to the operations that apply them |
| Plain strings for fields with a fixed set of values, such as `Rating` | Named string types with constants | Callers compare them with the spec's strings, and no behavior hangs on the type |
| `int64` for integers that can reach ±(2^53 − 1), `int` for those the schema bounds to small ranges | `int` everywhere | `int` is 32 bits on some platforms |
| `Keys` for `of`, and one `Part` struct for both kinds of part, holding only the fields its type defines | An interface, or raw JSON | Plain data is simplest to read, and an unknown field never reaches the model |
| `ext`, `initial` and the values in `set` are `jsontext.Value`, decoded from the bytes as written | Re-encoding the validator's tree | Writers keep every `ext` unchanged (Format 13), and the original bytes are the surest way |
| Unknown fields are the schema's `additionalProperties` failures, outside parts of an unknown type. The document is validated again, without them, only when there were any | A list of defined fields kept in Go | Every interface is an object with `additionalProperties: false` in the schemas, so the spec stays the one source |
| Limits and package paths are checked on the parsed tree, before the schema | On the typed model, after it | Format 14's limits come before the work they bound, and the schema walks what they count |
| The unpacked size is the sum of the entries' declared sizes, checked before decompressing | Counting while decompressing | `archive/zip` fails an entry whose data doesn't match its declared size, so the sum bounds the work and is known before any is done |
| A package with an entry that can't be read is `nil`, unless that entry is a session, which is left out | Leaving out any such entry | Sessions are documents of their own, and every other entry belongs to the archive |
| A ZIP name starting with a drive letter, such as `C:x`, is absolute | Only a leading `/` | Extracted on Windows, it escapes the target directory |
| A diagnostic about a ZIP entry names its file only when the name is a package path | Always naming it | Other names may hold anything, and diagnostics hold no document text |
| A message line that is I-JSON but not an object is `SCHEMA` | `PARSE` | Messages have no `kind`, and their schema requires an object |
| A document whose `kind` or version is wrong isn't checked against the limits | Checking every document against them | The limits are this version's, for that kind of document, and a document that fails `PARSE` or `UNSUPPORTED_VERSION` is reported with those codes only (Format 15.3) |
| The parsed tree holds each number as the nearest double | The number as written | A number means the nearest double (Format 2.2), so the schema judges `50.000000000000001` an integer |
| Diagnostics are sorted by file, line, pointer, with indices compared as numbers, and code, without duplicates | Document order | The validator's tree has no member order, and sorting makes the result deterministic |
| `Diagnostic.Location` gives the suite's form of the location | `Diagnostic.String` | `String` would be expected to describe the whole diagnostic, its code included |
| A `propertyNames` failure's location is rebuilt from its parent failure | The validator's own | jsonschema v6.0.3 keeps that location without copying it, so later validation overwrites it |
| `golang.org/x/text` required at v0.43.0 | v0.14.0, which jsonschema requires | v0.14.0 has known vulnerabilities, and the requirement keeps importers off it |
| `Check(b, s)` checks the bundle once for each of its scenarios, as written and without players, with a session as without one, then the session against its scenario | Checking a bundle that comes with a session only for the session's scenario | A bundle's validity doesn't depend on a session coming with it (conformance README section 2) |
| Checks that don't depend on a scenario run once per document, and the others once per scenario: the keys and values of openings, and of a session's changes, the openings roots name and activated entries | Every check per scenario | Repeating the rest would only repeat its diagnostics |
| A scope is a scenario as a session plays it, or as it is written: without a session, the characters in use are those its cast members name (Format 6.4), and the cast leaves out its seats (conformance README section 2) | One rule for both | The spec gives each its own; phase 3's state reuses the cast |
| Initial values are checked once, for every variable | Checking each variable in each scenario that uses it | A character's or world's variable may be in no scenario, and a `"cast"` initial member is checked whenever it names a character (Format 8.3) |
| A session's character or world that takes a scenario variable's ID is reported at the session's ID | At the variable's ID | The bundle is valid without the session (conformance README section 3.4) |
| `NewStory(b, s)` with `Initial`, `Apply` and `StateAt` | Only `StateAt` | The platform keeps each message's state and applies the next message's changes to it, so it never replays a path of up to 100,000 messages. A session given without messages serves `Apply` alone |
| `State` is immutable, and `Apply` returns a new one | Changing a state in place | A caller can keep any state, and states share what a change doesn't touch |
| `State.Get` takes the keys changes write (Format 8.5), and `All` yields them | Accessors per kind of value, or exported fields | One set of keys for writing and reading, and the suite's flattened form is exactly what `All` yields |
| Values are a `bool`, `int64`, `string` or `[]string` in an `any` | A value type of our own | They are the JSON types a state holds, and `Get` copies sets so callers can't change a state |
| `State` has no exported fields | Exported fields | It can gain serialization when the platform asks for it, without breaking callers |
| `NewStory` doesn't run `Check` | Running it | Callers check documents on import, not on every turn. For documents that fail `Check`, states are only bounded: invalid keys and values are skipped |
| The `"*"` map limit is judged after a whole change, and reported at each key of it that writes to the map | After each key | A change's keys have no order, so one that removes a key and adds another can't be judged partway |
| `Check` walks a session's paths with one state, undoing each message's changes on the way back up | A state per message | Memory stays the size of one state, however long the paths |
| `Story.KnownBy(m, before)` returns who knows each part of a message, from the state before it | `KnownBy(message, part)` by ID | The platform keeps states, not whole sessions, and one call per message applies its changes once for all its parts |
| `Story.Render(text, Context, r)`, where `Context` holds the state, the character `{{char}}` means or the narrator, and the owner `own` means | Separate arguments, or one function per kind of text | The caller knows where author text is used; one struct can grow without breaking calls |
| The player's character `{{user}}` means is part of `State`, and `Apply` keeps it | The caller working it out from the path | Like the state, it depends on the path to a message, so states the platform keeps carry it |
| A pick's option comes from `r`, a `math/rand/v2` source every call to `Render` and `RenderParts` takes | An optional field in `Context` | Forgetting the source doesn't compile, and rendering stays deterministic, as the README promises |
| `Story.RenderParts(m, at, r)` returns a message's parts as shown: resolved in a root made from an opening, as written in a player or model message, and `Render` is for author text | Leaving it to callers to use player and model text as written | A caller who forgot would let state leak through model output or prompt attacks (Format Appendix C) |
| Macros are parsed in `internal/macro`, and one resolver decides which variable a path names, for `Render` and for `UNKNOWN_MACRO` | Each doing its own | Rendering and checking can't disagree on what's declared |
| Regex keys are parsed by recursive descent over Format 14's grammar, which yields the translation to RE2 and the size written out in the same pass | Checking the grammar and translating separately | One walk over the pattern decides validity, the size limit and the meaning |
| Under `i`, a key matches the text with ſ and K (Kelvin) as s and k | Lookarounds RE2 doesn't have | ECMAScript's `\w` and `\b` count them as word characters under `i`. Every set a pattern matches under `i` holds a character's case variants with it, so the mapping changes no other match |
| `\P{…}` under `i` is an explicit class, computed on first use for each category | RE2's `(?i)\P{…}` | RE2 folds the category before taking its complement, and ECMAScript after |
| Key matching is in `internal/keys`, for `hit()` and entry activation | In package `troupe` | It has no caller until phase 7, and both later callers use it |
| Tests of regex keys' meaning take their expected results from V8, on text whose line breaks are `\n` | Results worked out by hand | ECMAScript's meaning is what the spec requires, and V8 is a reference for it |
| `keys.Text` writes every line break as `\n`, and the scanned text is converted once, as it is built | Converting in each match, or translating `.`, `^` and `$` | Format 7.2 matches keys against text whose line breaks are `\n`, where RE2 reads `.`, `^` and `$` as ECMAScript does |
| `Story.PointAt(before, recent, char)` builds an evaluation point from the path's last messages and the state before them, and `Point.Eval(expr, self, scanDepth)` evaluates a condition there | `Eval` taking a message ID, or the whole path | The platform keeps states, so it passes only the messages a scan reaches; building the point once does the work every condition there shares |
| The rounds, the path's opening, and the times of a message and its parent are part of `State`, kept by `Apply`, and aren't its keys | Working them out from the path | Like `{{user}}`, they depend on the path, so states the platform keeps carry them |
| `Eval` returns an error only for a `CEL_ERROR`, and a runtime error is `false` | An error for runtime errors too | Format 10.2 makes a runtime error a false condition |
| Variables are declared with qualified names, such as `state.bell` and `own.nerve` | `state` as a map or an object | `state` and `scene` alone, or indexed by a computed string, don't type-check, as Format 10.2 requires, and undeclared variables fail the type check |
| CEL's standard macros are cleared; `has()` expands to a non-strict check of its selection, and `hit()` to a look-up whose results the point computes for the expression's literal keys | The standard `has()`, and a `hit()` function | Format 10.2's `has()` is true exactly when selecting isn't a runtime error, and `hit()`'s keys are literals checked when the expression compiles |
| An AST validator rejects other functions, `double`, `uint`, `bytes` and `null` literals, type names as values, and `char` or `user` selected from a keyed map | Removing them from the environment | cel-go's standard library is all or nothing, and these rules read the checked AST |
| Conditions compile once per story, owner and expression, and `Check` builds an environment once per scenario and owner | Compiling for each evaluation | A platform evaluates every entry's condition on every turn |
| `internal/condition` | `internal/cel` | Its name doesn't clash with cel-go's `cel` package |
| A message's own session entries apply to its `activated` | Applying from the next message | Format 11.3: they apply "from that message onward" |
| A change key that doesn't follow the key grammar is `UNKNOWN_VARIABLE` as well as `SCHEMA` | Leaving it to the schema | Every check runs on the document as written (Format 15.3) |
| With an unknown scenario, the session is checked against an empty one | Skipping the checks that need it | The rest still holds; only what the scenario would make valid fails |
| A regex parser of our own, emitting RE2 | Compiling keys directly with `regexp` | Format 14 defines a grammar that has to be validated exactly, and keys mean what they mean in ECMAScript, which differs from RE2 in places |
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
internal/keys/      key matching: plain keys and regex keys (Format 7.2)
internal/condition/ conditions: the environment and the Format 10.2 rules
internal/macro/     macro parsing (Format 9)
spec/               copied from the spec repository; never edited by hand
  REF               the spec commit the copy comes from
  schemas/          embedded, and compiled on first use
  conformance/      the suite: README, case.schema.json, cases/
  examples/         the spec's worked examples, which some cases read
conformance_test.go package troupe_test: the runner
```

## The public API

The API has to do two things: everything the conformance runner asks for, through exported names only, and everything the platform needs (see [What the platform needs](#what-the-platform-needs)). Everything the suite tests is built: `ReadBundle`, `ReadSession`, `ReadPackage`, the typed model, `Diagnostic`, `Check`, `NewStory`, `Story`, `State`, `Story.KnownBy`, `Story.Render`, `Story.RenderParts`, `Story.PointAt` and `Point.Eval`.

The suite's operations map onto it like this (conformance README section 3):

| Test | Uses |
| --- | --- |
| `parse` | `ReadBundle`, `ReadSession`, `ReadPackage`, at the session-reader level |
| `check` | `Check`, once per scenario when there's no session |
| `state` | `StateAt`, flattened by `State.All` |
| `knows` | `KnownBy`, from the parent's `StateAt` |
| `macro` | `Render`, with `char`, `owner` and `narrator` in its `Context`, and the state at `at` |
| `cel` | `Point.Eval`, at the point `PointAt` builds from the path to `at`, with `char`, `self` and `scanDepth`; an error is a `CEL_ERROR` |

A case whose `bundleFile` is a package's `bundle.json` is read by zipping its directory in memory and calling `ReadPackage`, since package paths are `PARSE` anywhere else.

## Phases

Each phase ends with its part of the work tested, lint clean, and its decisions recorded above. The whole conformance suite passes.

### 8. Beyond the suite

These are what the platform needs that the suite doesn't test. Build them in the order the platform asks for them, with tests of our own, since the suite has no cases for them:

- Entry activation: keys, `when`, timing from `activated` records, `group`, `priority` and `chance` (Format 7.2). An activation test type was deliberately left out of the suite for now.
- Making a root from an opening: picks chosen once and escaped, and the opening's `set` with the players added to `scene.present` (Format 11.1).
- Escaping a player's or a model's words written into session characters, worlds or entries (Format 9).
- Checking one proposed change, key and value, before the platform records it (Format 8.5).
- Storing a `State` and restoring it, for a platform that keeps each message's state.
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

## Spec feedback

When the code shows the spec is wrong or unclear, write it up for the user: what the spec says, the case or input where it matters, and a proposed fix. The user decides, and changes the spec repository. Add a conformance case only when apps could disagree on the issue, not for coverage. Then sync the copy here with `task spec`.

These open items were deliberately left out of 0.14 and may come back as findings: label keys outside `values`, the loose `language` pattern, codes for nested `null`, and the missing activation test type.
