package mime

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
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

	Epilogue []byte
	Errors   []error

	boundary string
	reader   io.Reader
}

func ReadParts(r io.Reader) (*Part, error) {
	b := mem_constrained_buffer.New()
	n, err := b.ReadFrom(r)
	if err != nil {
		return nil, errors.Wrap(err, "error filling buffer")
	}

	br := bufio.NewReader(b)
	root := new(Part)
	root.Bytes = uint32(n)

	header, err := readHeader(br)
	if err != nil {
		return nil, errors.Wrap(err, "error reading header")
	}
	root.Header = header

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
		err = parseParts(root, br)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing multipart")
		}
	} else {
		// Read raw content into buffer
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r); err != nil {
			return nil, errors.Wrap(err, "error reading part")
		}
		root.reader = &buf
		// Content is text or data, build content reader pipeline
		//if err := root.buildContentReaders(br); err != nil {
		//	return nil, err
		//}
	}

	return root, nil
}

func NewPart(parent *Part, contentType string) *Part {
	return &Part{Parent: parent, ContentType: contentType}
}

func (p *Part) Read(b []byte) (int, error) {
	return p.reader.Read(b)
}

// parseParts recursively parses a mime multipart document and sets each Part's Descriptor.
func parseParts(parent *Part, reader *bufio.Reader) error {
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

		p := &Part{Parent: parent}

		// Set this Part's Descriptor, indicating its position within the MIME Part Tree
		if firstRecursion {
			p.Descriptor = strconv.Itoa(indexDescriptor)
		} else {
			p.Descriptor = p.Parent.Descriptor + "." + strconv.Itoa(indexDescriptor)
		}

		bbr := bufio.NewReader(br)
		header, err := readHeader(bbr)
		p.Header = header
		if err == ErrEmptyHeaderBlock {
			// Empty header probably means the part didn't use the correct trailing "--" syntax to
			// close its boundary.
			if _, err = br.Next(); err != nil {
				if err == io.EOF || strings.HasSuffix(err.Error(), "EOF") {
					// There are no more Parts, but the error belongs to a sibling or parent,
					// because this Part doesn't actually exist.
					//owner.addWarning(
					//	ErrorMissingBoundary,
					//	"Boundary %q was not closed correctly",
					//	parent.boundary)
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
			log.Printf("MIME parts should have a Content-Type header")
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
			err = parseParts(p, bbr)
			if err != nil {
				return err
			}
		} else {
			// Content is text or data: build content reader pipeline
			//if err := p.buildContentReaders(bbr); err != nil {
			//	return err
			//}
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
		p.Charset = mediaParams[hpCharset]
	}
}
