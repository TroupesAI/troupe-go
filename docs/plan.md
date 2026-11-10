# Plan

What troupe-go still has to build, and what the platform needs from it, which the API is judged against. Once an item is built, delete it: the code, its tests and its doc comments then say what it does, and the commit body says why.

## Still to build

What remains is what the platform needs and the suite doesn't test. Build each item when the platform asks for it, with tests of our own:

- Writing sessions and packages back, keeping every `ext` and a package's other files (Format 12.3, 13), with a session's file fields, so packages with sessions can be imported and exported. A session's file fields are its lines': image parts, and the header's avatars and location images. Each line is one JSON value, so `SessionHeader` and `Message` can each give theirs as `Story.FileFields` does, and `ReplaceFileFields` already replaces them.
- Revisit how states hold session entries once the platform adds them, or if stored states grow too large. Every stored state holds the entries' text, up to 10,000 (Format 14), and `Apply` copies them for each message that adds, replaces or removes one, so replaying 100,000 such messages over 10,000 entries takes seconds.

## What the platform needs

The platform's API (its architecture is in `docs/stack.md` of the platform repository) uses the library to:

- **Import** a single file or a package that a person uploads, show its diagnostics with their locations, and store it as `json`. Fields it queries come from the typed model: name, rating, tags.
- **Start a session** from an opening.
- **Prepare each generation:** the state at the evaluation point, the active entries, macros rendered for the character being written for, and what each character knows. A path can be 100,000 messages long, so the state at a message must be computable from its parent's state and that message's changes. Replaying the whole path for every message won't do.
- **Record a message**, after checking the changes a player or model proposes.
- **Show the state** at a message to the people playing: each state key it holds, with its variable and its value.
- **Edit** a stored story.json as people type, though a draft may not conform yet: check the ID of each object added, rename an ID with every reference to it, and offer the state keys an opening's `set` can write, each with its variable. The platform applies the editor's JSON Patches itself, then reads and checks the result.
- **Export** a package with its images and sessions.
