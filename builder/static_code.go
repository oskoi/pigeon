//go:generate go run ../bootstrap/cmd/static_code_generator/main.go -- $GOFILE generated_$GOFILE staticCode

//go:build static_code
// +build static_code

package builder

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// IMPORTANT: All code below this line is added to the parser as static code
var (
	// errNoRule is returned when the grammar to parse has no rule.
	errNoRule = errors.New("grammar has no rule")

	// errInvalidEntrypoint is returned when the specified entrypoint rule
	// does not exit.
	errInvalidEntrypoint = errors.New("invalid entrypoint")

	// errInvalidEncoding is returned when the source is not properly
	// utf8-encoded.
	errInvalidEncoding = errors.New("invalid encoding")

	// errMaxExprCnt is used to signal that the maximum number of
	// expressions have been parsed.
	errMaxExprCnt = errors.New("max number of expressions parsed")
)

// remove generic because it can't be compiled by gopherjs
type parserStack struct {
	data     []savepoint
	index    int
	size     int
}

func (ss *parserStack) init(size int) {
	ss.index = -1
	ss.data = make([]savepoint, size)
	ss.size = size
}

func (ss *parserStack) push(v *savepoint) {
	ss.index += 1
	if ss.index == ss.size {
		ss.data = append(ss.data, *v)
		ss.size = len(ss.data)
	} else {
		ss.data[ss.index] = *v
	}
}

func (ss *parserStack) pop() *savepoint {
	ref := &ss.data[ss.index]
	ss.index--
	return ref
}

func (ss *parserStack) top() *savepoint {
	return &ss.data[ss.index]
}

// option is a function that can set an option on the parser. It returns
// the previous setting as an option.
type option func(*parser) option

// ==template== {{ if not .Optimize }}
// statistics adds a user provided Stats struct to the parser to allow
// the user to process the results after the parsing has finished.
// Also the key for the "no match" counter is set.
//
// Example usage:
//
//	input := "input"
//	stats := Stats{}
//	_, err := Parse("input-file", []byte(input), Statistics(&stats, "no match"))
//	if err != nil {
//	    log.Panicln(err)
//	}
//	b, err := json.MarshalIndent(stats.ChoiceAltCnt, "", "  ")
//	if err != nil {
//	    log.Panicln(err)
//	}
//	fmt.Println(string(b))
func statistics(stats *Stats, choiceNoMatch string) option {
	return func(p *parser) option {
		oldStats := p.Stats
		p.Stats = stats
		oldChoiceNoMatch := p.choiceNoMatch
		p.choiceNoMatch = choiceNoMatch
		if p.Stats.ChoiceAltCnt == nil {
			p.Stats.ChoiceAltCnt = make(map[string]map[string]int)
		}
		return statistics(oldStats, oldChoiceNoMatch)
	}
}

// debug creates an option to set the debug flag to b. When set to true,
// debugging information is printed to stdout while parsing.
//
// The default is false.
func debug(b bool) option {
	return func(p *parser) option {
		old := p.debug
		p.debug = b
		return debug(old)
	}
}
// {{ end }} ==template==

// Parse parses the data from b using filename as information in the
// error messages.
func parse(filename string, b []byte, opts ...option) (any, error) {
	return newParser(filename, b, opts...).parse({{ .GrammarVarName }})
}

// position records a position in the text.
type position struct {
	line, col, offset int
}

func (p position) String() string {
	return strconv.Itoa(p.line) + ":" + strconv.Itoa(p.col) + " [" + strconv.Itoa(p.offset) + "]"
}

// savepoint stores all state required to go back to this point in the
// parser.
type savepoint struct {
	position
	rn rune
	w  int
}

type current struct {
	pos  position // start position of the match
	text []byte   // raw text of the match
	data *ParserCustomData
}

// the AST types...

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type grammar struct {
	// ==template== {{ if .SetRulePos }}
	pos         position
	// {{ end }} ==template==
	rules []*rule
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type rule struct {
	// ==template== {{ if .SetRulePos }}
	pos         position
	// {{ end }} ==template==
	name        string
	displayName string
	expr        any
	varExists   bool
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type choiceExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos          position
	// {{ end }} ==template==
	alternatives []any
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type actionExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	expr any
	run  func(*parser) any
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type recoveryExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	expr         any
	recoverExpr  any
	failureLabel []string
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type seqExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	exprs []any
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type throwExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	label string
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type labeledExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	label string
	expr  any
	textCapture bool
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type expr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	expr any
}

type (
	andExpr        expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
	notExpr        expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
	andLogicalExpr expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
	notLogicalExpr expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
	zeroOrOneExpr  expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
	zeroOrMoreExpr expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
	oneOrMoreExpr  expr //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
)

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type ruleRefExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	name string
}

