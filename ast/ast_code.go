package ast

import "fmt"

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
