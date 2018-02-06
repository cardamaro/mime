package mime_test

import (
	"testing"

	"github.com/cardamaro/mime"
	"github.com/cardamaro/mime/internal/test"
)

func TestPlainTextPart(t *testing.T) {
	var want, got string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "textplain.raw")
	p, err := mime.ReadParts(r)
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		ContentType: "text/plain",
		Charset:     "us-ascii",
	}
	test.ComparePart(t, p, wantp)

	want = "7bit"
	got = p.Header.Get("Content-Transfer-Encoding")
	if got != want {
		t.Errorf("Content-Transfer-Encoding got: %q, want: %q", got, want)
	}

	want = "Test of text/plain section\r\n"
	test.ContentEqualsString(t, p, want)
}

func TestQuotedPrintablePart(t *testing.T) {
	var want, got string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "quoted-printable.raw")
	p, err := mime.ReadParts(r)
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		ContentType: "text/plain",
		Charset:     "us-ascii",
	}
	test.ComparePart(t, p, wantp)

	want = "quoted-printable"
	got = p.Header.Get("Content-Transfer-Encoding")
	if got != want {
		t.Errorf("Content-Transfer-Encoding got: %q, want: %q", got, want)
	}

	d, err := p.Decode()
	if err != nil {
		t.Error(err)
	}

	want = "Start=ABC=Finish"
	test.ContentEqualsString(t, d, want)
}

func TestQuotedPrintableInvalidPart(t *testing.T) {
	var want, got string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "quoted-printable-invalid.raw")
	p, err := mime.ReadParts(r)
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		ContentType: "text/plain",
		Charset:     "utf-8",
		Disposition: "inline",
	}
	test.ComparePart(t, p, wantp)

	want = "quoted-printable"
	got = p.Header.Get("Content-Transfer-Encoding")
	if got != want {
		t.Errorf("Content-Transfer-Encoding got: %q, want: %q", got, want)
	}

	d, err := p.Decode()
	if err != nil {
		t.Error(err)
	}

	want = "Stuffs’s Weekly Summary"
	test.ContentContainsString(t, d, want)
}

func TestSingleRfc822(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "singlerfc822.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists},
		ContentType: "message/rfc822",
		Descriptor:  "1",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "Hello world\n"
	test.ContentEqualsString(t, p1, want)
}

func TestMultiRfc822(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "multirfc822.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/mixed",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/x-myown",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "hello"
	test.ContentContainsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		Subparts:    []*mime.Part{test.PartExists},
		ContentType: "message/rfc822",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "Sub MIME prologue"
	test.ContentContainsString(t, p2, want)

	// Message attachment
	p3 := p2.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "2.0",
	}
	test.ComparePart(t, p3, wantp)

	// Message attachment
	p4 := p3.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Descriptor:  "2.1",
	}
	test.ComparePart(t, p4, wantp)

	want = "<p>Hello world</p>"
	test.ContentContainsString(t, p4, want)

	// Message attachment
	p5 := p3.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Descriptor:  "2.2",
	}
	test.ComparePart(t, p5, wantp)

	want = "Hello another world"
	test.ContentContainsString(t, p5, want)
}

func TestMultiAlternParts(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "multialtern.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "An HTML section"
	test.ContentEqualsString(t, p2, want)
}

// TestRootMissingContentType expects a default content type to be set for the root if not specified
func TestRootMissingContentType(t *testing.T) {
	var want string
	r := test.OpenTestData("parts", "missing-ctype-root.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}
	want = "text/plain"
	if p.ContentType != want {
		t.Errorf("Content-Type got: %q, want: %q", p.ContentType, want)
	}
	want = "us-ascii"
	if p.Charset != want {
		t.Errorf("Charset got: %q, want: %q", p.Charset, want)
	}
}

func TestPartMissingContentType(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "missing-ctype.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		Charset:     "us-ascii",
		ContentType: "text/plain",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "An HTML section"
	test.ContentEqualsString(t, p2, want)
}

func TestPartEmptyHeader(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "empty-header.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]

	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "An HTML section"
	test.ContentEqualsString(t, p2, want)
}

func TestMultiMixedParts(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "multimixed.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/mixed",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "Section one"
	test.ContentContainsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "Section two"
	test.ContentContainsString(t, p2, want)
}

func TestMultiOtherParts(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "multiother.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/x-enmime",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "Section one"
	test.ContentContainsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "Section two"
	test.ContentContainsString(t, p2, want)
}

func TestNestedAlternParts(t *testing.T) {
	var wantp *mime.Part
	r := test.OpenTestData("parts", "nestedmulti.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	test.ContentEqualsString(t, p1, "A text section")

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent: test.PartExists,
		Subparts: []*mime.Part{
			test.PartExists, test.PartExists, test.PartExists},
		ContentType: "multipart/related",
		Descriptor:  "2.0",
	}
	test.ComparePart(t, p2, wantp)

	// First nested
	p3 := p2.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2.1",
	}
	test.ComparePart(t, p3, wantp)

	test.ContentEqualsString(t, p3, "An HTML section")

	// Second nested
	p4 := p2.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Disposition: "inline",
		Filename:    "attach.txt",
		Descriptor:  "2.2",
	}
	test.ComparePart(t, p4, wantp)

	test.ContentEqualsString(t, p4, "An inline text attachment")

	// Third nested
	p5 := p2.Subparts[2]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Disposition: "inline",
		Filename:    "attach2.txt",
		Descriptor:  "2.3",
	}
	test.ComparePart(t, p5, wantp)

	test.ContentEqualsString(t, p5, "Another inline text attachment")
}