// {{ if .IRefEnable }}
// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type ruleIRefExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	index int
}
// {{ end }} ==template==

// {{ if .IRefCodeEnable }}
// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type ruleIRefExprX struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	index int
	call func(p*parser, expr any) (any, bool)
}
// {{ end }} ==template==

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type andCodeExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	run func(*parser) bool
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type notCodeExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	run func(*parser) bool
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type litMatcher struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	val        string
	ignoreCase bool
	want       string
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type codeExpr struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	run func(*parser) any
	notSkip bool
}

// {{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
type charClassMatcher struct {
	// ==template== {{ if .SetRulePos }}
	pos position
	// {{ end }} ==template==
	val             string
	chars           []rune
	ranges          []rune
	classes         []*unicode.RangeTable
	ignoreCase      bool
	inverted        bool
}

// ==template== {{ if .SetRulePos }}
type anyMatcher position //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
// {{ else }} ==template==
type anyMatcher struct{} //{{ if .Nolint }} nolint: structcheck {{else}} ==template== {{ end }}
// {{ end }} ==template==

// errList cumulates the errors found by the parser.
type errList []error

func (e *errList) add(err error) {
	*e = append(*e, err)
}

func (e errList) err() error {
	if len(e) == 0 {
		return nil
	}
	e.dedupe()
	return e
}

func (e *errList) dedupe() {
	var cleaned []error
	set := make(map[string]bool)
	for _, err := range *e {
		if msg := err.Error(); !set[msg] {
			set[msg] = true
			cleaned = append(cleaned, err)
		}
	}
	*e = cleaned
}

func (e errList) Error() string {
	switch len(e) {
	case 0:
		return ""
	case 1:
		return e[0].Error()
	default:
		var buf bytes.Buffer

		for i, err := range e {
			if i > 0 {
				buf.WriteRune('\n')
			}
			buf.WriteString(err.Error())
		}
		return buf.String()
	}
}

// parserError wraps an error with a prefix indicating the rule in which
// the error occurred. The original error is stored in the Inner field.
type parserError struct {
	Inner    error
	pos      position
	prefix   string
	expected []string
}

// Error returns the error message.
func (p *parserError) Error() string {
	return p.prefix + ": " + p.Inner.Error()
}

// {{ if .Nolint }} nolint: structcheck,deadcode {{else}} ==template== {{ end }}
type resultTuple struct {
	v   any
	b   bool
	end savepoint
}

// {{ if .Nolint }} nolint: varcheck {{else}} ==template== {{ end }}
const choiceNoMatch = -1

// Stats stores some statistics, gathered during parsing
type Stats struct {
	// ExprCnt counts the number of expressions processed during parsing
	// This value is compared to the maximum number of expressions allowed
	// (set by the MaxExpressions option).
	ExprCnt uint64

	// ChoiceAltCnt is used to count for each ordered choice expression,
	// which alternative is used how may times.
	// These numbers allow to optimize the order of the ordered choice expression
	// to increase the performance of the parser
	//
	// The outer key of ChoiceAltCnt is composed of the exprType of the rule as well
	// as the line and the column of the ordered choice.
	// The inner key of ChoiceAltCnt is the number (one-based) of the matching alternative.
	// For each alternative the number of matches are counted. If an ordered choice does not
	// match, a special counter is incremented. The exprType of this counter is set with
	// the parser option Statistics.
	// For an alternative to be included in ChoiceAltCnt, it has to match at least once.
	ChoiceAltCnt map[string]map[string]int
}

// {{ if .Nolint }} nolint: structcheck,maligned {{else}} ==template== {{ end }}
type parser struct {
	filename string
	pt       savepoint
	cur      current

	data []byte
	errs *errList

	depth   int
	recover bool
	// ==template== {{ if not .Optimize }}
	debug bool
	// {{ end }} ==template==

	// rules table, maps the rule identifier to the rule node
	rules map[string]*rule
	rulesArray []*rule
	// variables stack, map of label to value
	vstack []map[string]any
	// rule stack, allows identification of the current rule in errors
	rstack []*rule

	// parse fail
	maxFailPos            position
	maxFailExpected       []string
	maxFailInvertExpected bool

	// max number of expressions to be parsed
	maxExprCnt uint64
	// entrypoint for the parser
	entrypoint string

	allowInvalidUTF8 bool

	*Stats

	choiceNoMatch string
	// recovery expression stack, keeps track of the currently available recovery expression, these are traversed in reverse
	recoveryStack []map[string]any

	_errPos *position
	// skip code stack
	scStack []bool
	// save point stack
	spStack parserStack
}

