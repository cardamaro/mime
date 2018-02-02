package mime_test

import (
	"bytes"
	"io/ioutil"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"github.com/cardamaro/mime"
)

func TestSmall(t *testing.T) {
	var buf1, buf2, buf3, buf4, buf5 bytes.Buffer

	fill(&buf1, 'a', 10)
	fill(&buf2, 'b', 20)
	fill(&buf3, 'c', 30)
	fill(&buf4, 'd', 1)
	fill(&buf5, 'e', 2)

	content := [][]byte{
		buf1.Bytes(),
		buf2.Bytes(),
		buf3.Bytes(),
		buf4.Bytes(),
		buf5.Bytes(),
	}

	autogen(t, content)
}

func TestMedium(t *testing.T) {
	var buf1, buf2, buf3, buf4, buf5 bytes.Buffer

	fill(&buf1, 'a', 100)
	fill(&buf2, 'b', 200)
	fill(&buf3, 'c', 3000)
	fill(&buf4, 'd', 10)
	fill(&buf5, 'e', 20)

	content := [][]byte{
		buf1.Bytes(),
		buf2.Bytes(),
		buf3.Bytes(),
		buf4.Bytes(),
		buf5.Bytes(),
	}

	autogen(t, content)
}

func TestLarge(t *testing.T) {
	var buf1, buf2, buf3, buf4, buf5 bytes.Buffer

	fill(&buf1, 'a', 10000)
	fill(&buf2, 'b', 20000)
	fill(&buf3, 'c', 300)
	fill(&buf4, 'd', 1000)
	fill(&buf5, 'e', 20000)

	content := [][]byte{
		buf1.Bytes(),
		buf2.Bytes(),
		buf3.Bytes(),
		buf4.Bytes(),
		buf5.Bytes(),
	}

	autogen(t, content)
}

func autogen(t *testing.T, content [][]byte) {
	t.Helper()
	s := generate(t, content)

	p, err := mime.ReadParts(strings.NewReader(s))
	if err != nil {
		t.Fatal("Unexpected parse error:", err)
	}
	if p == nil {
		t.Fatal("Root node should not be nil")
	}
	defer p.Close()

	var i int
	p.Walk(func(pp *mime.Part) error {
		t.Logf("pp: %s", pp)

		if strings.HasPrefix(pp.ContentType, "multipart") {
			return nil
		}

		b, err := ioutil.ReadAll(pp)
		if err != nil {
			t.Errorf("error reading part %s: %v", pp, err)
		}
		if want, got := string(content[i]), string(b); want != got {
			t.Errorf("wanted %q, got %q", want, got)
		}

		i++

		return nil
	})
}

func fill(b *bytes.Buffer, ch byte, size int) {
	for i := 0; i < size; i++ {
		b.WriteByte(ch)
		if i > 0 && i%70 == 0 {
			b.Write([]byte("\n"))
		}
	}
}

func generate(t *testing.T, c [][]byte) string {
	t.Helper()

	var buf bytes.Buffer
	buf.Write([]byte(`From: John Doe <jdoe@machine.example>
To: Mary Smith <mary@example.net>
Subject: Saying Hello
Message-ID: <1234@local.machine.example>
Content-Type: multipart/mixed; boundary="part_0"

`))

	/*
	  0 <multipart/mixed>
	  1.0 <multipart/related>
	  1.1.0 <multipart/alternative>
	  1.1.1 <text/plain>
	  1.1.2 <text/html>
	  1.2 <image/bmp>
	  2 <application/octet-stream>
	  3 <application/octet-stream>
	*/

	w := multipart.NewWriter(&buf)
	w.SetBoundary("part_0")

	buf.Write([]byte("preamble\n\n"))

	p1_0, _ := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/related; boundary=\"part_1_0\""},
	})
	w1_0 := multipart.NewWriter(p1_0)
	w1_0.SetBoundary("part_1_0")

	p1_1_0, _ := w1_0.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/alternative; boundary=\"part_1_1_0\""},
	})
	w1_1_0 := multipart.NewWriter(p1_1_0)
	w1_1_0.SetBoundary("part_1_1_0")
	p1_1_1, _ := w1_1_0.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"text/plain"},
	})

	p1_1_1.Write(c[0])

	p1_1_2, _ := w1_1_0.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"text/html"},
	})

	p1_1_2.Write(c[1])

	w1_1_0.Close()

	p1_2, _ := w1_0.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"image/bmp"},
	})
	p1_2.Write(c[2])

	w1_0.Close()

	p2, _ := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"application/octet-stream"},
	})
	p2.Write(c[3])

	p3, _ := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"application/octet-stream"},
	})
	p3.Write(c[4])

	w.Close()

	buf.Write([]byte("\nepilogue"))

	return buf.String()
}
