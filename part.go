package mime

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/quotedprintable"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/cardamaro/mem_constrained_buffer"
	"github.com/pkg/errors"
)

const (
	ContentTypeRfc822 = "message/rfc822"
)

type ReaderAtCloser interface {
	io.ReaderAt
	io.Closer
}

type Part struct {
	Descriptor string

	ContentType       string
	ContentParams     map[string]string
	Disposition       string
	DispositionParams map[string]string
	Encoding          string
	Charset           string
	Filename          string

	Size  int
	Lines int

	Parent       *Part
	Subparts     []*Part
	Header       textproto.MIMEHeader
	HeaderReader io.Reader

	PartOffset, HeaderLen, PartLen int
	Epilogue                       []byte
	Errors                         []error

	boundary  string
	reader    io.Reader
	rawReader ReaderAtCloser
}

func ReadParts(r io.Reader) (*Part, error) {
	b := mem_constrained_buffer.New()
	_, err := b.ReadFrom(r)
	if err != nil {
		return nil, errors.Wrap(err, "error filling buffer")
	}

	root := NewPart(nil)
	// this rawReader will be copied to subparts in NewPart via the Parent pointer
	root.rawReader = b

	err = root.readPart(b, 0)
	if err != nil {
		return nil, errors.Wrap(err, "error reading part")
	}

	return root, nil
}

func NewPart(parent *Part) *Part {
	part := &Part{
		Parent: parent,
	}
	if parent != nil {
		part.rawReader = parent.rawReader
	}
	return part
}

func (p *Part) Close() error {
	return p.rawReader.Close()
}

func (p *Part) RawReader() io.Reader {
	return io.MultiReader(p.HeaderReader, p)
}

func (p *Part) Decode() (io.Reader, error) {
	valid := true
	r := p.reader

	// Allow later access to Base64 errors
	var b64cleaner *base64Cleaner

	// Build content decoding reader
	encoding := p.Header.Get(hnContentEncoding)
	switch strings.ToLower(encoding) {
	case "quoted-printable":
		r = newQPCleaner(r)
		r = quotedprintable.NewReader(r)
	case "base64":
		b64cleaner = newBase64Cleaner(r)
		r = base64.NewDecoder(base64.RawStdEncoding, b64cleaner)
	case "8bit", "7bit", "binary", "":
		// No decoding required
	default:
		// Unknown encoding
		valid = false
		log.Printf("%s: unrecognized Content-Transfer-Encoding type %q", ErrorContentEncoding, encoding)
		//p.addWarning(
		//	ErrorContentEncoding,
		//	"Unrecognized Content-Transfer-Encoding type %q",
		//	encoding)
	}

	if valid && !detectAttachmentHeader(p.Header) {
		// decodedReader is good; build character set conversion reader
		if p.Charset != "" {
			if reader, err := newCharsetReader(p.Charset, r); err == nil {
				r = reader
			} else {
				// Try to parse charset again here to see if we can salvage some badly formed ones
				// like charset="charset=utf-8"
				charsetp := strings.Split(p.Charset, "=")
				if strings.ToLower(charsetp[0]) == "charset" && len(charsetp) > 1 {
					p.Charset = charsetp[1]
					if reader, err := newCharsetReader(p.Charset, r); err == nil {
						r = reader
					} else {
						// Failed to get a conversion reader
						//p.addWarning(ErrorCharsetConversion, err.Error())
						log.Print(ErrorCharsetConversion)
					}
				} else {
					// Failed to get a conversion reader
					//p.addWarning(ErrorCharsetConversion, err.Error())
					log.Print(ErrorCharsetConversion)
				}
			}
		}
	}

	return r, nil
	//if b64cleaner != nil {
	//	p.Errors = append(p.Errors, b64cleaner.Errors...)
	//}
}

type PartVisitor func(p *Part) error

func (p *Part) Walk(v PartVisitor) error {
	if err := v(p); err != nil {
		return err
	}
	for _, s := range p.Subparts {
		if err := s.Walk(v); err != nil {
			return err
		}
	}
	return nil
}

func (p *Part) String() string {
	return fmt.Sprintf("%s <%s>", p.Descriptor, p.ContentType)
}

func (p *Part) Read(b []byte) (int, error) {
	return p.reader.Read(b)
}

