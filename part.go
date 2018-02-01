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

type Part struct {
	Descriptor string

	ContentType       string
	ContentParams     map[string]string
	Disposition       string
	DispositionParams map[string]string
	Encoding          string
	Charset           string
	Filename          string

	Bytes uint32
	Lines uint32

	Parent   *Part
	Subparts []*Part
	Header   textproto.MIMEHeader

	PartOffset, HeaderLen, PartLen int
	Epilogue                       []byte
	Errors                         []error

	boundary  string
	reader    io.Reader
	rawReader io.ReaderAt
}

func ReadParts(r io.Reader) (*Part, error) {
	b := mem_constrained_buffer.New()
	n, err := b.ReadFrom(r)
	if err != nil {
		return nil, errors.Wrap(err, "error filling buffer")
	}

	cr := countingReader{Reader: b}
	br := bufio.NewReader(&cr)
	root := NewPart(nil)
	root.rawReader = b

	header, err := readHeader(br)
	if err != nil {
		return nil, errors.Wrap(err, "error reading header")
	}
	root.Header = header
	root.HeaderLen = cr.N - br.Buffered()
	root.PartLen = int(n)

	// Content-Type, default is text/plain us-ascii according to RFC 822
	mediatype := "text/plain"
	params := map[string]string{
		"charset": "us-ascii",
	}
	contentType := header.Get(hnContentType)
	if contentType != "" {
		mediatype, params, err = parseMediaType(contentType)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing media type")
		}
	}
	root.ContentType = mediatype
	root.Charset = params[hpCharset]
	root.ContentParams = params

	if strings.HasPrefix(mediatype, ctMultipartPrefix) {
		// Content is multipart, parse it
		boundary := params[hpBoundary]
		root.boundary = boundary
		err = parseParts(root, br, &cr, 0)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing multipart")
		}
	} else {
		if _, err := io.Copy(ioutil.Discard, br); err != nil {
			return nil, err
		}
	}
	root.reader = io.NewSectionReader(
		root.rawReader,
		int64(root.HeaderLen),
		int64(root.PartLen-root.HeaderLen))

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

func (p *Part) Decode() error {
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

	if b64cleaner != nil {
		p.Errors = append(p.Errors, b64cleaner.Errors...)
	}
	p.reader = r

	return nil
}

func (p *Part) Read(b []byte) (int, error) {
	return p.reader.Read(b)
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

		// Set this Part's Descriptor, indicating its position within the MIME Part Tree
		if firstRecursion {
			p.Descriptor = strconv.Itoa(indexDescriptor)
		} else {
			p.Descriptor = p.Parent.Descriptor + "." + strconv.Itoa(indexDescriptor)
		}

		p.PartOffset = offset + (cr.N - reader.Buffered())

		ccr := countingReader{Reader: br}
		bbr := bufio.NewReader(&ccr)
		header, err := readHeader(bbr)
		p.Header = header
		p.HeaderLen = ccr.N - bbr.Buffered()
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
			return err
		}

		ctype := header.Get(hnContentType)
		if ctype == "" {
			//p.addWarning(
			//	ErrorMissingContentType,
			//	"MIME parts should have a Content-Type header")
			log.Printf("%s: MIME parts should have a Content-Type header", p.Descriptor)
		} else {
			// Parse Content-Type header
			mtype, mparams, err := parseMediaType(ctype)
			if err != nil {
				return err
			}
			p.ContentType = mtype
			p.ContentParams = mparams

			// Set disposition, filename, charset if available
			p.setupContentHeaders(mparams)
			p.boundary = mparams[hpBoundary]
		}

		// Insert this Part into the MIME tree
		if parent != nil {
			parent.Subparts = append(parent.Subparts, p)
		}

		if p.boundary != "" {
			// Content is another multipart
			err = parseParts(p, bbr, &ccr, p.PartOffset)
			if err != nil {
				return err
			}
		} else {
			if _, err := io.Copy(ioutil.Discard, bbr); err != nil {
				return err
			}
		}
		p.PartLen = ccr.N - bbr.Buffered()
		p.reader = io.NewSectionReader(
			p.rawReader,
			int64(p.PartOffset+p.HeaderLen),
			int64(p.PartLen-p.HeaderLen))
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
		p.Charset = mediaParams[hpCharset]
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
