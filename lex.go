package iolang

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

type token struct {
	Kind  tokenKind
	Value string
	Err   error

	// Line, Col int
}

type tokenKind int

const (
	badToken tokenKind = iota
	semiToken
	identToken
	openToken
	closeToken
	commaToken
	numberToken
	hexToken
	stringToken
	triquoteToken
)

type lexFn func(src *bufio.Reader, tokens chan<- token) lexFn

func lex(src *bufio.Reader, tokens chan<- token) {
	state := eatSpace
	for state != nil {
		state = state(src, tokens)
	}
	close(tokens)
}

// Append the next run of characters in src which satisfy the predicate to b.
// Returns b after appending, the first rune which did not satisfy the
// predicate, and any error that occurred. Iff there was no such error, the
// last rune is unread.
func accept(src *bufio.Reader, predicate func(rune) bool, b []byte) ([]byte, rune, error) {
	r, _, err := src.ReadRune()
	for {
		if err != nil {
			return b, r, err
		}
		if !predicate(r) {
			break
		}
		b = append(b, string(r)...)
		r, _, err = src.ReadRune()
	}
	src.UnreadRune()
	return b, r, nil
}

func lexsend(err error, tokens chan<- token, good token) lexFn {
	if err != nil && err != io.EOF {
		good.Kind = badToken
		good.Err = err
	}
	tokens <- good
	if err != nil {
		return nil
	}
	return eatSpace
}

func eatSpace(src *bufio.Reader, tokens chan<- token) lexFn {
	// Could use accept here, but I've already written this.
	r, _, err := src.ReadRune()
	for {
		if err != nil {
			if err != io.EOF {
				tokens <- token{
					Kind:  badToken,
					Value: string(r),
					Err:   err,
				}
			}
			return nil
		}
		if !strings.ContainsRune(" \r\f\t\v", r) {
			break
		}
		r, _, err = src.ReadRune()
	}
	switch {
	case r == ';', r == '\n':
		tokens <- token{
			Kind:  semiToken,
			Value: string(r),
		}
		return eatSpace
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', r == '_', r >= 0x80:
		src.UnreadRune()
		return lexIdent
	case strings.ContainsRune("!$%&'*+-/:<=>?@\\^|~", r):
		src.UnreadRune()
		return lexOp
	case strings.ContainsRune("([{", r):
		tokens <- token{
			Kind:  openToken,
			Value: string(r),
		}
		return eatSpace
	case strings.ContainsRune(")]}", r):
		tokens <- token{
			Kind:  closeToken,
			Value: string(r),
		}
		return eatSpace
	case r == ',':
		tokens <- token{
			Kind:  commaToken,
			Value: ",",
		}
		return eatSpace
	case '0' <= r && r <= '9':
		src.UnreadRune()
		return lexNumber
	case r == '.':
		// . can be either a number or an identifier, because Dumbledore.
		src.UnreadRune()
		peek, _ := src.Peek(2)
		if len(peek) > 1 && '0' <= peek[1] && peek[1] <= '9' {
			return lexNumber
		}
		return lexIdent
	case r == '"':
		src.UnreadRune()
		return lexString
	}
	panic(r)
}

func lexIdent(src *bufio.Reader, tokens chan<- token) lexFn {
	b, _, err := accept(src, func(r rune) bool {
		return 'a' <= r && r <= 'z' ||
			'A' <= r && r <= 'Z' ||
			'0' <= r && r <= '9' ||
			r == '_' || r == '.' || r >= 0x80
	}, nil)
	return lexsend(err, tokens, token{Kind: identToken, Value: string(b)})
}

func lexOp(src *bufio.Reader, tokens chan<- token) lexFn {
	b, _, err := accept(src, func(r rune) bool {
		return strings.ContainsRune("!$%&'*+-/:<=>?@\\^|~", r)
	}, nil)
	return lexsend(err, tokens, token{Kind: identToken, Value: string(b)})
}

func lexNumber(src *bufio.Reader, tokens chan<- token) lexFn {
	b, r, err := accept(src, func(r rune) bool { return '0' <= r && r <= '9' }, nil)
	if err != nil {
		return lexsend(err, tokens, token{Kind: numberToken, Value: string(b)})
	}
	if r == 'x' || r == 'X' {
		b = append(b, 'x')
		b, _, err = accept(src, func(r rune) bool {
			return '0' <= r && r <= '9' || 'a' <= r && r <= 'f' || 'A' <= r && r <= 'F'
		}, b)
		lexsend(err, tokens, token{Kind: numberToken, Value: string(b)})
	}
	if r == '.' {
		b = append(b, '.')
		_, _, err = src.ReadRune()
		if err != nil {
			return lexsend(err, tokens, token{Kind: numberToken, Value: string(b)})
		}
		b, r, err = accept(src, func(r rune) bool { return '0' <= r && r <= '9' }, b)
		if err != nil {
			return lexsend(err, tokens, token{Kind: numberToken, Value: string(b)})
		}
	}
	if r == 'e' || r == 'E' {
		r, _, err = src.ReadRune()
		if err != nil {
			return lexsend(err, tokens, token{Kind: numberToken, Value: string(b)})
		}
		if r == '-' || r == '+' {
			r, _, err = src.ReadRune()
			b = append(b, 'e', byte(r))
		} else {
			b = append(b, 'e')
		}
		b, _, err = accept(src, func(r rune) bool { return '0' <= r && r <= '9' }, b)
	}
	return lexsend(err, tokens, token{Kind: numberToken, Value: string(b)})
}

func lexString(src *bufio.Reader, tokens chan<- token) lexFn {
	peek, _ := src.Peek(3)
	if bytes.Equal(peek, []byte{'"', '"', '"'}) {
		return lexTriquote(src, tokens)
	}
	return lexMonoquote(src, tokens)
}

func lexTriquote(src *bufio.Reader, tokens chan<- token) lexFn {
	b := make([]byte, 3, 6)
	src.Read(b)
	for {
		r, _, err := src.ReadRune()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			tokens <- token{
				Kind:  badToken,
				Value: string(b),
				Err:   err,
			}
			return nil
		}
		if r == '"' {
			peek, err := src.Peek(2)
			if bytes.Equal(peek, []byte{'"', '"'}) {
				return lexsend(err, tokens, token{Kind: triquoteToken, Value: string(b) + `"""`})
			}
		}
		b = append(b, string(r)...)
	}
}

func lexMonoquote(src *bufio.Reader, tokens chan<- token) lexFn {
	b := make([]byte, 1, 2)
	src.Read(b)
	for {
		r, _, err := src.ReadRune()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			tokens <- token{
				Kind:  badToken,
				Value: string(b),
				Err:   err,
			}
			return nil
		}
		b = append(b, string(r)...)
		if r == '\\' {
			continue
		}
		if r == '"' {
			return lexsend(err, tokens, token{Kind: stringToken, Value: string(b)})
		}
	}
}