// newParser creates a parser with the specified input source and options.
func newParser(filename string, b []byte, opts ...option) *parser {
	stats := Stats{
		ChoiceAltCnt: make(map[string]map[string]int),
	}

	p := &parser{
		filename: filename,
		errs:     new(errList),
		data:     b,
		pt:       savepoint{position: position{line: 1}},
		recover:  true,
		cur: current{
			data: &ParserCustomData{},
		},
		maxFailPos:      position{col: 1, line: 1},
		maxFailExpected: make([]string, 0, 20),
		Stats:           &stats,
		// start rule is rule [0] unless an alternate entrypoint is specified
		entrypoint: "{{ .Entrypoint }}",
		scStack: []bool{false},
	}

	p.spStack.init(5)
	p.setOptions(opts)

	if p.maxExprCnt == 0 {
		p.maxExprCnt = math.MaxUint64
	}

	return p
}

// setOptions applies the options to the parser.
func (p *parser) setOptions(opts []option) {
	for _, opt := range opts {
		opt(p)
	}
}

// setCustomData to the parser.
func (p *parser) setCustomData(data *ParserCustomData) {
	p.cur.data = data;
}

func (p *parser) checkSkipCode() bool {
	return p.scStack[len(p.scStack)-1]
}

// push a variable set on the vstack.
func (p *parser) pushV() {
	if cap(p.vstack) == len(p.vstack) {
		// create new empty slot in the stack
		p.vstack = append(p.vstack, nil)
	} else {
		// slice to 1 more
		p.vstack = p.vstack[:len(p.vstack)+1]
	}

	// get the last args set
	m := p.vstack[len(p.vstack)-1]
	if m != nil && len(m) == 0 {
		// empty map, all good
		return
	}

	m = make(map[string]any)
	p.vstack[len(p.vstack)-1] = m
}

// pop a variable set from the vstack.
func (p *parser) popV() {
	// if the map is not empty, clear it
	m := p.vstack[len(p.vstack)-1]
	if len(m) > 0 {
		// GC that map
		p.vstack[len(p.vstack)-1] = nil
	}
	p.vstack = p.vstack[:len(p.vstack)-1]
}

// push a recovery expression with its labels to the recoveryStack
func (p *parser) pushRecovery(labels []string, expr any) {
	if cap(p.recoveryStack) == len(p.recoveryStack) {
		// create new empty slot in the stack
		p.recoveryStack = append(p.recoveryStack, nil)
	} else {
		// slice to 1 more
		p.recoveryStack = p.recoveryStack[:len(p.recoveryStack)+1]
	}

	m := make(map[string]any, len(labels))
	for _, fl := range labels {
		m[fl] = expr
	}
	p.recoveryStack[len(p.recoveryStack)-1] = m
}

// pop a recovery expression from the recoveryStack
func (p *parser) popRecovery() {
	// GC that map
	p.recoveryStack[len(p.recoveryStack)-1] = nil

	p.recoveryStack = p.recoveryStack[:len(p.recoveryStack)-1]
}

// ==template== {{ if not .Optimize }}
func (p *parser) print(prefix, s string) string {
	if !p.debug {
		return s
	}

	fmt.Printf("%s %d:%d:%d: %s [%#U]\n",
		prefix, p.pt.line, p.pt.col, p.pt.offset, s, p.pt.rn)
	return s
}

func (p *parser) printIndent(mark string, s string) string {
	return p.print(strings.Repeat(" ", p.depth)+mark, s)
}

func (p *parser) in(s string) string {
	res := p.printIndent(">", s)
	p.depth++
	return res
}

func (p *parser) out(s string) string {
	p.depth--
	return p.printIndent("<", s)
}

// {{ end }} ==template==

func (p *parser) addErr(err error) {
	if p._errPos != nil {
		p.addErrAt(err, *p._errPos, []string{})
	} else {
		p.addErrAt(err, p.pt.position, []string{})
	}
}

