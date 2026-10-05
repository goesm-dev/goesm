package lower

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Promote a 64-bit field to a local for a run of assignments.
//
// int64 and uint64 are BigInts, and storing one into an object field
// allocates it: V8 computes BigInt.asUintN(64, …) arithmetic on machine
// words only while the value stays in a local. A run of statements that
// only assign one int64 or uint64 field of a variable, from that field,
// locals and constants, as xorshift's
//
//	x.s ^= x.s << 13
//	x.s ^= x.s >> 7
//	x.s ^= x.s << 17
//
// therefore works on a local copy of the field, stored back once after the
// run. Nothing can observe the field meanwhile: the statements make no
// calls, read no other memory and cannot panic (no division, only constant
// shift counts), so only the first load can fail, as the first statement
// would.

type promoKey struct {
	v     *types.Var
	field int
}

// fieldRun returns the length of the run of assignments to one 64-bit
// field that starts at list[i], with the field's key and a selector of it,
// or 0 when there is no run of two or more.
func (fe *funcEmitter) fieldRun(list []ast.Stmt, i int) (int, promoKey, *ast.SelectorExpr) {
	var key promoKey
	var first *ast.SelectorExpr
	n := 0
run:
	for _, s := range list[i:] {
		a, ok := s.(*ast.AssignStmt)
		if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
			break
		}
		switch a.Tok {
		case token.DEFINE, token.QUO_ASSIGN, token.REM_ASSIGN:
			break run
		case token.SHL_ASSIGN, token.SHR_ASSIGN:
			if fe.info.Types[a.Rhs[0]].Value == nil {
				break run
			}
		}
		sel, k, ok := fe.promotable(a.Lhs[0])
		if !ok || (n > 0 && k != key) || !fe.pureArith(a.Rhs[0], k) {
			break
		}
		if n == 0 {
			key, first = k, sel
		}
		n++
	}
	if n < 2 {
		return 0, key, nil
	}
	return n, key, first
}

// promotable reports whether e selects an int64 or uint64 field of a local
// variable (through a pointer or not), and returns its key.
func (fe *funcEmitter) promotable(e ast.Expr) (*ast.SelectorExpr, promoKey, bool) {
	sel, ok := unparen(e).(*ast.SelectorExpr)
	if !ok {
		return nil, promoKey{}, false
	}
	s, ok := fe.info.Selections[sel]
	if !ok || s.Kind() != types.FieldVal || len(s.Index()) != 1 || !isBig(s.Type()) {
		return nil, promoKey{}, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, promoKey{}, false
	}
	v, ok := fe.info.Uses[id].(*types.Var)
	if !ok || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() || fe.fieldLocals[v] != nil {
		return nil, promoKey{}, false
	}
	return sel, promoKey{v, s.Index()[0]}, true
}

// pureArith reports whether e is integer arithmetic on the field key,
// variables and constants that can neither panic nor read other memory.
func (fe *funcEmitter) pureArith(e ast.Expr, key promoKey) bool {
	if fe.info.Types[e].Value != nil {
		return true
	}
	switch e := e.(type) {
	case *ast.ParenExpr:
		return fe.pureArith(e.X, key)
	case *ast.Ident:
		_, ok := fe.info.Uses[e].(*types.Var)
		return ok
	case *ast.SelectorExpr:
		_, k, ok := fe.promotable(e)
		return ok && k == key
	case *ast.UnaryExpr:
		switch e.Op {
		case token.ADD, token.SUB, token.XOR:
			return fe.pureArith(e.X, key)
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.ADD, token.SUB, token.MUL, token.AND, token.OR, token.XOR, token.AND_NOT:
			return fe.pureArith(e.X, key) && fe.pureArith(e.Y, key)
		case token.SHL, token.SHR:
			return fe.pureArith(e.X, key) && fe.info.Types[e.Y].Value != nil
		}
	case *ast.CallExpr:
		// A conversion between integer types.
		if len(e.Args) == 1 && fe.info.Types[e.Fun].IsType() {
			b, ok := under(fe.info.TypeOf(e)).(*types.Basic)
			return ok && b.Info()&types.IsInteger != 0 && fe.pureArith(e.Args[0], key)
		}
	}
	return false
}

// promotedRun lowers the n assignments to the field of sel starting at
// list[i] on a local copy of it.
func (fe *funcEmitter) promotedRun(list []ast.Stmt, i, n int, key promoKey, sel *ast.SelectorExpr) {
	name := fe.declareName(jsName(key.v.Name()) + "$" + jsName(sel.Sel.Name))
	fe.w.ln("%slet %s = %s;", fe.mark(list[i]), name, fe.selector(sel))
	if fe.promoted == nil {
		fe.promoted = map[promoKey]string{}
	}
	fe.promoted[key] = name
	for _, s := range list[i : i+n] {
		fe.stmt(s, "")
	}
	delete(fe.promoted, key)
	fe.w.ln("%s;", fe.lvalue(sel, false).set(name))
}

// promotedField returns the local holding the field e selects, if any.
func (fe *funcEmitter) promotedField(e *ast.SelectorExpr) (string, bool) {
	if len(fe.promoted) == 0 {
		return "", false
	}
	_, k, ok := fe.promotable(e)
	if !ok {
		return "", false
	}
	name, ok := fe.promoted[k]
	return name, ok
}
