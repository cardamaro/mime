package test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cardamaro/mime"
)

// Syntatic sugar for Part comparisons
var PartExists = &mime.Part{}

// OpenTestData is a utility function to open a file in testdata for reading, it will panic if there
// is an error.
func OpenTestData(subdir, filename string) io.Reader {
	// Open test part for parsing
	raw, err := os.Open(filepath.Join("testdata", subdir, filename))
	if err != nil {
		// err already contains full path to file
		panic(err)
	}
	return raw
}

// ComparePart test helper compares the attributes of two parts, returning true if they are equal.
// t.Errorf() will be called for each field that is not equal.  The presence of child and siblings
// will be checked, but not the attributes of them.  Header, Errors and unexported fields are
// ignored.
func ComparePart(t *testing.T, got *mime.Part, want *mime.Part) (equal bool) {
	t.Helper()
	if got == nil && want != nil {
		t.Error("Part == nil, want not nil")
		return
	}
	if got != nil && want == nil {
		t.Error("Part != nil, want nil")
		return
	}
	equal = true
	if got == nil && want == nil {
		return
	}
	if (got.Parent == nil) != (want.Parent == nil) {
		equal = false
		gs := "nil"
		ws := "nil"
		if got.Parent != nil {
			gs = "present"
		}
		if want.Parent != nil {
			ws = "present"
		}
		t.Errorf("Part.Parent == %q, want: %q", gs, ws)
	}
	/*
		if (got.FirstChild == nil) != (want.FirstChild == nil) {
			equal = false
			gs := "nil"
			ws := "nil"
			if got.FirstChild != nil {
				gs = "present"
			}
			if want.FirstChild != nil {
				ws = "present"
			}
			t.Errorf("Part.FirstChild == %q, want: %q", gs, ws)
		}
		if (got.NextSibling == nil) != (want.NextSibling == nil) {
			equal = false
			gs := "nil"
			ws := "nil"
			if got.NextSibling != nil {
				gs = "present"
			}
			if want.NextSibling != nil {
				ws = "present"
			}
			t.Errorf("Part.NextSibling == %q, want: %q", gs, ws)
		}
	*/
	if got.ContentType != want.ContentType {
		equal = false
		t.Errorf("Part.ContentType == %q, want: %q", got.ContentType, want.ContentType)
	}
	if got.Disposition != want.Disposition {
		equal = false
		t.Errorf("Part.Disposition == %q, want: %q", got.Disposition, want.Disposition)
	}
	if got.Filename != want.Filename {
		equal = false
		t.Errorf("Part.Filename == %q, want: %q", got.Filename, want.Filename)
	}
	if got.Charset != want.Charset {
		equal = false
		t.Errorf("Part.Charset == %q, want: %q", got.Charset, want.Charset)
	}
	if got.Descriptor != want.Descriptor {
		equal = false
		t.Errorf("Part.Descriptor == %q, want: %q", got.Descriptor, want.Descriptor)
	}

	return
}

// TestHelperComparePartsEqual tests compareParts with equalivent Parts
func TestHelperComparePartsEqual(t *testing.T) {
	testCases := []struct {
		name string
		part *mime.Part
	}{
		{"nil", nil},
		{"empty", &mime.Part{}},
		{"Parent", &mime.Part{Parent: &mime.Part{}}},
		//{"FirstChild", &mime.Part{FirstChild: &mime.Part{}}},
		//{"NextSibling", &mime.Part{NextSibling: &mime.Part{}}},
		{"ContentType", &mime.Part{ContentType: "such/wow"}},
		{"Disposition", &mime.Part{Disposition: "irritable"}},
		{"Filename", &mime.Part{Filename: "readme.txt"}},
		{"Charset", &mime.Part{Charset: "utf-7.999"}},
		{"Descriptor", &mime.Part{Descriptor: "0.1"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockt := &testing.T{}
			if !ComparePart(mockt, tc.part, tc.part) {
				t.Errorf("Got false while comparing a Part %v to itself: %+v", tc.name, tc.part)
			}
			if mockt.Failed() {
				t.Errorf("Helper failed test for %q, should have been successful", tc.name)
			}
		})
	}
}

// TestHelperComparePartsInequal tests compareParts with differing Parts
func TestHelperComparePartsInequal(t *testing.T) {
	testCases := []struct {
		name string
		a, b *mime.Part
	}{
		{
			name: "nil",
			a:    nil,
			b:    &mime.Part{},
		},
		{
			name: "Parent",
			a:    &mime.Part{},
			b:    &mime.Part{Parent: &mime.Part{}},
		},
		{
			name: "ContentType",
			a:    &mime.Part{ContentType: "text/plain"},
			b:    &mime.Part{ContentType: "text/html"},
		},
		{
			name: "Disposition",
			a:    &mime.Part{Disposition: "happy"},
			b:    &mime.Part{Disposition: "sad"},
		},
		{
			name: "Filename",
			a:    &mime.Part{Filename: "foo.gif"},
			b:    &mime.Part{Filename: "bar.jpg"},
		},
		{
			name: "Charset",
			a:    &mime.Part{Charset: "foo"},
			b:    &mime.Part{Charset: "bar"},
		},
		{
			name: "Descriptor",
			a:    &mime.Part{Descriptor: "0"},
			b:    &mime.Part{Descriptor: "1.1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockt := &testing.T{}
			if ComparePart(mockt, tc.a, tc.b) {
				t.Errorf(
					"Got true while comparing inequal Parts (%v):\n"+
						"Part A: %+v\nPart B: %+v", tc.name, tc.a, tc.b)
			}
			if tc.name != "" && !mockt.Failed() {
				t.Errorf("Mock test succeeded for %s, should have failed", tc.name)
			}
		})
	}
}

// ContentContainsString checks if the provided readers content contains the specified substring
func ContentContainsString(t *testing.T, b []byte, substr string) {
	t.Helper()
	got := string(b)
	if !strings.Contains(got, substr) {
		t.Errorf("content == %q, should contain: %q", got, substr)
	}
}

// ContentEqualsString checks if the provided readers content is the specified string
func ContentEqualsString(t *testing.T, b []byte, str string) {
	t.Helper()
	got := string(b)
	if got != str {
		t.Errorf("content == %q, want: %q", got, str)
	}
}

// ContentEqualsBytes checks if the provided readers content is the specified []byte
func ContentEqualsBytes(t *testing.T, b []byte, want []byte) {
	t.Helper()
	if !bytes.Equal(b, want) {
		t.Errorf("content:\n%v, want:\n%v", b, want)
	}
}