func (p *Part) readPart(r io.Reader, offset int) error {
	cr := countingReader{Reader: r}
	br := bufio.NewReader(&cr)

	header, err := readHeader(br)
	if err != nil {
		return err
	}

	p.HeaderLen = cr.N - br.Buffered()
	p.Header = header

	// Content-Type, default is text/plain us-ascii according to RFC 2046
	// https://tools.ietf.org/html/rfc2046#section-5.1
	mediatype := "text/plain"
	params := map[string]string{
		"charset": "us-ascii",
	}
	ctype := header.Get(hnContentType)
	if ctype == "" {
		//p.addWarning(
		//	ErrorMissingContentType,
		//	"MIME parts should have a Content-Type header")
		log.Printf("%s: MIME parts should have a Content-Type header", p.Descriptor)
	} else {
		// Parse Content-Type header
		mediatype, params, err = parseMediaType(ctype)
		if err != nil {
			return err
		}
	}
	p.ContentType = strings.ToLower(mediatype)
	p.ContentParams = params
	p.Charset = strings.ToLower(params[hpCharset])

	// Set disposition, filename, charset if available
	p.setupContentHeaders(params)
	p.boundary = params[hpBoundary]

	if p.boundary != "" {
		// Content is another multipart
		err = parseParts(p, br, &cr, p.PartOffset)
		if err != nil {
			return err
		}
	} else {
		if p.ContentType == ContentTypeRfc822 {
			pp := NewPart(p)
			pp.PartOffset = p.PartOffset + p.HeaderLen
			pp.Descriptor = p.Descriptor
			err = pp.readPart(br, offset)
			if err != nil {
				return err
			}
		} else {
			if _, err := io.Copy(ioutil.Discard, br); err != nil {
				return err
			}
		}
	}

	// Insert this Part into the MIME tree
	if p.Parent != nil {
		p.Parent.Subparts = append(p.Parent.Subparts, p)
	}

	p.PartLen = cr.N - br.Buffered()
	p.Size = p.PartLen - p.HeaderLen

	p.reader = io.NewSectionReader(
		p.rawReader, int64(p.PartOffset+p.HeaderLen), int64(p.PartLen-p.HeaderLen))
	p.HeaderReader = io.NewSectionReader(
		p.rawReader, int64(p.PartOffset), int64(p.HeaderLen))

	return nil
}

// parseParts recursively parses a mime multipart document and sets each Part's Descriptor.
func parseParts(parent *Part, reader *bufio.Reader, cr *countingReader, offset int) error {
	firstRecursion := parent.Parent == nil
	// Set root Descriptor
	if firstRecursion {
		parent.Descriptor = "0"
	}

	var indexDescriptor int

	// Loop over MIME parts
	br := newBoundaryReader(reader, parent.boundary)
	for {
		indexDescriptor++

		next, err := br.Next()
		if err != nil && err != io.EOF {
			return err
		}
		if !next {
			break
		}

		p := NewPart(parent)

		p.PartOffset = offset + (cr.N - reader.Buffered())

		// Set this Part's Descriptor, indicating its position within the MIME Part Tree
		if firstRecursion {
			p.Descriptor = strconv.Itoa(indexDescriptor)
		} else {
			p.Descriptor = p.Parent.Descriptor + "." + strconv.Itoa(indexDescriptor)
		}

		err = p.readPart(br, offset)
		if err == ErrEmptyHeaderBlock {
			// Empty header probably means the part didn't use the correct trailing "--" syntax to
			// close its boundary.
			if _, err = br.Next(); err != nil {
				if err == io.EOF || strings.HasSuffix(err.Error(), "EOF") {
					// There are no more Parts, but the error belongs to a sibling or parent,
					// because this Part doesn't actually exist.
					// TODO
					log.Printf("%v: boundary %q was not closed correctly", ErrorMissingBoundary, parent.boundary)
					break
				}
				return fmt.Errorf("error at boundary %v: %v", parent.boundary, err)
			}
		} else if err != nil {
			return errors.Wrap(err, "error reading part")
		}
	}

	// Store any content following the closing boundary marker into the epilogue
	epilogue := new(bytes.Buffer)
	if _, err := io.Copy(epilogue, reader); err != nil {
		return err
	}
	parent.Epilogue = epilogue.Bytes()

	// If a Part is "multipart/" Content-Type, it will have .0 appended to its Descriptor
	// i.e. it is the root of its MIME Part subtree
	if !firstRecursion {
		parent.Descriptor += ".0"
	}

	return nil
}

// setupContentHeaders uses Content-Type media params and Content-Disposition headers to populate
// the disposition, filename, and charset fields.
func (p *Part) setupContentHeaders(mediaParams map[string]string) {
	// Determine content disposition, filename, character set
	disposition, dparams, err := parseMediaType(p.Header.Get(hnContentDisposition))
	if err == nil {
		// Disposition is optional
		p.Disposition = disposition
		p.Filename = decodeHeader(dparams[hpFilename])
	}
	if p.Filename == "" && mediaParams[hpName] != "" {
		p.Filename = decodeHeader(mediaParams[hpName])
	}
	if p.Filename == "" && mediaParams[hpFile] != "" {
		p.Filename = decodeHeader(mediaParams[hpFile])
	}
	if p.Charset == "" {
		p.Charset = strings.ToLower(mediaParams[hpCharset])
	}
}

type countingReader struct {
	io.Reader
	N int
}

func (cr *countingReader) Read(p []byte) (n int, err error) {
	n, err = cr.Reader.Read(p)
	cr.N += n
	return n, err
}