func (p *parser) addErrAt(err error, pos position, expected []string) {
	var buf bytes.Buffer
	if p.filename != "" {
		buf.WriteString(p.filename)
	}
	if buf.Len() > 0 {
		buf.WriteString(":")
	}
	buf.WriteString(fmt.Sprintf("%d:%d (%d)", pos.line, pos.col, pos.offset))
	if len(p.rstack) > 0 {
		if buf.Len() > 0 {
			buf.WriteString(": ")
		}
		rule := p.rstack[len(p.rstack)-1]
		if rule.displayName != "" {
			buf.WriteString("rule " + rule.displayName)
		} else {
			buf.WriteString("rule " + rule.name)
		}
	}
	pe := &parserError{Inner: err, pos: pos, prefix: buf.String(), expected: expected}
	p.errs.add(pe)
}

func (p *parser) failAt(fail bool, pos *position, want string) {
	// process fail if parsing fails and not inverted or parsing succeeds and invert is set
	if fail == p.maxFailInvertExpected {
		if pos.offset < p.maxFailPos.offset {
			return
		}

		if pos.offset > p.maxFailPos.offset {
			p.maxFailPos = *pos
			p.maxFailExpected = p.maxFailExpected[:0]
		}

		if p.maxFailInvertExpected {
			want = "!" + want
		}
		p.maxFailExpected = append(p.maxFailExpected, want)
	}
}

// read advances the parser to the next rune.
func (p *parser) read() {
	p.pt.offset += p.pt.w
	rn, n := utf8.DecodeRune(p.data[p.pt.offset:])
	p.pt.rn = rn
	p.pt.w = n
	p.pt.col++
	if rn == '\n' {
		p.pt.line++
		p.pt.col = 0
	}

	if rn == utf8.RuneError && n == 1 { // see utf8.DecodeRune
		if !p.allowInvalidUTF8 {
			p.addErr(errInvalidEncoding)
		}
	}
}

// restore parser position to the savepoint pt.
func (p *parser) restore(pt *savepoint) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("restore"))
	}
	// {{ end }} ==template==
	if pt.offset == p.pt.offset {
		return
	}
	p.pt = *pt
}

// get the slice of bytes from the savepoint start to the current position.
func (p *parser) sliceFrom(start *savepoint) []byte {
	return p.data[start.position.offset:p.pt.position.offset]
}

// get the slice of bytes from the savepoint start to the current position.
func (p *parser) sliceFromOffset(offset int) []byte {
	return p.data[offset:p.pt.position.offset]
}

// {{ if .GrammarMap }}

func (p *parser) parse(grammar map[string]*rule) (val any, err error) {
	if grammar == nil {
		grammar = {{ .GrammarVarName }}
	}
	p.rules = grammar

	if p.recover {
		// panic can be used in action code to stop parsing immediately
		// and return the panic as an error.
		defer func() {
			if e := recover(); e != nil {
				// ==template== {{ if not .Optimize }}
				if p.debug {
					defer p.out(p.in("panic handler"))
				}
				// {{ end }} ==template==
				val = nil
				switch e := e.(type) {
				case error:
					p.addErr(e)
				default:
					p.addErr(fmt.Errorf("%v", e))
				}
				err = p.errs.err()
			}
		}()
	}

	startRule, ok := p.rules[p.entrypoint]
	if !ok {
		p.addErr(errInvalidEntrypoint)
		return nil, p.errs.err()
	}

	p.read() // advance to first rune
	val, ok = p.parseRuleWrap(startRule)
	if !ok {
		if len(*p.errs) == 0 {
			// If parsing fails, but no errors have been recorded, the expected values
			// for the farthest parser position are returned as error.
			maxFailExpectedMap := make(map[string]struct{}, len(p.maxFailExpected))
			for _, v := range p.maxFailExpected {
				maxFailExpectedMap[v] = struct{}{}
			}
			expected := make([]string, 0, len(maxFailExpectedMap))
			eof := false
			if _, ok := maxFailExpectedMap["!."]; ok {
				delete(maxFailExpectedMap, "!.")
				eof = true
			}
			for k := range maxFailExpectedMap {
				expected = append(expected, k)
			}
			sort.Strings(expected)
			if eof {
				expected = append(expected, "EOF")
			}
			p.addErrAt(errors.New("no match found, expected: "+listJoin(expected, ", ", "or")), p.maxFailPos, expected)
		}

		return nil, p.errs.err()
	}
	return val, p.errs.err()
}

// {{ else }} ==template==

func (p *parser) buildRulesTable(g *grammar) {
	p.rules = make(map[string]*rule, len(g.rules))
	for _, r := range g.rules {
		p.rules[r.name] = r
	}
}

