package bencode

import (
	"crypto/sha1"
	"fmt"
	"runtime"
)

func HashInfo(b []byte) ([20]byte, error) {
	l := lex(string(b))
	t := token{}
	info := ""
	depth := -1
	for t = <-l.tokens; t.typ != tokenEOF && t.typ != tokenError; t = <-l.tokens {
		if depth >= 0 {
			info += t.val
		}
		if t.typ == tokenSuffix && t.depth == depth {
			depth = -1
		}
		if t.typ == tokenString && t.val == "info" {
			depth = t.depth
		}
	}
	if t.typ == tokenError {
		return [20]byte{}, fmt.Errorf("lexer: %s", l.err)
	}
	return sha1.Sum([]byte(info)), nil
}

func UnmarshalDict(b []byte) (d map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)

		}
	}()

	d = parseDictTop(lex(string(b)))
	err = nil
	return
}
