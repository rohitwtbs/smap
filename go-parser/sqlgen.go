package main

import (
	"fmt"
	"strings"
)

// escapeString mirrors smap.archiver.data.escape_string (psycopg2
// QuotedString): backslashes doubled, single quotes doubled, wrapped
// in single quotes.
func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'"
}

// AuthContext carries the request credentials that build_authcheck reads.
type AuthContext struct {
	Keys    []string
	Private bool
}

// buildAuthcheck mirrors querygen.build_authcheck for the archiver's
// configuration (permissions feature disabled).
func buildAuthcheck(auth AuthContext, ti string, forcePrivate bool) string {
	query := "sub.id = s.subscription_id AND "
	if !auth.Private && !forcePrivate {
		query += fmt.Sprintf("(sub%s.public ", ti)
	} else {
		query += "(false "
	}
	if len(auth.Keys) > 0 {
		parts := make([]string, len(auth.Keys))
		for i, k := range auth.Keys {
			parts[i] = fmt.Sprintf("sub.key = %s", escapeString(k+ti))
		}
		query += "OR ( (" + strings.Join(parts, " OR ") + ") )"
	}
	query += ")"
	return query
}

// makeSelectSQL mirrors make_select_rv's SQL string exactly,
// including its internal whitespace.
func makeSelectSQL(selectClause, whereStmt string, auth AuthContext) string {
	return fmt.Sprintf(`SELECT %s FROM stream s, subscription sub
              WHERE (%s) AND (%s)`, selectClause, whereStmt, buildAuthcheck(auth, "", false))
}

// ---- where-clause AST (mirrors smap.archiver.ast.Statement) ----

type StmtOp int

const (
	OpUUID StmtOp = iota
	OpHas
	OpContains
	OpEquals
	OpLike
	OpRegex
	OpAnd
	OpOr
	OpNot
)

type Stmt struct {
	Op       StmtOp
	Args     []string // escaped (leaf ops), mirrors Statement.__init__
	Children []*Stmt
}

func leafStmt(op StmtOp, args ...string) *Stmt {
	esc := make([]string, len(args))
	for i, a := range args {
		esc[i] = escapeString(a)
	}
	return &Stmt{Op: op, Args: esc}
}

func (s *Stmt) Render() string {
	switch s.Op {
	case OpUUID:
		if len(s.Args) > 0 {
			// args[0] is the escaped operator, unquoted via [1:-1]
			return fmt.Sprintf("s.uuid %s %s", s.Args[0][1:len(s.Args[0])-1], s.Args[1])
		}
		return "s.uuid IS NOT NULL"
	case OpHas:
		return fmt.Sprintf("s.metadata ? %s", s.Args[0])
	case OpContains:
		return fmt.Sprintf("CAST(avals(s.metadata) AS text) ILIKE %s", s.Args[0])
	case OpEquals:
		q := fmt.Sprintf("(s.metadata -> %s) = %s", s.Args[0], s.Args[1])
		return fmt.Sprintf("(s.metadata ? %s) AND (%s)", s.Args[0], q)
	case OpLike:
		q := fmt.Sprintf("(s.metadata -> %s) LIKE %s", s.Args[0], s.Args[1])
		return fmt.Sprintf("(s.metadata ? %s) AND (%s)", s.Args[0], q)
	case OpRegex:
		q := fmt.Sprintf("(s.metadata -> %s) ~ %s", s.Args[0], s.Args[1])
		return fmt.Sprintf("(s.metadata ? %s) AND (%s)", s.Args[0], q)
	case OpAnd:
		parts := make([]string, len(s.Children))
		for i, c := range s.Children {
			parts[i] = "(" + c.Render() + ")"
		}
		return strings.Join(parts, " AND ")
	case OpOr:
		parts := make([]string, len(s.Children))
		for i, c := range s.Children {
			parts[i] = "(" + c.Render() + ")"
		}
		return strings.Join(parts, " OR ")
	case OpNot:
		return fmt.Sprintf("NOT (%s)", s.Children[0].Render())
	}
	return ""
}

// addFormulaRestrictions mirrors queryparse.add_formula_restrictions.
// Restrictions are (key, value) pairs; duplicates are deduplicated
// preserving first-seen order (Python uses SetDict: dict of sets).
func addFormulaRestrictions(cRestrict string, fRestrict [][2]string) string {
	// dedupe like SetDict (dict keyed insertion order; set semantics per key)
	type kv struct{ k, v string }
	seen := map[kv]bool{}
	keyOrder := []string{}
	perKey := map[string][]string{}
	for _, r := range fRestrict {
		p := kv{r[0], r[1]}
		if seen[p] {
			continue
		}
		seen[p] = true
		if _, ok := perKey[r[0]]; !ok {
			keyOrder = append(keyOrder, r[0])
		}
		perKey[r[0]] = append(perKey[r[0]], r[1])
	}
	var extra []string
	for _, k := range keyOrder {
		for _, v := range perKey[k] {
			if k != "uuid" {
				extra = append(extra, fmt.Sprintf("((s.metadata -> %s) ~ %s)",
					escapeString(k), escapeString(v)))
			} else {
				extra = append(extra, fmt.Sprintf("(s.uuid = %s)", escapeString(v)))
			}
		}
	}
	if len(extra) > 0 {
		return cRestrict + " AND (" + strings.Join(extra, " OR ") + ")"
	}
	return cRestrict
}