// {{ if .Nolint }} nolint: gocyclo {{else}} ==template== {{ end }}
func (p *parser) parse(grammar *grammar) (val any, err error) {
	if grammar == nil {
		grammar = {{ .GrammarVarName }}
	}
	if len(grammar.rules) == 0 {
		p.addErr(errNoRule)
		return nil, p.errs.err()
	}

	p.rulesArray = grammar.rules
	p.buildRulesTable(grammar)

	if p.recover {
		// panic can be used in action code to stop parsing immediately
		// and return the panic as an error.
		defer func() {
			if e := recover(); e != nil {
				// ==template== {{ if not .Optimize }}
				if p.debug {
					defer p.out(p.in("panic handler"))
				}
				// {{ end }} ==template==
				val = nil
				switch e := e.(type) {
				case error:
					p.addErr(e)
				default:
					p.addErr(fmt.Errorf("%v", e))
				}
				err = p.errs.err()
			}
		}()
	}

	startRule, ok := p.rules[p.entrypoint]
	if !ok {
		p.addErr(errInvalidEntrypoint)
		return nil, p.errs.err()
	}

	p.read() // advance to first rune
	val, ok = p.parseRuleWrap(startRule)
	if !ok {
		if len(*p.errs) == 0 {
			// If parsing fails, but no errors have been recorded, the expected values
			// for the farthest parser position are returned as error.
			maxFailExpectedMap := make(map[string]struct{}, len(p.maxFailExpected))
			for _, v := range p.maxFailExpected {
				maxFailExpectedMap[v] = struct{}{}
			}
			expected := make([]string, 0, len(maxFailExpectedMap))
			eof := false
			if _, ok := maxFailExpectedMap["!."]; ok {
				delete(maxFailExpectedMap, "!.")
				eof = true
			}
			for k := range maxFailExpectedMap {
				expected = append(expected, k)
			}
			sort.Strings(expected)
			if eof {
				expected = append(expected, "EOF")
			}
			p.addErrAt(errors.New("no match found, expected: "+listJoin(expected, ", ", "or")), p.maxFailPos, expected)
		}

		return nil, p.errs.err()
	}
	return val, p.errs.err()
}

// {{ end }} ==template==

func listJoin(list []string, sep string, lastSep string) string {
	switch len(list) {
	case 0:
		return ""
	case 1:
		return list[0]
	default:
		return strings.Join(list[:len(list)-1], sep) + " " + lastSep + " " + list[len(list)-1]
	}
}

// ==template== {{ if .NeedExprWrap }}
func (p *parser) parseRuleWrap(rule *rule) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseRule " + rule.name))
	}
	// {{ end }} ==template==
	var (
		val any
		ok  bool
		// ==template== {{ if not .Optimize }}
		startMark = &p.pt
		// {{ end }} ==template==
	)

	val, ok = p.parseRule(rule)

	// ==template== {{ if not .Optimize }}
	if ok && p.debug {
		p.printIndent("MATCH", string(p.sliceFrom(startMark)))
	}
	// {{ end }} ==template==
	return val, ok
}

func (p *parser) parseRule(rule *rule) (any, bool) {
	p.rstack = append(p.rstack, rule)
	p.pushV()
	val, ok := p.parseExprWrap(rule.expr)
	p.popV()
	p.rstack = p.rstack[:len(p.rstack)-1]
	return val, ok
}
// {{ else }}
func (p *parser) parseRuleWrap(rule *rule) (any, bool) {
	p.rstack = append(p.rstack, rule)
	var val any
	var ok bool
	if rule.varExists && !p.checkSkipCode() {
		p.pushV()
		val, ok = p.parseExprWrap(rule.expr)
		p.popV()
	} else {
		val, ok = p.parseExprWrap(rule.expr)
	}
	p.rstack = p.rstack[:len(p.rstack)-1]
	return val, ok
}
// {{ end }} ==template==

// ==template== {{ if .NeedExprWrap }}
func (p *parser) parseExprWrap(expr any) (any, bool) {
	val, ok := p.parseExpr(expr)
	return val, ok
}
// {{ end }} ==template==

