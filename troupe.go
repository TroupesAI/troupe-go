// Package troupe reads files in the Troupe format, checks them, and computes
// what they mean, as the Troupe specification defines.
//
// ReadBundle, ReadSession and ReadPackage read the three forms of a file,
// Package.ReadSessions reads a package's sessions, and Check runs the checks
// reading leaves. A document with any diagnostic doesn't conform, and serves
// only to report them. Package.Open streams a package's other files, such as
// its images, and ReadPackageBundle reads its bundle.json stored apart from
// it. Bundle.FileFields yields the fields that
// hold a file, and ReplaceFileFields sets them in a document's JSON, keeping
// every other byte.
//
// For an editor, Bundle.CheckID checks the ID of an object being added, and
// Bundle.RenameID makes the JSON Patch that renames one with every reference
// to it.
//
// NewStory makes the story a session plays, and NewScenarioStory a scenario
// played on its own. A story gives its cast, the worlds in use, its mode,
// what it calls the narrator, the keys its changes can write, and its states:
// Initial, StateAt for a message, and Apply, which gives a message's state
// from its parent's, so a caller that keeps each message's state never
// replays a path. A State marshals to JSON, and UnmarshalState restores it.
// From a message's parent's state, KnownBy tells who knows each of its parts
// and RenderParts shows them. Render resolves the macros in author text, and
// PointAt builds the point where the next message is written, where
// Point.Eval evaluates conditions and Point.Active gives the entries active.
// Root makes the root a session starts with from an opening, and Escape
// writes a player's or a model's words where macros resolve.
//
// The typed model mirrors documents as written (Format 2.2). An absent field
// is its zero value, or nil where the zero value means something, and the
// spec's defaults are left to the operations that apply them; field comments
// give them. Fields hold strings where the spec has a fixed set of values. In
// a document that fails the schema, a value of the wrong type reads as
// absent, and so does a list holding an item of the wrong type. Marshaled to
// JSON, the model is written as the spec writes it, so a caller can write the
// messages and characters it makes, and a conforming document marshals as the
// same JSON.
//
// The package does no I/O, and is deterministic: randomness comes from a
// source the caller passes. Stories, states and points are safe for
// concurrent use, and the package only reads the documents it is given, so
// a bundle or session read once can serve any number of stories, checks and
// calls at once.
package troupe

// Version is the Troupe version this module reads.
const Version = "0.14"
