package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ---- results ----

type DataSpec struct {
	Method      string  `json:"method"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Limit       float64 `json:"limit"`
	StreamLimit float64 `json:"streamlimit"`
}

type Extract struct {
	Type string   `json:"type"`           // recursive | plural | nonnull
	Tags []string `json:"tags,omitempty"` // for plural, aligned with select columns
}

type ParseResult struct {
	Kind     string                 `json:"kind"` // select | data | apply | unsupported
	SQL      string                 `json:"sql,omitempty"`
	Extract  *Extract               `json:"extract,omitempty"`
	DataSpec *DataSpec              `json:"dataSpec,omitempty"`
	TagSQL   string                 `json:"tagSql,omitempty"`
	DataSQL  string                 `json:"dataSql,omitempty"`
	Plan     map[string]interface{} `json:"plan,omitempty"`
	Group    []string               `json:"group,omitempty"`
	Reason   string                 `json:"reason,omitempty"`
}

// ---- parser ----

type Parser struct {
	toks []Token
	pos  int
	auth AuthContext
}

func NewParser(query string, auth AuthContext) (*Parser, error) {
	toks, err := lex(query)
	if err != nil {
		return nil, err
	}
	return &Parser{toks: toks, auth: auth}, nil
}

func (p *Parser) peek() Token  { return p.toks[p.pos] }
func (p *Parser) next() Token  { t := p.toks[p.pos]; p.pos++; return t }
func (p *Parser) isKw(s string) bool {
	t := p.peek()
	return t.Type == TKEYWORD && t.Str == s
}
func (p *Parser) isLit(s string) bool {
	t := p.peek()
	return t.Type == TLIT && t.Str == s
}
func (p *Parser) expectKw(s string) error {
	if !p.isKw(s) {
		return p.synErr()
	}
	p.next()
	return nil
}
func (p *Parser) expectLit(s string) error {
	if !p.isLit(s) {
		return p.synErr()
	}
	p.next()
	return nil
}
func (p *Parser) synErr() error {
	t := p.peek()
	if t.Type == TEOF {
		return perr("Syntax error: unexpected end of query")
	}
	var v string
	switch t.Type {
	case TNUMBER:
		v = trimFloat(t.Num, t.IsInt)
	default:
		v = t.Str
	}
	return perr("Syntax error at '%s'", v)
}

func trimFloat(f float64, isInt bool) string {
	if isInt {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// Parse parses a full query.
func (p *Parser) Parse() (*ParseResult, error) {
	t := p.peek()
	if t.Type != TKEYWORD {
		return nil, p.synErr()
	}
	switch t.Str {
	case "select":
		p.next()
		return p.parseSelect()
	case "apply":
		p.next()
		return p.parseApply()
	case "delete", "set", "help":
		return &ParseResult{Kind: "unsupported", Reason: t.Str + " statements are handled by the Python parser"}, nil
	default:
		return nil, p.synErr()
	}
}

func (p *Parser) atEOF() bool { return p.peek().Type == TEOF }

// ---- select ----

func (p *Parser) parseSelect() (*ParseResult, error) {
	// data clause?
	if p.isKw("data") {
		ds, err := p.parseDataClause()
		if err != nil {
			return nil, err
		}
		if err := p.expectKw("where"); err != nil {
			return nil, err
		}
		st, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if !p.atEOF() {
			return nil, p.synErr()
		}
		sql := makeSelectSQL("distinct(s.uuid), s.id", st.Render(), p.auth)
		return &ParseResult{Kind: "data", SQL: sql, DataSpec: ds}, nil
	}

	sel, ext, err := p.parseSelector()
	if err != nil {
		return nil, err
	}
	where := "true"
	if p.isKw("where") {
		p.next()
		st, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		where = st.Render()
	}
	if !p.atEOF() {
		return nil, p.synErr()
	}
	sql := makeSelectSQL(sel, where, p.auth)
	return &ParseResult{Kind: "select", SQL: sql, Extract: ext}, nil
}

// parseSelector handles '*', DISTINCT [LVALUE], and tag lists.
func (p *Parser) parseSelector() (string, *Extract, error) {
	if p.isLit("*") {
		p.next()
		return "s.metadata || hstore('uuid', s.uuid)", &Extract{Type: "recursive"}, nil
	}
	if p.isKw("distinct") {
		p.next()
		if p.peek().Type == TLVALUE {
			lv := p.next().Str
			if lv == "uuid" {
				return "DISTINCT s.uuid", &Extract{Type: "nonnull"}, nil
			}
			return fmt.Sprintf("DISTINCT (s.metadata -> %s)", escapeString(lv)),
				&Extract{Type: "nonnull"}, nil
		}
		return "DISTINCT skeys(s.metadata)", &Extract{Type: "nonnull"}, nil
	}
	tags, err := p.parseTagList()
	if err != nil {
		return "", nil, err
	}
	// mirror make_tag_select: if uuid requested, also select Path
	hasUUID := false
	hasPath := false
	for _, tg := range tags {
		if tg == "uuid" {
			hasUUID = true
		}
		if tg == "Path" {
			hasPath = true
		}
	}
	if hasUUID && !hasPath {
		tags = append(tags, "Path")
	}
	cols := make([]string, len(tags))
	for i, tg := range tags {
		if tg == "uuid" {
			cols[i] = "(s.uuid)"
		} else {
			cols[i] = fmt.Sprintf("(s.metadata -> %s)", escapeString(tg))
		}
	}
	return strings.Join(cols, ", "), &Extract{Type: "plural", Tags: tags}, nil
}

// parseTagList parses LVALUE (',' LVALUE)* deduplicated (Python uses a set).
func (p *Parser) parseTagList() ([]string, error) {
	if p.peek().Type != TLVALUE {
		return nil, p.synErr()
	}
	var tags []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			tags = append(tags, s)
		}
	}
	add(p.next().Str)
	for p.isLit(",") {
		p.next()
		if p.peek().Type != TLVALUE {
			return nil, p.synErr()
		}
		add(p.next().Str)
	}
	return tags, nil
}

// ---- where statements ----
// PLY precedence: AND binds *looser* than OR, NOT binds tightest.

func (p *Parser) parseStatement() (*Stmt, error) {
	left, err := p.parseOrLevel()
	if err != nil {
		return nil, err
	}
	for p.isKw("and") {
		p.next()
		right, err := p.parseOrLevel()
		if err != nil {
			return nil, err
		}
		left = &Stmt{Op: OpAnd, Children: []*Stmt{left, right}}
	}
	return left, nil
}

func (p *Parser) parseOrLevel() (*Stmt, error) {
	left, err := p.parseNotLevel()
	if err != nil {
		return nil, err
	}
	for p.isKw("or") {
		p.next()
		right, err := p.parseNotLevel()
		if err != nil {
			return nil, err
		}
		left = &Stmt{Op: OpOr, Children: []*Stmt{left, right}}
	}
	return left, nil
}

func (p *Parser) parseNotLevel() (*Stmt, error) {
	if p.isKw("not") {
		p.next()
		inner, err := p.parseNotLevel()
		if err != nil {
			return nil, err
		}
		return &Stmt{Op: OpNot, Children: []*Stmt{inner}}, nil
	}
	return p.parseStatementPrimary()
}

func (p *Parser) parseStatementPrimary() (*Stmt, error) {
	if p.isLit("(") {
		p.next()
		st, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if err := p.expectLit(")"); err != nil {
			return nil, err
		}
		return st, nil
	}
	if p.isKw("has") {
		p.next()
		if p.peek().Type != TLVALUE {
			return nil, p.synErr()
		}
		lv := p.next().Str
		if lv == "uuid" {
			return &Stmt{Op: OpUUID}, nil
		}
		return leafStmt(OpHas, lv), nil
	}
	if p.isKw("contains") {
		p.next()
		if p.peek().Type != TQSTRING {
			return nil, p.synErr()
		}
		return leafStmt(OpContains, p.next().Str), nil
	}
	if p.peek().Type == TLVALUE {
		lv := p.next().Str
		var op string
		if p.isLit("=") {
			op = "="
		} else if p.isKw("like") {
			op = "like"
		} else if p.isKw("~") {
			op = "~"
		} else {
			return nil, p.synErr()
		}
		p.next()
		if p.peek().Type != TQSTRING {
			return nil, p.synErr()
		}
		val := p.next().Str
		if lv == "uuid" {
			return leafStmt(OpUUID, op, val), nil
		}
		switch op {
		case "=":
			return leafStmt(OpEquals, lv, val), nil
		case "like":
			return leafStmt(OpLike, lv, val), nil
		default:
			return leafStmt(OpRegex, lv, val), nil
		}
	}
	return nil, p.synErr()
}

// ---- data clause ----

var laLoc *time.Location

func init() {
	var err error
	laLoc, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		laLoc = time.UTC
	}
}

// parseTimeString mirrors queryparse.parse_time (America/Los_Angeles).
func parseTimeString(s string) (time.Time, error) {
	layouts := []string{"1/2/2006", "1/2/2006 15:04", "2006-01-02T15:04:05"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, laLoc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, perr("Invalid time string:%s", s)
}

// parseTimeref parses abstime [reltime...]; returns epoch ms (seconds
// floored to whole seconds like dt2ts, then *1000).
func (p *Parser) parseTimeref() (float64, error) {
	var secs float64
	t := p.peek()
	switch {
	case t.Type == TKEYWORD && t.Str == "now":
		p.next()
		secs = float64(time.Now().UnixNano()) / 1e9
	case t.Type == TNUMBER:
		p.next()
		secs = t.Num / 1000.0
	case t.Type == TQSTRING:
		p.next()
		tm, err := parseTimeString(t.Str)
		if err != nil {
			return 0, err
		}
		secs = float64(tm.Unix())
	default:
		return 0, p.synErr()
	}
	// reltime: NUMBER LVALUE ...
	for p.peek().Type == TNUMBER && p.toks[p.pos+1].Type == TLVALUE {
		numTok := p.next()
		unitTok := p.next()
		mult, err := timeunitSeconds(unitTok.Str)
		if err != nil {
			return 0, err
		}
		secs += numTok.Num * mult
	}
	// dt2ts uses utctimetuple() which floors to whole seconds
	return math.Floor(secs) * 1000, nil
}

func timeunitSeconds(u string) (float64, error) {
	switch {
	case u == "d" || u == "day" || u == "days":
		return 86400, nil
	case u == "h" || u == "hour" || u == "hours":
		return 3600, nil
	case u == "m" || u == "minute" || u == "minutes":
		return 60, nil
	case u == "s" || u == "second" || u == "seconds":
		return 1, nil
	}
	return 0, perr("Invalid timeunit: %s", u)
}

// parseLimit parses [LIMIT n] [STREAMLIMIT n]; returns (limit, slimit)
// with limit==nil when unset.
func (p *Parser) parseLimit() (*float64, float64, error) {
	var limit *float64
	slimit := 1000.0
	if p.isKw("limit") {
		p.next()
		if p.peek().Type != TNUMBER {
			return nil, 0, p.synErr()
		}
		v := p.next().Num
		limit = &v
		if p.isKw("streamlimit") {
			p.next()
			if p.peek().Type != TNUMBER {
				return nil, 0, p.synErr()
			}
			slimit = p.next().Num
		}
	} else if p.isKw("streamlimit") {
		p.next()
		if p.peek().Type != TNUMBER {
			return nil, 0, p.synErr()
		}
		slimit = p.next().Num
	}
	return limit, slimit, nil
}

// parseDataClause parses DATA IN/BEFORE/AFTER ... limit.
func (p *Parser) parseDataClause() (*DataSpec, error) {
	if err := p.expectKw("data"); err != nil {
		return nil, err
	}
	var method string
	var start, end float64
	if p.isKw("in") {
		p.next()
		method = "data"
		paren := false
		if p.isLit("(") {
			paren = true
			p.next()
		}
		var err error
		start, err = p.parseTimeref()
		if err != nil {
			return nil, err
		}
		if err := p.expectLit(","); err != nil {
			return nil, err
		}
		end, err = p.parseTimeref()
		if err != nil {
			return nil, err
		}
		if paren {
			if err := p.expectLit(")"); err != nil {
				return nil, err
			}
		}
		limit, slimit, err := p.parseLimit()
		if err != nil {
			return nil, err
		}
		lim := 10000.0
		if limit != nil {
			lim = *limit
		}
		if lim == -1 {
			lim = 1e7
		}
		return &DataSpec{Method: method, Start: start, End: end, Limit: lim, StreamLimit: slimit}, nil
	}
	if p.isKw("before") || p.isKw("after") {
		if p.isKw("before") {
			method = "prev"
		} else {
			method = "next"
		}
		p.next()
		var err error
		start, err = p.parseTimeref()
		if err != nil {
			return nil, err
		}
		end = 0
		limit, slimit, err := p.parseLimit()
		if err != nil {
			return nil, err
		}
		lim := 1.0
		if limit != nil {
			lim = *limit
		}
		if lim == -1 {
			lim = 1e7
		}
		return &DataSpec{Method: method, Start: start, End: end, Limit: lim, StreamLimit: slimit}, nil
	}
	return nil, p.synErr()
}

// ---- apply ----

// Formula plan node: {"op": name, "args": [...], "kwargs": {...},
// "children": [...]} — a structural description only; the Python side
// binds real operator classes.
type Formula struct {
	Plan     map[string]interface{}
	Restrict [][2]string
	IsNumber bool
	Num      float64
}

func opNode(name string, args []interface{}, kwargs map[string]interface{}, children ...map[string]interface{}) map[string]interface{} {
	n := map[string]interface{}{"op": name}
	if len(args) > 0 {
		n["args"] = args
	}
	if len(kwargs) > 0 {
		n["kwargs"] = kwargs
	}
	if len(children) > 0 {
		n["children"] = children
	}
	return n
}

func (p *Parser) parseApply() (*ParseResult, error) {
	f, mapping, err := p.parseFormulaPipe()
	if err != nil {
		return nil, err
	}
	_ = mapping
	if err := p.expectKw("to"); err != nil {
		return nil, err
	}
	ds, err := p.parseDataClause()
	if err != nil {
		return nil, err
	}
	if err := p.expectKw("where"); err != nil {
		return nil, err
	}
	st, err := p.parseStatement()
	if err != nil {
		return nil, err
	}
	var group []string
	if p.isKw("group") {
		p.next()
		if err := p.expectKw("by"); err != nil {
			return nil, err
		}
		group, err = p.parseTagList()
		if err != nil {
			return nil, err
		}
	}
	if !p.atEOF() {
		return nil, p.synErr()
	}
	restrict := addFormulaRestrictions(st.Render(), f.Restrict)
	tagSQL := makeSelectSQL("s.metadata || hstore('uuid', s.uuid)", restrict, p.auth)
	dataSQL := makeSelectSQL("distinct(s.uuid), s.id", restrict, p.auth)
	return &ParseResult{
		Kind: "apply", TagSQL: tagSQL, DataSQL: dataSQL,
		Plan: f.Plan, DataSpec: ds, Group: group,
	}, nil
}

// parseFormulaPipe: formula ('<' formula_pipe)?  (right-assoc), with
// rename-restriction post-processing like rename_restrictions().
func (p *Parser) parseFormulaPipe() (*Formula, map[string]string, error) {
	left, err := p.parseFormulaCmp()
	if err != nil {
		return nil, nil, err
	}
	var tree map[string]interface{}
	var restrict [][2]string
	mapping := map[string]string{}
	if p.isLit("<") {
		p.next()
		right, rmapping, err := p.parseFormulaPipe()
		if err != nil {
			return nil, nil, err
		}
		tree = opNode("pipe", nil, nil, left.Plan, right.Plan)
		restrict = append(append([][2]string{}, left.Restrict...), right.Restrict...)
		mapping = rmapping
	} else {
		tree = left.Plan
		restrict = left.Restrict
	}
	// rename_restrictions
	var newTags [][2]string
	for i := len(restrict) - 1; i >= 0; i-- {
		name, value := restrict[i][0], restrict[i][1]
		if name == "rename" {
			// value encodes "old\x00new"
			parts := strings.SplitN(value, "\x00", 2)
			old, nw := parts[0], parts[1]
			if mapped, ok := mapping[old]; ok {
				mapping[nw] = mapped
				delete(mapping, old)
			} else {
				mapping[nw] = old
			}
		} else if mapped, ok := mapping[name]; ok {
			newTags = append(newTags, [2]string{mapped, value})
		} else {
			newTags = append(newTags, [2]string{name, value})
		}
	}
	// reverse
	for i, j := 0, len(newTags)-1; i < j; i, j = i+1, j-1 {
		newTags[i], newTags[j] = newTags[j], newTags[i]
	}
	return &Formula{Plan: tree, Restrict: newTags}, mapping, nil
}

// comparator level: formula cmp NUMBER | NUMBER cmp formula
func (p *Parser) parseFormulaCmp() (*Formula, error) {
	left, err := p.parseFormulaAdd()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	isCmp := (t.Type == TLIT && t.Str == ">") || t.Type == TLTE || t.Type == TGTE || t.Type == TNE
	if !isCmp {
		return left, nil
	}
	cmpName := map[string]string{">": "greater", "<=": "less_equal", ">=": "greater_equal", "!=": "not_equal"}
	var opname string
	if t.Type == TLIT {
		opname = cmpName[t.Str]
	} else {
		opname = cmpName[t.Str]
	}
	p.next()
	if left.IsNumber {
		// NUMBER comparator formula: python leaves t[0] unset (None)
		right, err := p.parseFormulaAdd()
		_ = right
		if err != nil {
			return nil, err
		}
		return &Formula{Plan: nil}, nil
	}
	if p.peek().Type != TNUMBER {
		return nil, p.synErr()
	}
	n := p.next().Num
	inner := opNode(opname, []interface{}{n}, nil, opNode("null", nil, nil))
	return &Formula{
		Plan:     opNode("nonzero", []interface{}{inner}, nil, left.Plan),
		Restrict: left.Restrict,
	}, nil
}

// '+' level (lowest arithmetic)
func (p *Parser) parseFormulaAdd() (*Formula, error) {
	left, err := p.parseFormulaSub()
	if err != nil {
		return nil, err
	}
	for p.isLit("+") {
		p.next()
		right, err := p.parseFormulaSub()
		if err != nil {
			return nil, err
		}
		left = combineArith(left, right, "add", "sum")
	}
	return left, nil
}

func (p *Parser) parseFormulaSub() (*Formula, error) {
	left, err := p.parseFormulaMul()
	if err != nil {
		return nil, err
	}
	for p.isLit("-") {
		p.next()
		right, err := p.parseFormulaMul()
		if err != nil {
			return nil, err
		}
		left = combineSub(left, right)
	}
	return left, nil
}

func (p *Parser) parseFormulaMul() (*Formula, error) {
	left, err := p.parseFormulaPow()
	if err != nil {
		return nil, err
	}
	for p.isLit("*") || p.isLit("/") {
		op := p.next().Str
		if op == "/" {
			if p.peek().Type != TNUMBER {
				return nil, p.synErr()
			}
			n := p.next().Num
			left = &Formula{
				Plan:     opNode("multiply", []interface{}{1.0 / n}, nil, left.Plan),
				Restrict: left.Restrict,
			}
			continue
		}
		right, err := p.parseFormulaPow()
		if err != nil {
			return nil, err
		}
		left = combineArith(left, right, "multiply", "product")
	}
	return left, nil
}

func (p *Parser) parseFormulaPow() (*Formula, error) {
	left, err := p.parseFormulaOperand()
	if err != nil {
		return nil, err
	}
	for p.isLit("^") {
		p.next()
		if p.peek().Type != TNUMBER {
			return nil, p.synErr()
		}
		n := p.next().Num
		left = &Formula{
			Plan:     opNode("power", []interface{}{n}, nil, left.Plan),
			Restrict: left.Restrict,
		}
	}
	return left, nil
}

func combineArith(a, b *Formula, scalarOp, vectorOp string) *Formula {
	if a.IsNumber && b.IsNumber {
		var v float64
		if scalarOp == "add" {
			v = a.Num + b.Num
		} else {
			v = a.Num * b.Num
		}
		return &Formula{IsNumber: true, Num: v}
	}
	if a.IsNumber {
		return &Formula{
			Plan:     opNode(scalarOp, []interface{}{a.Num}, nil, b.Plan),
			Restrict: b.Restrict,
		}
	}
	if b.IsNumber {
		return &Formula{
			Plan:     opNode(scalarOp, []interface{}{b.Num}, nil, a.Plan),
			Restrict: a.Restrict,
		}
	}
	comp := opNode("compose", nil, nil,
		opNode("paste", nil, map[string]interface{}{"sort": nil}),
		opNode(vectorOp, nil, map[string]interface{}{"axis": 1}))
	return &Formula{
		Plan:     opNode("apply2", nil, nil, comp, a.Plan, b.Plan),
		Restrict: append(append([][2]string{}, a.Restrict...), b.Restrict...),
	}
}

func combineSub(a, b *Formula) *Formula {
	if a.IsNumber && b.IsNumber {
		return &Formula{IsNumber: true, Num: a.Num - b.Num}
	}
	if a.IsNumber {
		return &Formula{
			Plan:     opNode("add", []interface{}{a.Num}, nil, b.Plan),
			Restrict: b.Restrict,
		}
	}
	if b.IsNumber {
		return &Formula{
			Plan:     opNode("add", []interface{}{-b.Num}, nil, a.Plan),
			Restrict: a.Restrict,
		}
	}
	comp := opNode("compose", nil, nil,
		opNode("paste", nil, map[string]interface{}{"sort": nil}),
		opNode("diff", nil, map[string]interface{}{"axis": 1}))
	// note: python nests (b, a) for subtraction
	return &Formula{
		Plan:     opNode("apply2", nil, nil, comp, b.Plan, a.Plan),
		Restrict: append(append([][2]string{}, b.Restrict...), a.Restrict...),
	}
}

// parseFormulaOperand: NUMBER | LVALUE arg_clause where_clause | where_clause
func (p *Parser) parseFormulaOperand() (*Formula, error) {
	t := p.peek()
	if t.Type == TNUMBER {
		p.next()
		return &Formula{IsNumber: true, Num: t.Num}, nil
	}
	if t.Type == TLVALUE || (t.Type == TKEYWORD && t.Str == "all") {
		if t.Type == TKEYWORD {
			// ALL where-clause
			p.next()
			return &Formula{
				Plan: opNode("w", []interface{}{"uuid", ".*"}, nil),
			}, nil
		}
		name := p.next().Str
		args, kwargs, err := p.parseArgClause()
		if err != nil {
			return nil, err
		}
		sub, err := p.parseFormulaWhereClause()
		if err != nil {
			return nil, err
		}
		var restrict [][2]string
		if name == "rename" {
			// restrict [('rename', (old,new))] — args are (old, new)
			if len(args) >= 2 {
				o, _ := args[0].(string)
				nw, _ := args[1].(string)
				restrict = append(restrict, [2]string{"rename", o + "\x00" + nw})
			}
		}
		restrict = append(restrict, sub.Restrict...)
		return &Formula{
			Plan:     opNode(name, args, kwargs, sub.Plan),
			Restrict: restrict,
		}, nil
	}
	return p.parseFormulaWhereClause()
}

// parseFormulaWhereClause: '[' LVALUE '.' QSTRING ']' | '[' formula ']'
// | ALL | QSTRING | <empty>
func (p *Parser) parseFormulaWhereClause() (*Formula, error) {
	t := p.peek()
	if t.Type == TLIT && t.Str == "[" {
		p.next()
		// try LVALUE '.' QSTRING
		if p.peek().Type == TLVALUE && p.toks[p.pos+1].Type == TLIT && p.toks[p.pos+1].Str == "." {
			lv := p.next().Str
			p.next() // '.'
			if p.peek().Type != TQSTRING {
				return nil, p.synErr()
			}
			qs := p.next().Str
			if err := p.expectLit("]"); err != nil {
				return nil, err
			}
			return &Formula{
				Plan:     opNode("w", []interface{}{lv, qs}, nil),
				Restrict: [][2]string{{lv, qs}},
			}, nil
		}
		f, err := p.parseFormulaCmp()
		if err != nil {
			return nil, err
		}
		if err := p.expectLit("]"); err != nil {
			return nil, err
		}
		return f, nil
	}
	if t.Type == TKEYWORD && t.Str == "all" {
		p.next()
		return &Formula{Plan: opNode("w", []interface{}{"uuid", ".*"}, nil)}, nil
	}
	if t.Type == TQSTRING {
		p.next()
		return &Formula{
			Plan:     opNode("w", []interface{}{"x", t.Str}, nil),
			Restrict: [][2]string{{"x", t.Str}},
		}, nil
	}
	// empty → null operator
	return &Formula{Plan: opNode("null", nil, nil)}, nil
}

// parseArgClause: '(' arg_list ')' | <empty>
func (p *Parser) parseArgClause() ([]interface{}, map[string]interface{}, error) {
	args := []interface{}{}
	kwargs := map[string]interface{}{}
	if !p.isLit("(") {
		return args, kwargs, nil
	}
	p.next()
	for !p.isLit(")") {
		t := p.peek()
		switch {
		case t.Type == TQSTRING:
			p.next()
			args = append(args, t.Str)
		case t.Type == TNUMBER:
			p.next()
			if t.IsInt {
				args = append(args, int64(t.Num))
			} else {
				args = append(args, t.Num)
			}
		case t.Type == TLVALUE && p.toks[p.pos+1].Type == TLIT && p.toks[p.pos+1].Str == "=":
			key := p.next().Str
			p.next() // '='
			vt := p.peek()
			if vt.Type == TNUMBER {
				p.next()
				if vt.IsInt {
					kwargs[key] = int64(vt.Num)
				} else {
					kwargs[key] = vt.Num
				}
			} else if vt.Type == TQSTRING {
				p.next()
				kwargs[key] = vt.Str
			} else {
				return nil, nil, p.synErr()
			}
		default:
			// nested formula_pipe argument
			f, _, err := p.parseFormulaPipe()
			if err != nil {
				return nil, nil, err
			}
			args = append(args, f.Plan)
		}
		if p.isLit(",") {
			p.next()
			continue
		}
		break
	}
	if err := p.expectLit(")"); err != nil {
		return nil, nil, err
	}
	return args, kwargs, nil
}
