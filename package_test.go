package troupe_test

import (
	"bytes"
	"io"
	"reflect"
	"strconv"
	"testing"

	"github.com/TroupesAI/troupe-go"
)

// A package written reads back with its sessions and every other file,
// including one nothing names, which a writer keeps (Format 12.3).
func TestPackageWriter(t *testing.T) {
	files := []struct{ path, content string }{
		{"images/map.webp", "RIFF"},
		{"story.json", `{"troupe":"0.15","kind":"story","name":"Heist","cover":"images/map.webp"}`},
		{"sessions/a.troupe-session.jsonl", `{"troupe":"0.15","kind":"session","scenario":"s"}` + "\n"},
		{"x.y/notes.txt", "kept"},
	}
	var buf bytes.Buffer
	pw := troupe.NewPackageWriter(&buf)
	for _, f := range files {
		w, err := pw.Create(f.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, f.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := pw.Create("late.txt"); err == nil {
		t.Error("Create after Close: no error")
	}
	p, diags, err := troupe.ReadPackage(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil || p == nil || len(diags) > 0 {
		t.Fatalf("ReadPackage: %v, %v", diags, err)
	}
	if p.Story.Cover != "images/map.webp" || !reflect.DeepEqual(p.Sessions, []string{"sessions/a.troupe-session.jsonl"}) || !reflect.DeepEqual(p.Files, []string{"images/map.webp", "x.y/notes.txt"}) {
		t.Errorf("got the story %+v, sessions %v, files %v", p.Story, p.Sessions, p.Files)
	}
	if sessions, _, err := p.ReadSessions(); err != nil || sessions["sessions/a.troupe-session.jsonl"] == nil {
		t.Errorf("ReadSessions: %v, %v", sessions, err)
	}
	r, err := p.Open("x.y/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadAll(r); err != nil || string(got) != "kept" {
		t.Errorf("x.y/notes.txt holds %q, %v", got, err)
	}
}

// PackageWriter refuses what would make no package: a name that isn't a
// package path or that clashes with another file's, a package without
// story.json, and one past its limits (Format 12.3, 14).
func TestPackageWriterErrors(t *testing.T) {
	t.Parallel() // writing 512 MiB takes seconds under the race detector
	pw := troupe.NewPackageWriter(io.Discard)
	if _, err := pw.Create("a/../b"); err == nil {
		t.Error("Create of a name that isn't a package path: no error")
	}
	if _, err := pw.Create("Story.json"); err != nil {
		t.Fatal(err)
	}
	if err := pw.Close(); err == nil {
		t.Error("Close without story.json: no error")
	}

	for _, path := range []string{"Images/A/b.png", "images", "IMAGES/A", "images/a/b.png/c"} {
		pw := troupe.NewPackageWriter(io.Discard)
		for _, p := range []string{"story.json", "images/a/b.png", path} {
			if _, err := pw.Create(p); err != nil {
				t.Fatal(err)
			}
		}
		if err := pw.Close(); err == nil {
			t.Errorf("Close with %q beside images/a/b.png: no error", path)
		}
	}

	pw = troupe.NewPackageWriter(io.Discard)
	for i := range 10_000 {
		if _, err := pw.Create(strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pw.Create("story.json"); err == nil {
		t.Error("Create of a file past 10,000: no error")
	}

	pw = troupe.NewPackageWriter(io.Discard)
	w, err := pw.Create("story.json")
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for range 512 {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Write(chunk[:1]); err == nil {
		t.Error("Write past 512 MiB: no error")
	}
	if err := pw.Close(); err == nil {
		t.Error("Close past 512 MiB: no error")
	}
}