// {{ if .Nolint }} nolint: gocyclo {{else}} ==template== {{ end }}
func (p *parser) {{ .ParseExprName }}(expr any) (any, bool) {
	p.ExprCnt++
	if p.ExprCnt > p.maxExprCnt {
		panic(errMaxExprCnt)
	}

	var val any
	var ok bool
	switch expr := expr.(type) {
	case *actionExpr:
		val, ok = p.parseActionExpr(expr)
	case *andCodeExpr:
		val, ok = p.parseAndCodeExpr(expr)
	case *andExpr:
		val, ok = p.parseAndExpr(expr)
	case *andLogicalExpr:
		val, ok = p.parseAndLogicalExpr(expr)
	case *anyMatcher:
		val, ok = p.parseAnyMatcher(expr)
	case *charClassMatcher:
		val, ok = p.parseCharClassMatcher(expr)
	case *choiceExpr:
		val, ok = p.parseChoiceExpr(expr)
	case *codeExpr:
		val, ok = p.parseCodeExpr(expr)
	case *labeledExpr:
		val, ok = p.parseLabeledExpr(expr)
	case *litMatcher:
		val, ok = p.parseLitMatcher(expr)
	case *notCodeExpr:
		val, ok = p.parseNotCodeExpr(expr)
	case *notExpr:
		val, ok = p.parseNotExpr(expr)
	case *notLogicalExpr:
		val, ok = p.parseNotLogicalExpr(expr)
	case *oneOrMoreExpr:
		val, ok = p.parseOneOrMoreExpr(expr)
	case *recoveryExpr:
		val, ok = p.parseRecoveryExpr(expr)
	case *ruleRefExpr:
		val, ok = p.parseRuleRefExpr(expr)
	// ==template== {{ if .IRefEnable }}
	case *ruleIRefExpr:
		val, ok = p.parseRuleIRefExpr(expr)
	// {{ end }} ==template==
	// {{ if .IRefCodeEnable }}
	case *ruleIRefExprX:
		val, ok = p.parseRuleIRefExprX(expr)
	// {{ end }} ==template==
	case *seqExpr:
		val, ok = p.parseSeqExpr(expr)
	case *throwExpr:
		val, ok = p.parseThrowExpr(expr)
	case *zeroOrMoreExpr:
		val, ok = p.parseZeroOrMoreExpr(expr)
	case *zeroOrOneExpr:
		val, ok = p.parseZeroOrOneExpr(expr)
	default:
		panic(fmt.Sprintf("unknown expression type %T", expr))
	}

	return val, ok
}

func (p *parser) parseActionExpr(act *actionExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseActionExpr"))
	}

	// {{ end }} ==template==
	if p.checkSkipCode() {
		_, ok := p.parseExprWrap(act.expr)
		return nil, ok
	}

	p.spStack.push(&p.pt)
	val, ok := p.parseExprWrap(act.expr)
	start := p.spStack.pop()

	if ok {
		p.cur.pos = start.position
		p.cur.text = p.sliceFrom(start)
		p._errPos = &start.position
		actVal := act.run(p)
		p._errPos = nil
		val = actVal
	}
	// ==template== {{ if not .Optimize }}
	if ok && p.debug {
		p.printIndent("MATCH", string(p.sliceFrom(&p.spStack.data[p.spStack.index+1])))
	}
	// {{ end }} ==template==
	return val, ok
}

func (p *parser) parseAndCodeExpr(and *andCodeExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseAndCodeExpr"))
	}

	// {{ end }} ==template==
	ok := and.run(p)
	return nil, ok
}

func (p *parser) parseAndExpr(and *andExpr) (any, bool) {
	return p.parseAndExprBase(and, false)
}

func (p *parser) parseAndLogicalExpr(and *andLogicalExpr) (any, bool) {
	return p.parseAndExprBase((*andExpr)(and), true)
}

func (p *parser) parseAndExprBase(and *andExpr, logical bool) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseAndExpr"))
	}

	// {{ end }} ==template==
	pt := p.pt

	p.scStack = append(p.scStack, true)
	_, ok := p.parseExprWrap(and.expr)
	p.scStack = p.scStack[:len(p.scStack)-1]

	matchedOffset := p.pt.offset
	p.restore(&pt)

	if logical {
		return nil, ok && p.pt.offset != matchedOffset
	}
	return nil, ok
}

func (p *parser) parseAnyMatcher(any *anyMatcher) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseAnyMatcher"))
	}

	// {{ end }} ==template==
	if p.pt.rn == utf8.RuneError && p.pt.w == 0 {
		// EOF - see utf8.DecodeRune
		p.failAt(false, &p.pt.position, ".")
		return nil, false
	}
	p.failAt(true, &p.pt.position, ".")
	p.read()
	return nil, true
}

