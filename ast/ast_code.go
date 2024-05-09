package ast

import "fmt"

// ActionExpr is an expression that has an associated block of code to
// execute when the expression matches.
type ActionExpr struct {
	p      Pos
	Expr   Expression
	Code   *CodeBlock
	FuncIx int

	Nullable bool
}

var _ Expression = (*ActionExpr)(nil)

// NewActionExpr creates a new action expression at the specified position.
func NewActionExpr(p Pos) *ActionExpr {
	return &ActionExpr{p: p}
}

// Pos returns the starting position of the node.
func (a *ActionExpr) Pos() Pos { return a.p }

// String returns the textual representation of a node.
func (a *ActionExpr) String() string {
	return fmt.Sprintf("%s: %T{Expr: %v, Code: %v}", a.p, a, a.Expr, a.Code)
}

// NullableVisit recursively determines whether an object is nullable.
func (a *ActionExpr) NullableVisit(rules map[string]*Rule) bool {
	a.Nullable = a.Expr.NullableVisit(rules)
	return a.Nullable
}

// IsNullable returns the nullable attribute of the node.
func (a *ActionExpr) IsNullable() bool {
	return a.Nullable
}

// InitialNames returns names of nodes with which an expression can begin.
func (a *ActionExpr) InitialNames() map[string]struct{} {
	names := make(map[string]struct{})
	for name := range a.Expr.InitialNames() {
		names[name] = struct{}{}
	}
	return names
}

// CodeExpr supports custom parser.
type CodeExpr struct {
	p      Pos
	Code   *CodeBlock
	FuncIx int
}

var _ Expression = (*CodeExpr)(nil)

// NewCodeExpr creates a new code expression at the specified position.
func NewCodeExpr(p Pos) *CodeExpr {
	return &CodeExpr{p: p}
}

// Pos returns the starting position of the node.
func (s *CodeExpr) Pos() Pos { return s.p }

// String returns the textual representation of a node.
func (s *CodeExpr) String() string {
	return fmt.Sprintf("%s: %T{Code: %v}", s.p, s, s.Code)
}

// NullableVisit recursively determines whether an object is nullable.
func (s *CodeExpr) NullableVisit(rules map[string]*Rule) bool {
	return true
}

// IsNullable returns the nullable attribute of the node.
func (s *CodeExpr) IsNullable() bool {
	return true
}

// InitialNames returns names of nodes with which an expression can begin.
func (s *CodeExpr) InitialNames() map[string]struct{} {
	return make(map[string]struct{})
}
