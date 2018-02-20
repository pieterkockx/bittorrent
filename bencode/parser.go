package bencode

import (
	"fmt"
	"strconv"
)

func expect(t token, err string, typ tokenType) token {
	if t.typ == tokenError {
		panic(fmt.Errorf("lexer: %s", err))
	} else if t.typ != typ {
		panic(fmt.Errorf("parser: expected %s, got %s", typ, t.typ))
	}
	return t
}

func parseInt(l *lexer) int64 {
	t := expect(<-l.tokens, l.err, tokenInt)
	i, _ := strconv.ParseInt(t.val, 10, 64)
	expect(<-l.tokens, l.err, tokenSuffix)
	return i
}

func parseString(l *lexer) string {
	expect(<-l.tokens, l.err, tokenSemicolon)
	t := expect(<-l.tokens, l.err, tokenString)
	return t.val
}

func parseDict(l *lexer, m map[string]interface{}) map[string]interface{} {
	for {
		t := <-l.tokens
		if t.typ == tokenSuffix {
			break
		}
		expect(t, l.err, tokenStringLength)
		key := parseString(l)
		t = <-l.tokens
		m[key] = parseNext(l, t)
	}
	return m
}

func parseList(l *lexer, m []interface{}) []interface{} {
	for {
		t := <-l.tokens
		if t.typ == tokenSuffix {
			break
		}
		m = append(m, parseNext(l, t))
	}
	return m
}

func parseNext(l *lexer, t token) interface{} {
	switch t.typ {
	case tokenIntPrefix:
		return parseInt(l)
	case tokenStringLength:
		return parseString(l)
	case tokenDict:
		return parseDict(l, make(map[string]interface{}))
	case tokenList:
		return parseList(l, make([]interface{}, 0))
	default:
		expect(t, l.err, tokenError)
	}
	panic("unreachable")
}

func parseDictTop(l *lexer) map[string]interface{} {
	expect(<-l.tokens, l.err, tokenDict)
	d := parseDict(l, make(map[string]interface{}))
	expect(<-l.tokens, l.err, tokenEOF)
	return d
}