// {{ if .Nolint }} nolint: gocyclo {{else}} ==template== {{ end }}
func (p *parser) parseCharClassMatcher(chr *charClassMatcher) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseCharClassMatcher"))
	}

	// {{ end }} ==template==
	cur := p.pt.rn

	// can't match EOF
	if cur == utf8.RuneError && p.pt.w == 0 { // see utf8.DecodeRune
		p.failAt(false, &p.pt.position, chr.val)
		return nil, false
	}

	if chr.ignoreCase {
		cur = unicode.ToLower(cur)
	}

	// try to match in the list of available chars
	for _, rn := range chr.chars {
		if rn == cur {
			if chr.inverted {
				p.failAt(false, &p.pt.position, chr.val)
				return nil, false
			}
			p.failAt(true, &p.pt.position, chr.val)
			p.read()
			return nil, true
		}
	}

	// try to match in the list of ranges
	for i := 0; i < len(chr.ranges); i += 2 {
		if cur >= chr.ranges[i] && cur <= chr.ranges[i+1] {
			if chr.inverted {
				p.failAt(false, &p.pt.position, chr.val)
				return nil, false
			}
			p.failAt(true, &p.pt.position, chr.val)
			p.read()
			return nil, true
		}
	}

	// try to match in the list of Unicode classes
	for _, cl := range chr.classes {
		if unicode.Is(cl, cur) {
			if chr.inverted {
				p.failAt(false, &p.pt.position, chr.val)
				return nil, false
			}
			p.failAt(true, &p.pt.position, chr.val)
			p.read()
			return nil, true
		}
	}

	if chr.inverted {
		p.failAt(true, &p.pt.position, chr.val)
		p.read()
		return nil, true
	}
	p.failAt(false, &p.pt.position, chr.val)
	return nil, false
}

// ==template== {{ if not .Optimize }}

func (p *parser) incChoiceAltCnt(ch *choiceExpr, altI int) {
	// choiceIdent := fmt.Sprintf("%s %d:%d", p.rstack[len(p.rstack)-1].name, ch.pos.line, ch.pos.col)
	choiceIdent := fmt.Sprintf("%s", p.rstack[len(p.rstack)-1].name)
	m := p.ChoiceAltCnt[choiceIdent]
	if m == nil {
		m = make(map[string]int)
		p.ChoiceAltCnt[choiceIdent] = m
	}
	// We increment altI by 1, so the keys do not start at 0
	alt := strconv.Itoa(altI + 1)
	if altI == choiceNoMatch {
		alt = p.choiceNoMatch
	}
	m[alt]++
}

// {{ end }} ==template==

func (p *parser) parseChoiceExpr(ch *choiceExpr) (any, bool) {
	for altI, alt := range ch.alternatives {
		// dummy assignment to prevent compile error if optimized
		_ = altI

		val, ok := p.parseExprWrap(alt)
		if ok {
			// ==template== {{ if not .Optimize }}
			p.incChoiceAltCnt(ch, altI)
			// {{ end }} ==template==
			return val, ok
		}
	}
	// ==template== {{ if not .Optimize }}
	p.incChoiceAltCnt(ch, choiceNoMatch)
	// {{ end }} ==template==
	return nil, false
}

func (p *parser) parseLabeledExpr(lab *labeledExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseLabeledExpr"))
	}

	// {{ end }} ==template==
	startOffset := p.pt.position.offset
	var val any
	var ok bool
	val, ok = p.parseExprWrap(lab.expr)
	if ok && lab.label != "" && !p.checkSkipCode() {
		m := p.vstack[len(p.vstack)-1]
		if lab.textCapture {
			m[lab.label] = string(p.sliceFromOffset(startOffset))
		} else {
			m[lab.label] = val
		}
	}
	return val, ok
}

func (p *parser) parseCodeExpr(code *codeExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseCodeExpr"))
	}

	// {{ end }} ==template==
	if !code.notSkip && p.checkSkipCode() {
		return nil, true
	}
	return code.run(p), true
}

func (p *parser) parseLitMatcher(lit *litMatcher) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseLitMatcher"))
	}

	// {{ end }} ==template==
	start := p.pt
	for _, want := range lit.val {
		cur := p.pt.rn
		if lit.ignoreCase {
			cur = unicode.ToLower(cur)
		}
		if cur != want {
			p.failAt(false, &start.position, lit.want)
			p.restore(&start)
			return nil, false
		}
		p.read()
	}
	p.failAt(true, &start.position, lit.want)
	return nil, true
}

