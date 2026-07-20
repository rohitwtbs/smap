package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Token types mirroring the PLY lexer in queryparse.py
type TokType int

const (
	TEOF TokType = iota
	TLTE
	TGTE
	TNE
	TQSTRING
	TNUMBER
	TLVALUE
	TKEYWORD // reserved words: value holds lowercase keyword
	TLIT     // single-char literal from ()[]*^.,<>=+-/
)

type Token struct {
	Type TokType
	Str  string  // for QSTRING, LVALUE, KEYWORD, LIT
	Num  float64 // for NUMBER
	IsInt bool
	Pos  int
}

var reserved = map[string]bool{
	"where": true, "distinct": true, "select": true, "delete": true,
	"set": true, "tags": true, "has": true, "and": true, "or": true,
	"not": true, "like": true, "data": true, "in": true, "before": true,
	"after": true, "now": true, "limit": true, "streamlimit": true,
	"apply": true, "to": true, "as": true, "group": true, "by": true,
	"help": true, "all": true, "contains": true,
}

const literals = "()[]*^.,<>=+-/"

type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return e.Msg }

func perr(format string, args ...interface{}) *ParseError {
	return &ParseError{Msg: fmt.Sprintf(format, args...)}
}

func isLvalStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '~' || c == '$' || c == '_'
}

func isLvalCont(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '/' || c == '%' || c == '_' || c == '-'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lex tokenizes the query following the same match order as the PLY
// lexer: CMP, QSTRING, LVALUE, NUMBER, then single-char literals.
func lex(input string) ([]Token, error) {
	var toks []Token
	i := 0
	n := len(input)
	for i < n {
		c := input[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		// t_CMP: <= >= !=
		if i+1 < n {
			two := input[i : i+2]
			switch two {
			case "<=":
				toks = append(toks, Token{Type: TLTE, Str: "<=", Pos: i})
				i += 2
				continue
			case ">=":
				toks = append(toks, Token{Type: TGTE, Str: ">=", Pos: i})
				i += 2
				continue
			case "!=":
				toks = append(toks, Token{Type: TNE, Str: "!=", Pos: i})
				i += 2
				continue
			}
		}
		// t_QSTRING
		if c == '"' || c == '\'' {
			q := c
			j := i + 1
			var sb strings.Builder
			closed := false
			for j < n {
				if input[j] == '\\' && j+1 < n {
					// PLY regex allows escapes; python only unescapes \" or \'
					if input[j+1] == q {
						sb.WriteByte(q)
						j += 2
						continue
					}
					sb.WriteByte(input[j])
					j++
					continue
				}
				if input[j] == q {
					closed = true
					j++
					break
				}
				sb.WriteByte(input[j])
				j++
			}
			if !closed {
				return nil, perr("Syntax error at '%s'", string(q))
			}
			toks = append(toks, Token{Type: TQSTRING, Str: sb.String(), Pos: i})
			i = j
			continue
		}
		// t_LVALUE (checked before NUMBER, as in PLY definition order)
		if isLvalStart(c) {
			j := i + 1
			for j < n && isLvalCont(input[j]) {
				j++
			}
			word := input[i:j]
			if word == "~" {
				toks = append(toks, Token{Type: TKEYWORD, Str: "~", Pos: i})
			} else if reserved[strings.ToLower(word)] && word == strings.ToLower(word) {
				toks = append(toks, Token{Type: TKEYWORD, Str: word, Pos: i})
			} else {
				toks = append(toks, Token{Type: TLVALUE, Str: word, Pos: i})
			}
			i = j
			continue
		}
		// t_NUMBER: ([+-]?([0-9]*\.)?[0-9]+)
		if isDigit(c) || ((c == '+' || c == '-') && i+1 < n && (isDigit(input[i+1]) || (input[i+1] == '.' && i+2 < n && isDigit(input[i+2])))) || (c == '.' && i+1 < n && isDigit(input[i+1])) {
			j := i
			if input[j] == '+' || input[j] == '-' {
				j++
			}
			for j < n && isDigit(input[j]) {
				j++
			}
			isFloat := false
			if j < n && input[j] == '.' {
				k := j + 1
				if k < n && isDigit(input[k]) {
					isFloat = true
					j = k
					for j < n && isDigit(input[j]) {
						j++
					}
				}
			}
			numstr := input[i:j]
			val, err := strconv.ParseFloat(numstr, 64)
			if err != nil {
				return nil, perr("Syntax error at '%s'", numstr)
			}
			toks = append(toks, Token{Type: TNUMBER, Num: val, IsInt: !isFloat, Pos: i})
			i = j
			continue
		}
		// literals
		if strings.IndexByte(literals, c) >= 0 {
			toks = append(toks, Token{Type: TLIT, Str: string(c), Pos: i})
			i++
			continue
		}
		return nil, perr("Illegal character '%s'", string(c))
	}
	toks = append(toks, Token{Type: TEOF, Pos: n})
	return toks, nil
}
