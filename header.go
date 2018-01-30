package mime

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log"
	"mime"
	"net/textproto"
	"strings"
)

const (
	// Standard MIME content dispositions
	cdAttachment = "attachment"
	cdInline     = "inline"

	// Standard MIME content types
	ctAppOctetStream  = "application/octet-stream"
	ctMultipartAltern = "multipart/alternative"
	ctMultipartPrefix = "multipart/"
	ctTextPlain       = "text/plain"
	ctTextHTML        = "text/html"

	// Standard MIME header names
	hnContentDisposition = "Content-Disposition"
	hnContentEncoding    = "Content-Transfer-Encoding"
	hnContentType        = "Content-Type"

	// Standard MIME header parameters
	hpBoundary = "boundary"
	hpCharset  = "charset"
	hpFile     = "file"
	hpFilename = "filename"
	hpName     = "name"
)

var (
	ErrEmptyHeaderBlock = errors.New("empty header block")
	// ErrorMalformedBase64 name
	ErrorMalformedBase64 = errors.New("malformed base64")
	// ErrorMalformedHeader name
	ErrorMalformedHeader = errors.New("malformed header")
	// ErrorMissingBoundary name
	ErrorMissingBoundary = errors.New("missing boundary")
	// ErrorMissingContentType name
	ErrorMissingContentType = errors.New("missing Content-Type")
	// ErrorCharsetConversion name
	ErrorCharsetConversion = errors.New("character set conversion")
	// ErrorContentEncoding name
	ErrorContentEncoding = errors.New("content encoding")
)

// Terminology from RFC 2047:
//  encoded-word: the entire =?charset?encoding?encoded-text?= string
//  charset: the character set portion of the encoded word
//  encoding: the character encoding type used for the encoded-text
//  encoded-text: the text we are decoding

// readHeader reads a block of SMTP or MIME headers and returns a textproto.MIMEHeader.
// Header parse warnings & errors will be added to p.Errors, io errors will be returned directly.
func readHeader(r *bufio.Reader) (textproto.MIMEHeader, error) {
	// buf holds the massaged output for textproto.Reader.ReadMIMEHeader()
	buf := &bytes.Buffer{}
	tp := textproto.NewReader(r)
	firstHeader := true
	for {
		// Pull out each line of the headers as a temporary slice s
		s, err := tp.ReadLineBytes()
		if err != nil {
			if err == io.ErrUnexpectedEOF && buf.Len() == 0 {
				return nil, ErrEmptyHeaderBlock
			} else if err == io.EOF {
				buf.Write([]byte{'\r', '\n'})
				break
			}
			return nil, err
		}
		firstColon := bytes.IndexByte(s, ':')
		firstSpace := bytes.IndexAny(s, " \t\n\r")
		if firstSpace == 0 {
			// Starts with space: continuation
			buf.WriteByte(' ')
			buf.Write(textproto.TrimBytes(s))
			continue
		}
		if firstColon == 0 {
			// Can't parse line starting with colon: skip
			//p.Errors = append(p.Errors, (ErrorMalformedHeader, "Header line %q started with a colon", s)
			log.Printf("%v: header line %q started with a colon", ErrorMalformedHeader, s)
			continue
		}
		if firstColon > 0 {
			// Contains a colon, treat as a new header line
			if !firstHeader {
				// New Header line, end the previous
				buf.Write([]byte{'\r', '\n'})
			}
			s = textproto.TrimBytes(s)
			buf.Write(s)
			firstHeader = false
		} else {
			// No colon: potential non-indented continuation
			if len(s) > 0 {
				// Attempt to detect and repair a non-indented continuation of previous line
				buf.WriteByte(' ')
				buf.Write(s)
				//p.addWarning(ErrorMalformedHeader, "Continued line %q was not indented", s)
				log.Printf("%v: continued line %q was not indented", ErrorMalformedHeader, s)
			} else {
				// Empty line, finish header parsing
				buf.Write([]byte{'\r', '\n'})
				break
			}
		}
	}
	buf.Write([]byte{'\r', '\n'})
	tr := textproto.NewReader(bufio.NewReader(buf))
	header, err := tr.ReadMIMEHeader()
	return header, err
}

// decodeHeader decodes a single line (per RFC 2047) using Golang's mime.WordDecoder
func decodeHeader(input string) string {
	if !strings.Contains(input, "=?") {
		// Don't scan if there is nothing to do here
		return input
	}

	dec := new(mime.WordDecoder)
	dec.CharsetReader = newCharsetReader
	header, err := dec.DecodeHeader(input)
	if err != nil {
		return input
	}
	return header
}

// decodeToUTF8Base64Header decodes a MIME header per RFC 2047, reencoding to =?utf-8b?
func decodeToUTF8Base64Header(input string) string {
	if !strings.Contains(input, "=?") {
		// Don't scan if there is nothing to do here
		return input
	}

	log.Printf("input = %q", input)
	tokens := strings.FieldsFunc(input, isWhiteSpaceRune)
	output := make([]string, len(tokens))
	for i, token := range tokens {
		if len(token) > 4 && strings.Contains(token, "=?") {
			// Stash parenthesis, they should not be encoded
			prefix := ""
			suffix := ""
			if token[0] == '(' {
				prefix = "("
				token = token[1:]
			}
			if token[len(token)-1] == ')' {
				suffix = ")"
				token = token[:len(token)-1]
			}
			// Base64 encode token
			output[i] = prefix + mime.BEncoding.Encode("UTF-8", decodeHeader(token)) + suffix
		} else {
			output[i] = token
		}
		log.Printf("%v %q %q", i, token, output[i])
	}

	// Return space separated tokens
	return strings.Join(output, " ")
}

// Detects a RFC-822 linear-white-space, passed to strings.FieldsFunc
func isWhiteSpaceRune(r rune) bool {
	switch r {
	case ' ':
		return true
	case '\t':
		return true
	case '\r':
		return true
	case '\n':
		return true
	default:
		return false
	}
}

func parseMediaType(ctype string) (string, map[string]string, error) {
	// Parse Content-Type header
	mtype, mparams, err := mime.ParseMediaType(ctype)
	if err != nil {
		// Small hack to remove harmless charset duplicate params
		mctype := parseBadContentType(ctype, ";")
		mtype, mparams, err = mime.ParseMediaType(mctype)
		if err != nil {
			// Some badly formed content-types forget to send a ; between fields
			mctype := parseBadContentType(ctype, " ")
			if strings.Contains(mctype, `name=""`) {
				mctype = strings.Replace(mctype, `name=""`, `name=" "`, -1)
			}
			mtype, mparams, err = mime.ParseMediaType(mctype)
			if err != nil {
				return "", make(map[string]string), err
			}
		}
	}
	return mtype, mparams, err
}

func parseBadContentType(ctype, sep string) string {
	cp := strings.Split(ctype, sep)
	mctype := ""
	for _, p := range cp {
		if strings.Contains(p, "=") {
			params := strings.Split(p, "=")
			if !strings.Contains(mctype, params[0]+"=") {
				mctype += p + ";"
			}
		} else {
			mctype += p + ";"
		}
	}
	return mctype
}