func (p *parser) parseNotCodeExpr(not *notCodeExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseNotCodeExpr"))
	}

	// {{ end }} ==template==
	ok := not.run(p)
	return nil, !ok
}

func (p *parser) parseNotExpr(not *notExpr) (any, bool) {
	return p.parseNotExprBase(not, false)
}

func (p *parser) parseNotLogicalExpr(not *notLogicalExpr) (any, bool) {
	return p.parseNotExprBase((*notExpr)(not), true)
}

func (p *parser) parseNotExprBase(not *notExpr, logical bool) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseNotExpr"))
	}

	// {{ end }} ==template==
	pt := p.pt
	p.maxFailInvertExpected = !p.maxFailInvertExpected

	p.scStack = append(p.scStack, true)
	_, ok := p.parseExprWrap(not.expr)
	p.scStack = p.scStack[:len(p.scStack)-1]

	p.maxFailInvertExpected = !p.maxFailInvertExpected
	matchedOffset := p.pt.offset
	p.restore(&pt)

	if logical {
		return nil, !ok && p.pt.offset != matchedOffset
	}
	return nil, !ok
}

func (p *parser) parseOneOrMoreExpr(expr *oneOrMoreExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseOneOrMoreExpr"))
	}

	// {{ end }} ==template==
	var vals []any
	var matched bool
	for {
		val, ok := p.parseExprWrap(expr.expr)
		if !ok {
			if len(vals) > 0 {
				return vals, matched
			}
			return nil, matched
		}
		matched = true
		if val != nil {
			vals = append(vals, val)
		}
	}
}

func (p *parser) parseRecoveryExpr(recover *recoveryExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseRecoveryExpr (" + strings.Join(recover.failureLabel, ",") + ")"))
	}

	// {{ end }} ==template==
	p.pushRecovery(recover.failureLabel, recover.recoverExpr)
	val, ok := p.parseExprWrap(recover.expr)
	p.popRecovery()

	return val, ok
}

func (p *parser) parseRuleRefExpr(ref *ruleRefExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseRuleRefExpr " + ref.name))
	}

	// {{ end }} ==template==
	rule := p.rules[ref.name]
	if rule == nil {
		p.addErr(fmt.Errorf("undefined rule: %s", ref.name))
		return nil, false
	}
	return p.parseRuleWrap(rule)
}

// {{ if .IRefEnable }}
func (p *parser) parseRuleIRefExpr(ref *ruleIRefExpr) (any, bool) {
	return p.parseRuleWrap(p.rulesArray[ref.index])
}
// {{ end }} ==template==

// {{ if .IRefCodeEnable }}
func (p *parser) parseRuleIRefExprX(ref *ruleIRefExprX) (any, bool) {
	return ref.call(p, p.rulesArray[ref.index])
}
// {{ end }} ==template==

func (p *parser) parseSeqExpr(seq *seqExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseSeqExpr"))
	}

	// {{ end }} ==template==
	var vals []any

	pt := p.pt
	for _, expr := range seq.exprs {
		val, ok := p.parseExprWrap(expr)
		if !ok {
			p.restore(&pt)
			return nil, false
		}

		if val == nil {
			continue
		}

		vals = append(vals, val)
	}
	if len(vals) > 0 {
		return vals, true
	}
	return nil, true
}

func (p *parser) parseThrowExpr(expr *throwExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseThrowExpr"))
	}

	// {{ end }} ==template==
	for i := len(p.recoveryStack) - 1; i >= 0; i-- {
		if recoverExpr, ok := p.recoveryStack[i][expr.label]; ok {
			if val, ok := p.parseExprWrap(recoverExpr); ok {
				return val, ok
			}
		}
	}
	return nil, false
}

func (p *parser) parseZeroOrMoreExpr(expr *zeroOrMoreExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseZeroOrMoreExpr"))
	}

	// {{ end }} ==template==
	var vals []any
	for {
		val, ok := p.parseExprWrap(expr.expr)
		if !ok {
			if len(vals) > 0 {
				return vals, true
			}
			return nil, true
		}
		if val != nil {
			vals = append(vals, val)
		}
	}
}

func (p *parser) parseZeroOrOneExpr(expr *zeroOrOneExpr) (any, bool) {
	// ==template== {{ if not .Optimize }}
	if p.debug {
		defer p.out(p.in("parseZeroOrOneExpr"))
	}

	// {{ end }} ==template==
	val, _ := p.parseExprWrap(expr.expr)
	// whether it matched or not, consider it a match
	return val, true
}