func TestPartSimilarBoundaryNested(t *testing.T) {
	var want string
	var wantp *mime.Part

	r := test.OpenTestData("parts", "similar-boundary-nested.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		ContentType: "multipart/mixed",
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "Section one\n"
	test.ContentEqualsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		Subparts:    []*mime.Part{test.PartExists, test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "2.0",
	}
	test.ComparePart(t, p2, wantp)

	p3 := p2.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "2.1.0",
	}
	test.ComparePart(t, p3, wantp)

	// First nested
	p31 := p3.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "2.1.1",
	}
	test.ComparePart(t, p31, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p31, want)

	// Second nested
	p41 := p3.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2.1.2",
	}
	test.ComparePart(t, p41, wantp)

	want = "An HTML section"
	test.ContentEqualsString(t, p41, want)

	// First nested
	p5 := p2.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "2.2",
	}
	test.ComparePart(t, p5, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p5, want)

	// Second nested
	p4 := p2.Subparts[2]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2.3",
	}
	test.ComparePart(t, p4, wantp)

	want = "An HTML section"
	test.ContentEqualsString(t, p4, want)
}

func TestPartSimilarBoundary(t *testing.T) {
	var want string
	var wantp *mime.Part

	var tests = [][]string{
		[]string{"small", "similar-boundary.raw"},
		[]string{"large", "similar-boundary-large.raw"},
	}

	for _, f := range tests {
		t.Run(f[0], func(t *testing.T) {
			r := test.OpenTestData("parts", f[1])
			p, err := mime.ReadParts(r)

			// Examine root
			if err != nil {
				t.Fatal("Unexpected parse error:", err)
			}
			if p == nil {
				t.Fatal("Root node should not be nil")
			}

			wantp = &mime.Part{
				ContentType: "multipart/mixed",
				Subparts:    []*mime.Part{test.PartExists, test.PartExists},
				Descriptor:  "0",
			}
			test.ComparePart(t, p, wantp)

			// Examine first child
			p1 := p.Subparts[0]
			wantp = &mime.Part{
				Parent:      test.PartExists,
				ContentType: "text/plain",
				Charset:     "us-ascii",
				Descriptor:  "1",
			}
			test.ComparePart(t, p1, wantp)

			want = "Section one\n"
			test.ContentEqualsString(t, p1, want)

			// Examine sibling
			p2 := p.Subparts[1]
			wantp = &mime.Part{
				Parent:      test.PartExists,
				Subparts:    []*mime.Part{test.PartExists, test.PartExists},
				ContentType: "multipart/alternative",
				Descriptor:  "2.0",
			}
			test.ComparePart(t, p2, wantp)

			// First nested
			p3 := p2.Subparts[0]
			wantp = &mime.Part{
				Parent:      test.PartExists,
				ContentType: "text/plain",
				Charset:     "us-ascii",
				Descriptor:  "2.1",
			}
			test.ComparePart(t, p3, wantp)

			want = "A text section"
			test.ContentEqualsString(t, p3, want)

			// Second nested
			p4 := p2.Subparts[1]
			wantp = &mime.Part{
				Parent:      test.PartExists,
				ContentType: "text/html",
				Charset:     "us-ascii",
				Descriptor:  "2.2",
			}
			test.ComparePart(t, p4, wantp)

			want = "An HTML section"
			test.ContentEqualsString(t, p4, want)
		})
	}
}

// Check we don't UTF-8 decode attachments
func TestBinaryDecode(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "bin-attach.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/mixed",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "application/octet-stream",
		Charset:     "us-ascii",
		Disposition: "attachment",
		Filename:    "test.bin",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	d, err := p2.Decode()
	if err != nil {
		t.Error(err)
	}

	wantBytes := []byte{
		0x50, 0x4B, 0x03, 0x04, 0x14, 0x00, 0x08, 0x00,
		0x08, 0x00, 0xC2, 0x02, 0x29, 0x4A, 0x00, 0x00}
	test.ContentEqualsBytes(t, d, wantBytes)
}

func TestMultiBase64Parts(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "multibase64.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/mixed",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	d, err := p1.Decode()
	if err != nil {
		t.Error(err)
	}

	want = "A text section"
	test.ContentEqualsString(t, d, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Disposition: "attachment",
		Filename:    "test.html",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	d, err = p2.Decode()
	if err != nil {
		t.Error(err)
	}

	want = "<html>\n"
	test.ContentEqualsString(t, d, want)
}

func TestBadBoundaryTerm(t *testing.T) {
	var want string
	var wantp *mime.Part
	r := test.OpenTestData("parts", "badboundary.raw")
	p, err := mime.ReadParts(r)

	// Examine root
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}

	wantp = &mime.Part{
		Subparts:    []*mime.Part{test.PartExists, test.PartExists},
		ContentType: "multipart/alternative",
		Descriptor:  "0",
	}
	test.ComparePart(t, p, wantp)

	// Examine first child
	p1 := p.Subparts[0]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/plain",
		Charset:     "us-ascii",
		Descriptor:  "1",
	}
	test.ComparePart(t, p1, wantp)

	want = "A text section"
	test.ContentEqualsString(t, p1, want)

	// Examine sibling
	p2 := p.Subparts[1]
	wantp = &mime.Part{
		Parent:      test.PartExists,
		ContentType: "text/html",
		Charset:     "us-ascii",
		Descriptor:  "2",
	}
	test.ComparePart(t, p2, wantp)

	want = "An HTML section"
	test.ContentEqualsString(t, p2, want)
}
