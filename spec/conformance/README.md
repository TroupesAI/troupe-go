# Troupe conformance cases

This folder holds the conformance suite for the Troupe spec. Each case is a language-neutral JSON file in `cases/`. Any implementation can run the suite by reading each case, setting up its inputs, performing each test's operation, and comparing the result with `expect`. Libraries live in their own repositories and run it as part of their tests.

This document is the contract between the cases and a runner. When a case and this document disagree, report it as a bug in the suite.

## 1. Case files

Cases live in `cases/<area>/<topic>.json`, one folder per area of the spec:

| Area | Covers |
| --- | --- |
| `state` | State at a message, keyed variables, normalization and rounds (Format 8, 11.1) |
| `macros` | Macro parsing and rendering (Format 9) |
| `cel` | Conditions (Format 10) |
| `knowledge` | Who knows a part (Format 7.4, 11.5) |
| `roots` | Roots made from openings (Format 6.3, 11.1) |
| `sessions` | Session lines, the head, players and the cast (Format 11) |
| `checks` | Checks and unknown fields (Format 13, 15.3) |
| `parsing` | Raw files (Format 2.2, 11, 12, 13, 14) |
| `examples` | The spec's worked examples |

The order of cases doesn't matter. A case file is one JSON object, described by [`case.schema.json`](case.schema.json), which `bun run check` applies:

| Field | Type | Meaning |
| --- | --- | --- |
| `description` | string | What the case covers, with spec sections. Informative. |
| `note` | string | Optional. How an expected value was derived, such as how a value is clamped. Informative. |
| `bundle` | object | The bundle, as a JSON document. |
| `bundleFile` | string | Instead of `bundle`: a path to a bundle JSON file, relative to the case file. A `bundle.json` is a package's (Format 12.3), and a `sessionFile` beside it in `sessions/` is one of that package's sessions. |
| `session` | object | Optional. `{"header": SessionHeader, "messages": [Message, …]}`. `messages` lists the session's lines after the header, in file order (Format 11). |
| `sessionFile` | string | Instead of `session`: a path to a `.jsonl` session file, relative to the case file. |
| `tests` | array | The tests (section 3). |

`bundleFile` and `sessionFile` may point into the repository's `examples/` folder, outside `conformance/`, so a library that copies the suite copies `examples/` with it, under its [CC BY 4.0](../spec/LICENSE) license.

Bundles and session headers given as objects leave out `troupe`: before anything else, `check` validation included, the runner sets it to the spec version. If no bundle is given, the bundle is `{"troupe": <the spec version>, "kind": "bundle"}`. Cases that test raw input instead use `parse` tests (section 3.6), which take their input inline.

## 2. Setup for each test

Every test starts from a fresh copy of the case's inputs. Tests don't affect each other.

- **Levels.** An implementation that reads only bundles (Format 15.2) skips the tests of reading sessions: `parse` tests of `kind: "session"`, `check` tests of cases with a `session` or `sessionFile`, and `parse` tests marked `"level": "session"`. Other tests use a case's session only as the chat they run on.
- **Conforming inputs.** `state`, `macro`, `cel` and `knows` tests run only on inputs that conform (Format 15.1), since the spec gives files that fail a check no meaning to test. Every case with such tests also has a `check` test that expects no codes, which a runner that doesn't skip it uses to confirm them.
- **The story.** The session's `scenario` names the scenario, and its header's `players` lists every player (Format 11.2). Without a session, use the bundle's first scenario, whose seats, those marked `player` included, are then unfilled and left out of the cast (Format 6.1), so there are no players. A `check` test checks the bundle once for each scenario (Format 6.4, 10.2), with a session as without one, and then checks the session against its scenario, or as Format 15.3 says when it names none.

## 3. Tests

Each test has `type`, an optional `name` and `note` (informative), the inputs below, and `expect`. When `expect` is an object, its fields must be ones this section defines for the type; a runner should treat any other field as a suite error. Results are compared this way:

- **Equality** is deep JSON equality. Object key order is ignored, array order matters, and numbers compare as IEEE 754 doubles.
- **Subset** comparisons check only the keys `expect` lists; other keys in the actual result are ignored.
- Lists are compared in the order the spec defines, such as cast order.

### 3.1 `state`: state at a message (Format 8.6)

| Input | Meaning |
| --- | --- |
| `at` | A message ID. Omit it for the initial state, before any message. |
| `absent` | Optional. Flattened keys that must not be present. |

The state is flattened to an object with these keys:

- `scene.location` and `scene.present`
- every variable in use by its full name (Format 8.2): `state.bell`, `state.kai.nerve`
- one key per map key for keyed variables, such as `state.trust.kai` or `state.kai.feelings.elena`. Absent keys of `"*"` maps aren't listed.

`expect` is a subset of the flattened state.

### 3.2 `macro`: macro resolution (Format 9)

| Input | Meaning |
| --- | --- |
| `text` | Author text. |
| `part` | Instead of `text` and `at`: `{message, index}`, the `text` of that part of that message as it is shown: resolved at that message in a root made from an opening, and as written in a player or model message (Format 9). |
| `at` | The message whose state the macros read. Omit it for the initial state, where `{{user}}` is the first player's character. `{{user}}` follows the path to `at`. |
| `char` | What `{{char}}` and a `char` path segment mean. Omit it for none: `{{char}}` is then left as written, and a `char` segment names no key (Format 9). |
| `owner` | What `own` means. Omit it for none. |
| `narrator` | Optional boolean: the text is used for a message written for the narrator, so `{{char}}` without `char` renders as the scenario's `narrator`. |

