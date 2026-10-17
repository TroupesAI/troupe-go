// Package troupe reads files in the Troupe format, checks them, and computes
// what they mean, as the Troupe specification defines.
//
// ReadBundle, ReadSession and ReadPackage read the three forms of a file, and
// Check runs the checks reading leaves. A document with any diagnostic
// doesn't conform, and serves only to report them.
//
// NewStory makes the story a session plays, and gives its states: Initial,
// StateAt for a message, and Apply, which gives a message's state from its
// parent's, so a caller that keeps each message's state never replays a path.
// From a message's parent's state, KnownBy tells who knows each of its parts
// and RenderParts shows them. Render resolves the macros in author text, and
// PointAt builds the point where conditions are evaluated, by Point.Eval.
//
// The typed model mirrors documents as written (Format 2.2). An absent field
// is its zero value, or nil where the zero value means something, and the
// spec's defaults are left to the operations that apply them; field comments
// give them. Fields hold strings where the spec has a fixed set of values. In
// a document that fails the schema, a value of the wrong type reads as
// absent, and so does a list holding an item of the wrong type. The model is
// for reading: to store or send a document, keep its bytes.
//
// The package does no I/O, and is deterministic: randomness comes from a
// source the caller passes. Stories, states and points are safe for
// concurrent use.
package troupe

// Version is the Troupe version this module reads.
const Version = "0.14"
