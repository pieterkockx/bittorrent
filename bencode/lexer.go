package bencode

import (
	"fmt"
	"strconv"
	"strings"
)

type tokenType int

const (
	tokenError tokenType = iota - 1
	tokenEOF
	tokenDict
	tokenInt
	tokenIntPrefix
	tokenList
	tokenString
	tokenStringLength
	tokenSemicolon
	tokenSuffix
)

type token struct {
	typ   tokenType
	val   string
	depth int
}

var tokenTypeToString = map[tokenType]string{
	tokenError:        "error",
	tokenEOF:          "EOF",
	tokenDict:         "dictionary",
	tokenInt:          "integer",
	tokenIntPrefix:    "integer prefix",
	tokenList:         "list",
	tokenString:       "string",
	tokenStringLength: "string length",
	tokenSemicolon:    "semicolon",
	tokenSuffix:       "suffix",
}

func (t tokenType) String() string {
	return tokenTypeToString[t]
}

func (t token) String() string {
	s := ""
	for i := 0; i < t.depth; i++ {
		s += " "
	}
	return fmt.Sprintf("%s%s", s, t.val)
}

type lexer struct {
	err    string
	input  string
	depth  int
	start  int
	pos    int
	tokens chan token
}

func (l *lexer) emit(t tokenType) {
	if t == tokenError {
		l.tokens <- token{t, l.err, l.depth}
	} else {
		l.tokens <- token{t, l.input[l.start:l.pos], l.depth}
	}
	l.start = l.pos
}

func (l *lexer) run() {
	for {
		if l.start >= len(l.input) {
			if l.depth > 0 {
				l.err = "unmatched prefix"
				l.emit(tokenError)
				return
			}
			l.emit(tokenEOF)
			return
		}
		switch r := l.input[l.start]; {
		case r == 'd':
			l.pos = l.start + 1
			l.emit(tokenDict)
			l.depth++
			continue
		case r == 'l':
			l.pos = l.start + 1
			l.emit(tokenList)
			l.depth++
			continue
		case r == 'e':
			l.pos = l.start + 1
			l.depth--
			if l.depth < 0 {
				l.err = fmt.Sprintf("unmatched suffix at position %d", l.start)
				l.emit(tokenError)
				return
			}
			l.emit(tokenSuffix)
			continue
		case r == 'i':
			if len(l.input[l.start:]) < 3 {
				l.err = fmt.Sprintf("invalid integer %q at position %d", string(l.input[l.start:]), l.start)
				l.emit(tokenError)
				return
			}
			l.pos = l.start + 1
			l.emit(tokenIntPrefix)
			n := strings.Index(l.input[l.start:], "e")
			if n == -1 {
				l.err = fmt.Sprintf("integer %.10q at position %d not followed by a suffix", string(l.input[l.start:]), l.start)
				l.emit(tokenError)
				return
			}
			if !strings.HasPrefix(l.input[l.start:], "0e") && l.input[l.start] == '0' {
				l.err = fmt.Sprintf("leading zero in integer %q at position %d", string(l.input[l.start:l.start+n]), l.start)
				l.emit(tokenError)
				return
			}
			if strings.TrimLeft(l.input[l.start:l.start+n], "0123456789") != "" {
				l.err = fmt.Sprintf("integer %q not in decimal at position %d", string(l.input[l.start:l.start+n]), l.start)
				l.emit(tokenError)
				return
			}
			l.pos = l.start + n
			l.depth++
			l.emit(tokenInt)
			continue
		default:
			if r >= '0' && r <= '9' {
				i := strings.Index(l.input[l.start:], ":")
				if i == -1 {
					l.err = fmt.Sprintf("string length %.10q at position %d not followed by a semicolon", string(l.input[l.start:]), l.start)
					l.emit(tokenError)
					return
				}
				n, err := strconv.ParseInt(l.input[l.start:l.start+i], 10, 0)
				if err != nil {
					l.err = fmt.Sprintf("parsing string length at position %d", l.start)
					l.emit(tokenError)
					return
				}
				l.pos = l.start + i
				l.emit(tokenStringLength)
				l.pos++
				l.emit(tokenSemicolon)
				if l.start + int(n) > len(l.input) {
					fmt.Println(l.input[l.start:])
					l.err = fmt.Sprintf("string length prefix of %d at position %d longer than remaining input", int(n), l.start)
					l.emit(tokenError)
					return
				}
				l.pos = l.start + int(n)
				l.emit(tokenString)
				continue
			} else {
				l.err = fmt.Sprintf("unexpected rune '%c' at position %d", r, l.start)
				l.emit(tokenError)
				return
			}
		}
	}
}

func lex(input string) *lexer {
	l := &lexer{
		input:  input,
		tokens: make(chan token),
	}
	go l.run()
	return l
}