`expect` is the rendered string (Format 9).

### 3.3 `cel`: conditions (Format 10)

| Input | Meaning |
| --- | --- |
| `expr` | A CEL expression. |
| `at` | The evaluation point (Format 10.1). Omit it for none: `turn` 0, the initial state, `opening` `""`, `elapsed` 0, `hour` and `weekday` −1, and `user` the first player's character, or `""`. |
| `char` | The value of `char`; `""` by default. |
| `self` | The expression's owner, a character, world or scenario ID or `session`, which also types `own`; the scenario by default. |
| `scanDepth` | The `scanDepth` of the entry `hit()` scans for; 4 by default. |

`players` is as section 2 says. `hit()` checks the scanned text (Format 7.2) of a message written as a child of `at`, for `char`, or for the narrator when `char` is `""`; without `at` nothing is scanned, so it is false. If `expect` is a boolean, the expression must type-check and evaluate to it; a runtime error counts as `false`. If `expect` is `{"checkError": true}`, the expression is a `CEL_ERROR` (Format 10.2).

### 3.4 `check`: checks (Format 15.3)

The runner validates the bundle and the session's lines against the schemas, then runs every other check (Format 15.3). Every check runs on the documents as written, so one value can fail several checks (Format 15.3).

- `expect.codes` is the sorted list of distinct diagnostic codes reported. It is compared exactly.
- Each session line after the header is validated against `message.schema.json`. Validators assert `format`, so a malformed timestamp is `SCHEMA`.
- `SCHEMA` covers schema failures, such as `null`, `""` or `[]` in an optional field (Format 2.2).
- `check` tests don't use `PARSE` or `UNSUPPORTED_VERSION`; `parse` tests (section 3.6) cover them.
- `expect.includes` lists `{code, at}` pairs that must each appear, compared on both fields.

Locations:

| Location | Form | Example |
| --- | --- | --- |
| In the bundle | A JSON Pointer, with array indices | `/scenarios/0/entries/1/when` |
| In a session header | `header/<pointer>` | `header/players` |
| In a message (its schema failures included) | `message:<id>/<pointer within the message>` | `message:3/changes/0/set/state.hp` |

A diagnostic about one value points at that value: a list item at its index (`/scenarios/0/worlds/1`), a reference at the field holding it (`/scenarios/0/cast/2/character`), an unknown property at the property (`/characters/0/mystery`), and a location cycle at the `parent` of each location on it. For keys inside `set`, the pointer segment is the key as written (`/set/scene.present`). A schema failure is at the instance location of the keyword that fails: a missing property, or a combination forbidden with `not`, at its object (`/scenarios/0/entries/1`); a property forbidden for its type at the property; a duplicate list item at the list; and a value that no branch of an `anyOf` accepts at the deepest location where a branch fails, so an `of` list item that isn't a key is at its index.

A duplicate ID is at the `id` of each one after the first (`/characters/1/id`), in this order: the bundle's characters, worlds and scenarios, then the session's characters and worlds; locations in world order, the bundle's before the session's. A scenario variable with the ID of a bundle character or world is at the variable's `id`, and a session character or world with a scenario variable's ID at its own `id`, and a character or world a scenario names twice is at its later `character` or list item.

### 3.5 `knows`: who knows a part (Format 7.4, 11.5)

| Input | Meaning |
| --- | --- |
| `message` | A message ID. |
| `part` | The index of one of its parts. |

`expect` is who knows it: the IDs of the characters who know it, and `narrator` when the narrator knows it, in code point order.

### 3.6 `parse`: raw input (Format 2.2, 11, 12, 13, 14, 15.2)

| Input | Meaning |
| --- | --- |
| `kind` | What the input is: `bundle` (a `.troupe.json` file), `session` or `package` (a `.troupe` ZIP archive). |
| `text` | The input as a string, encoded as UTF-8 to get its bytes. |
| `base64` | Instead of `text`: the input's bytes, in base64. Used for bytes that aren't valid UTF-8 text and for binary formats. |
| `level` | Optional. `"session"` when `expect` depends on a package's sessions, which a bundle reader may leave unread (Format 15.2). |

The runner parses the bytes, checks the version and, unless the input is `PARSE` or `UNSUPPORTED_VERSION` (Format 15.3), validates against the schema, with unknown properties as `UNKNOWN_FIELD` (Format 13). For a package it checks the archive and its `bundle.json` and, for a session reader, every session in it (Format 12.3, 15.2). It runs no other checks; `check` tests cover them.

`expect.codes` is the sorted list of distinct codes reported, compared exactly. A conforming input has none.

## 4. Numbers and text

- Numbers in `expect` are JSON integers. Compare them as doubles after parsing, so `8` and `8.0` are equal (Format 2.2).
- Strings are compared code point by code point, after JSON unescaping.
- No normalization is applied to expected strings.

## 5. Adding cases

- Put a case in the folder for its area, named after its topic: `state/clamping.json`.
- Prefer small inline bundles.
- Test failed checks in separate cases, with `check` and `parse` tests; other tests need conforming inputs (section 2).
- Keep inputs to what the tests read.
- When an expectation depends on ambiguous spec text, fix the spec text.
- Cite current sections in `description`, as `Format 7.2`.
