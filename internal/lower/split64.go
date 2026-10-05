package lower

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math/big"
	"os"
	"strconv"
	"strings"
)

// Split 64-bit locals.
//
// int64 and uint64 are BigInts, and BigInt arithmetic is slow: a hash or
// random number loop over uint64 runs 4 times slower than Go under V8 and
// 50 times slower under JavaScriptCore. A local int64 or uint64 variable
// whose arithmetic happens in a loop is therefore held as two JS numbers,
// the high and low 32 bits as int32 values (name$hi, name$lo), and its
// arithmetic is done on the halves with 32-bit operations. The variable
// becomes a BigInt again (pairU64 / pairI64) only where a BigInt is needed:
// a call argument, a return value, a store into memory.
//
// A variable is split when
//   - it is declared in the function body by `x := e` or `var x T [= e]`
//     with a single name, as a statement of a block or case clause;
//   - its address is not taken and the function has no goto (whose state
//     machine hoists variables);
//   - every assignment to it assigns it alone (`x = e`, `x op= e`, `x++`);
//   - some assignment computes it with pairable arithmetic (see pairable)
//     inside a loop, and every other read or assignment of it is in a
//     shallower loop, so that the conversions to and from BigInt happen
//     less often than the arithmetic they speed up.
//
// Closures may use a split variable: they capture name$hi and name$lo as
// they would capture the variable.

// GOESM_SPLIT64=off disables the splitting and GOESM_SPLIT64=all splits
// every candidate regardless of what it pays, which tests use to exercise
// the lowering on all code.
var (
	split64Off = os.Getenv("GOESM_SPLIT64") == "off"
	split64All = os.Getenv("GOESM_SPLIT64") == "all"
)

// split64Vars returns the local variables of body to split.
func split64Vars(info *types.Info, body *ast.BlockStmt, boxed map[*types.Var]bool) map[*types.Var]bool {
	if body == nil || containsGoto(body) || split64Off {
		return nil
	}
	a := &splitAnalysis{info: info, cands: map[*types.Var]bool{}, disq: map[*types.Var]bool{}}
	a.collect(body, boxed)
	if len(a.cands) == 0 {
		return nil
	}
	for !split64All {
		changed := false
		for v := range a.cands {
			if !a.worth(v) {
				delete(a.cands, v)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if len(a.cands) == 0 {
		return nil
	}
	return a.cands
}

type splitAnalysis struct {
	info  *types.Info
	cands map[*types.Var]bool
	disq  map[*types.Var]bool
	// For each candidate, its assignments and reads with their loop depth.
	assigns map[*types.Var][]splitSite
	reads   map[*types.Var][]splitSite
	parents map[ast.Node]ast.Node
}

type splitSite struct {
	depth int
	// For an assignment: the right-hand side as a pairable expression
	// (nil for a zero value), op for op-assignments and ++/--.
	rhs ast.Expr
	op  token.Token
	inc bool
	// For a read: the identifier and the expression it is part of that
	// consumes it (see consumer).
	id *ast.Ident
}

// isSplitType reports whether t is int64 or uint64, possibly named.
func isSplitType(t types.Type) bool {
	return t != nil && !isTypeParam(t) && isBig(t)
}

func (a *splitAnalysis) varOf(e ast.Expr) *types.Var {
	id, ok := unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	if v, ok := a.info.Uses[id].(*types.Var); ok {
		return v
	}
	v, _ := a.info.Defs[id].(*types.Var)
	return v
}

// collect finds the candidates, their assignments and reads.
func (a *splitAnalysis) collect(body *ast.BlockStmt, boxed map[*types.Var]bool) {
	a.assigns = map[*types.Var][]splitSite{}
	a.reads = map[*types.Var][]splitSite{}
	targets := map[*ast.Ident]bool{} // identifiers that are assignment targets
	// Declarations that may introduce candidates: statements of a block
	// or case clause.
	stmtLists := func(list []ast.Stmt, depth int) {
		for _, s := range list {
			switch s := s.(type) {
			case *ast.AssignStmt:
				if s.Tok == token.DEFINE && len(s.Lhs) == 1 && len(s.Rhs) == 1 {
					if id, ok := s.Lhs[0].(*ast.Ident); ok {
						if v, ok := a.info.Defs[id].(*types.Var); ok && isSplitType(v.Type()) && !boxed[v] {
							a.cands[v] = true
						}
					}
				}
			case *ast.DeclStmt:
				gd, ok := s.Decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					if len(vs.Names) != 1 || len(vs.Values) > 1 {
						continue
					}
					if v, ok := a.info.Defs[vs.Names[0]].(*types.Var); ok && isSplitType(v.Type()) && !boxed[v] {
						a.cands[v] = true
					}
				}
			}
		}
	}
	var walk func(n ast.Node, depth int)
	walk = func(n ast.Node, depth int) {
		ast.Inspect(n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.BlockStmt:
				stmtLists(n.List, depth)
			case *ast.CaseClause:
				stmtLists(n.Body, depth)
			case *ast.CommClause:
				stmtLists(n.Body, depth)
				if as, ok := n.Comm.(*ast.AssignStmt); ok {
					for _, l := range as.Lhs {
						if v := a.varOf(l); v != nil {
							a.disq[v] = true
						}
					}
				}
			case *ast.ForStmt:
				// The init statement runs once, outside the loop.
				if n.Init != nil {
					walk(n.Init, depth)
				}
				for _, c := range []ast.Node{n.Cond, n.Post, n.Body} {
					if c != nil && !isNilNode(c) {
						walk(c, depth+1)
					}
				}
				return false
			case *ast.RangeStmt:
				for _, x := range []ast.Expr{n.Key, n.Value} {
					if x == nil {
						continue
					}
					if v := a.varOf(x); v != nil {
						a.disq[v] = true
					}
				}
				walk(n.X, depth)
				walk(n.Body, depth+1)
				return false
			case *ast.AssignStmt:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
					if v := a.varOf(n.Lhs[0]); v != nil {
						targets[unparen(n.Lhs[0]).(*ast.Ident)] = true
						site := splitSite{depth: depth, rhs: n.Rhs[0]}
						if n.Tok != token.ASSIGN && n.Tok != token.DEFINE {
							site.op = opAssign[n.Tok]
						}
						a.assigns[v] = append(a.assigns[v], site)
					}
				} else {
					for _, l := range n.Lhs {
						if v := a.varOf(l); v != nil {
							targets[unparen(l).(*ast.Ident)] = true
							a.disq[v] = true
						}
					}
				}
			case *ast.IncDecStmt:
				if v := a.varOf(n.X); v != nil {
					targets[unparen(n.X).(*ast.Ident)] = true
					op := token.ADD
					if n.Tok == token.DEC {
						op = token.SUB
					}
					a.assigns[v] = append(a.assigns[v], splitSite{depth: depth, op: op, inc: true})
				}
			case *ast.ValueSpec:
				for i, id := range n.Names {
					v, _ := a.info.Defs[id].(*types.Var)
					if v == nil {
						continue
					}
					targets[id] = true
					if len(n.Names) != 1 {
						a.disq[v] = true
						continue
					}
					site := splitSite{depth: depth}
					if i < len(n.Values) {
						site.rhs = n.Values[i]
					}
					a.assigns[v] = append(a.assigns[v], site)
				}
			case *ast.Ident:
				if targets[n] {
					return true
				}
				if v, ok := a.info.Uses[n].(*types.Var); ok {
					a.reads[v] = append(a.reads[v], splitSite{depth: depth, id: n})
				}
			}
			return true
		})
	}
	walk(body, 0)
	for v := range a.disq {
		delete(a.cands, v)
	}
	a.parents = parentMap(body)
}

func isNilNode(n ast.Node) bool {
	switch n := n.(type) {
	case ast.Expr:
		return n == nil
	case ast.Stmt:
		return n == nil
	}
	return false
}

// parentMap maps each node of root to its parent.
func parentMap(root ast.Node) map[ast.Node]ast.Node {
	m := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			m[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return m
}

// worth reports whether splitting v pays: v is computed with pairable
// arithmetic in a loop, and at that loop depth and deeper, the operators
// saved outnumber twice the conversions between BigInt and halves (each
// costs a few BigInt operations) that splitting adds.
func (a *splitAnalysis) worth(v *types.Var) bool {
	best := -1 // deepest loop of a pairable arithmetic assignment
	for _, s := range a.assigns[v] {
		if a.pairableSite(s) && (s.inc || s.op != token.ILLEGAL || a.arith(s.rhs)) && s.depth > best {
			best = s.depth
		}
	}
	if best < 1 {
		return false
	}
	ops, convs := 0, 0
	for _, s := range a.assigns[v] {
		if s.depth < best {
			continue
		}
		if !a.pairableSite(s) {
			convs++
			continue
		}
		if s.inc || s.op != token.ILLEGAL {
			ops++
		}
		if s.rhs != nil {
			ops += opCount(s.rhs)
		}
	}
	consumers := map[ast.Node]bool{}
	for _, s := range a.reads[v] {
		if s.depth < best || a.cheapRead(s.id) {
			continue
		}
		consumers[a.consumer(s.id)] = true
	}
	convs += len(consumers)
	return ops >= 2*convs
}

func (a *splitAnalysis) pairableSite(s splitSite) bool {
	return s.inc || s.rhs == nil || a.pairableAssign(s)
}

// opCount counts the operators of e.
func opCount(e ast.Expr) int {
	n := 0
	ast.Inspect(e, func(x ast.Node) bool {
		switch x.(type) {
		case *ast.BinaryExpr, *ast.UnaryExpr:
			n++
		}
		return true
	})
	return n
}

// consumer returns the outermost pairable expression containing id, which
// the lowering converts to a BigInt once.
func (a *splitAnalysis) consumer(id *ast.Ident) ast.Node {
	var n ast.Node = id
	for {
		p, ok := a.parents[n].(ast.Expr)
		if !ok || !a.pairable(p) {
			return n
		}
		n = p
	}
}

func (a *splitAnalysis) pairableAssign(s splitSite) bool {
	if s.op == token.SHL || s.op == token.SHR {
		return a.constCount(s.rhs)
	}
	return a.pairable(s.rhs)
}

func (a *splitAnalysis) constCount(e ast.Expr) bool {
	tv, ok := a.info.Types[e]
	return ok && tv.Value != nil
}

// pairable reports whether e, of type int64 or uint64, can be computed on
// halves: constants, split variables, conversions from other integers, and
// + - * & | ^ &^, unary - and ^, and shifts by constants of such operands.
func (a *splitAnalysis) pairable(e ast.Expr) bool {
	return pairableExpr(a.info, e, a.cands)
}

func pairableExpr(info *types.Info, e ast.Expr, split map[*types.Var]bool) bool {
	tv, ok := info.Types[e]
	if !ok || !isSplitType(tv.Type) {
		return false
	}
	if tv.Value != nil {
		return true
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return pairableExpr(info, x.X, split)
	case *ast.Ident:
		v, _ := info.Uses[x].(*types.Var)
		return v != nil && split[v]
	case *ast.CallExpr:
		ftv, ok := info.Types[x.Fun]
		if !ok || !ftv.IsType() || len(x.Args) != 1 {
			return false
		}
		at := info.TypeOf(x.Args[0])
		if isSplitType(at) {
			return pairableExpr(info, x.Args[0], split)
		}
		b, ok := under(at).(*types.Basic)
		return ok && b.Info()&types.IsInteger != 0 && !isTypeParam(at)
	case *ast.UnaryExpr:
		switch x.Op {
		case token.SUB, token.XOR, token.ADD:
			return pairableExpr(info, x.X, split)
		}
	case *ast.BinaryExpr:
		switch x.Op {
		case token.ADD, token.SUB, token.MUL, token.AND, token.OR, token.XOR, token.AND_NOT:
			return pairableExpr(info, x.X, split) && pairableExpr(info, x.Y, split)
		case token.SHL, token.SHR:
			ytv, ok := info.Types[x.Y]
			return ok && ytv.Value != nil && pairableExpr(info, x.X, split)
		}
	}
	return false
}

// arith reports whether e computes something (an operator or a
// conversion), not just copies a variable or a constant.
func (a *splitAnalysis) arith(e ast.Expr) bool {
	switch x := unparen(e).(type) {
	case *ast.UnaryExpr, *ast.BinaryExpr:
		return true
	case *ast.CallExpr:
		return len(x.Args) == 1 && a.arith(x.Args[0])
	}
	return false
}

// cheapRead reports whether the read id of a candidate is consumed by
// pairable code: it is part of a pairable expression that is assigned to
// a candidate, or converted to a non-64-bit integer or a float64.
func (a *splitAnalysis) cheapRead(id *ast.Ident) bool {
	var n ast.Node = id
	for {
		p := a.parents[n]
		switch p := p.(type) {
		case *ast.ParenExpr, *ast.UnaryExpr, *ast.BinaryExpr:
			e := p.(ast.Expr)
			if a.pairable(e) {
				n = p
				continue
			}
			return false
		case *ast.CallExpr:
			if ftv, ok := a.info.Types[p.Fun]; ok && ftv.IsType() && len(p.Args) == 1 {
				if isSplitType(ftv.Type) {
					if a.pairable(p) {
						n = p
						continue
					}
					return false
				}
				return numberConversion(ftv.Type) && a.pairable(p.Args[0])
			}
			return false
		case *ast.AssignStmt:
			if len(p.Lhs) == 1 && len(p.Rhs) == 1 && p.Rhs[0] == n && p.Tok != token.SHL_ASSIGN && p.Tok != token.SHR_ASSIGN {
				if v := a.varOf(p.Lhs[0]); v != nil && a.cands[v] {
					return a.pairable(n.(ast.Expr))
				}
			}
			return false
		case *ast.ValueSpec:
			if len(p.Names) == 1 && len(p.Values) == 1 && p.Values[0] == n {
				if v, ok := a.info.Defs[p.Names[0]].(*types.Var); ok && a.cands[v] {
					return a.pairable(n.(ast.Expr))
				}
			}
			return false
		}
		return false
	}
}

// numberConversion reports whether a conversion to t from a 64-bit
// integer yields a JS number that the halves give directly.
func numberConversion(t types.Type) bool {
	if isTypeParam(t) {
		return false
	}
	b, ok := under(t).(*types.Basic)
	if !ok {
		return false
	}
	if b.Info()&types.IsInteger != 0 {
		return !isBigKind(b)
	}
	return b.Kind() == types.Float64
}

// ---- lowering ----

func (fe *funcEmitter) isSplit(v *types.Var) bool {
	return v != nil && fe.pe.split[v]
}

func (fe *funcEmitter) splitNames(v *types.Var) (string, string) {
	n := fe.nameOf(v)
	return n + "$hi", n + "$lo"
}

// splitRead returns a split variable as a BigInt.
func (fe *funcEmitter) splitRead(v *types.Var) string {
	hi, lo := fe.splitNames(v)
	return fe.materialize(hi, lo, v.Type())
}

func (fe *funcEmitter) materialize(hi, lo string, t types.Type) string {
	if ii, _ := intKind(t); ii.signed {
		return "$rt.pairI64(" + hi + ", " + lo + ")"
	}
	return "$rt.pairU64(" + hi + ", " + lo + ")"
}

// splitStmt lowers a statement assigning a split variable, reporting
// whether s was one.
func (fe *funcEmitter) splitStmt(s ast.Stmt, m string) bool {
	if fe.pe.split == nil {
		return false
	}
	switch s := s.(type) {
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return false
		}
		id, ok := unparen(s.Lhs[0]).(*ast.Ident)
		if !ok {
			return false
		}
		var v *types.Var
		if s.Tok == token.DEFINE {
			v, _ = fe.info.Defs[id].(*types.Var)
		} else {
			v, _ = fe.info.Uses[id].(*types.Var)
		}
		if !fe.isSplit(v) {
			return false
		}
		g := &pairGen{fe: fe}
		var hi, lo string
		switch s.Tok {
		case token.DEFINE, token.ASSIGN:
			hi, lo = g.value(s.Rhs[0], v.Type())
		default:
			op := opAssign[s.Tok]
			vh, vl := fe.splitNames(v)
			if op == token.SHL || op == token.SHR {
				if tv := fe.info.Types[s.Rhs[0]]; tv.Value != nil {
					hi, lo = g.shift(op, vh, vl, shiftConst(tv.Value), v.Type())
					break
				}
				val := fe.shift(op, fe.splitRead(v), fe.shiftCount(s.Rhs[0]), v.Type())
				hi, lo = g.fromBig(val)
				break
			}
			bh, bl := g.value(s.Rhs[0], v.Type())
			hi, lo = g.binary(op, vh, vl, bh, bl, fe.info.Types[s.Rhs[0]].Value)
		}
		fe.splitSet(m, v, hi, lo, g, s.Tok == token.DEFINE)
		return true
	case *ast.IncDecStmt:
		id, ok := unparen(s.X).(*ast.Ident)
		if !ok {
			return false
		}
		v, _ := fe.info.Uses[id].(*types.Var)
		if !fe.isSplit(v) {
			return false
		}
		g := &pairGen{fe: fe}
		op := token.ADD
		if s.Tok == token.DEC {
			op = token.SUB
		}
		vh, vl := fe.splitNames(v)
		hi, lo := g.binary(op, vh, vl, "0", "1", constant.MakeInt64(1))
		fe.splitSet(m, v, hi, lo, g, false)
		return true
	case *ast.DeclStmt:
		gd, ok := s.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR || len(gd.Specs) != 1 {
			return false
		}
		vs := gd.Specs[0].(*ast.ValueSpec)
		if len(vs.Names) != 1 {
			return false
		}
		v, _ := fe.info.Defs[vs.Names[0]].(*types.Var)
		if !fe.isSplit(v) {
			return false
		}
		g := &pairGen{fe: fe}
		hi, lo := "0", "0"
		if len(vs.Values) == 1 {
			hi, lo = g.value(vs.Values[0], v.Type())
		}
		fe.splitSet(fe.mark(vs), v, hi, lo, g, true)
		return true
	}
	return false
}

// splitSet assigns (or with define, declares) a split variable. An
// assignment is one statement, which a for loop's post statement can be.
func (fe *funcEmitter) splitSet(m string, v *types.Var, hi, lo string, g *pairGen, define bool) {
	if define {
		fe.declare(v)
	}
	vh, vl := fe.splitNames(v)
	if define {
		if len(g.pre) > 0 {
			fe.w.ln("%s%s;", m, strings.Join(g.pre, ", "))
			m = ""
		}
		fe.w.ln("%slet %s: number = %s, %s: number = %s;", m, vh, hi, vl, lo)
		return
	}
	var sets []string
	switch {
	case lo == vl && hi == vh: // x ^= 0
	case lo == vl:
		sets = []string{vh + " = " + hi}
	case hi == vh:
		sets = []string{vl + " = " + lo}
	case !strings.Contains(hi, vl):
		// hi does not read the old low half: assign lo first.
		sets = []string{vl + " = " + lo, vh + " = " + hi}
	default:
		t := g.temp(lo)
		sets = []string{vh + " = " + hi, vl + " = " + t}
	}
	if all := append(g.pre, sets...); len(all) > 0 {
		fe.w.ln("%s%s;", m, strings.Join(all, ", "))
	} else {
		fe.w.ln("%s;", m) // an empty statement keeps the source position
	}
}

// splitExpr lowers e, of type int64 or uint64, through its halves when it
// is pairable and uses a split variable, returning the BigInt.
func (fe *funcEmitter) splitExpr(e ast.Expr) (string, bool) {
	if fe.pe.split == nil || !pairableExpr(fe.info, e, fe.pe.split) || !fe.usesSplit(e) {
		return "", false
	}
	g := &pairGen{fe: fe}
	hi, lo := g.expr(e)
	return wrapPre(strings.Join(g.pre, ", "), fe.materialize(hi, lo, fe.info.TypeOf(e))), true
}

// splitConversion lowers T(x) for a pairable x that uses a split variable
// and a T whose values are JS numbers.
func (fe *funcEmitter) splitConversion(x ast.Expr, to types.Type) (string, bool) {
	if fe.pe.split == nil || !numberConversion(to) || !pairableExpr(fe.info, x, fe.pe.split) || !fe.usesSplit(x) {
		return "", false
	}
	g := &pairGen{fe: fe}
	hi, lo := g.expr(x)
	fromSigned := false
	if ii, _ := intKind(fe.info.TypeOf(x)); ii.signed {
		fromSigned = true
	}
	b := under(to).(*types.Basic)
	// number is the value of the halves as a JS number, read as signed or
	// unsigned: one rounding of an exact sum, as Number(BigInt) rounds.
	number := func(signed bool) string {
		if hi == "0" {
			return "(" + lo + " >>> 0)"
		}
		hi, lo := g.simple(hi), g.simple(lo)
		if signed {
			return "(" + hi + " * 4294967296 + (" + lo + " >>> 0))"
		}
		return "((" + hi + " >>> 0) * 4294967296 + (" + lo + " >>> 0))"
	}
	var s string
	if b.Info()&types.IsFloat != 0 {
		s = number(fromSigned)
	} else {
		ii, _ := intKind(to)
		if ii.bits == 64 {
			s = number(ii.signed)
		} else {
			s = wrap(lo, ii)
		}
	}
	return wrapPre(strings.Join(g.pre, ", "), s), true
}

func (fe *funcEmitter) usesSplit(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if v, ok := fe.info.Uses[id].(*types.Var); ok && fe.isSplit(v) {
				found = true
			}
		}
		return !found
	})
	return found
}

// pairGen lowers pairable expressions to their halves: JS expressions of
// int32 values. Intermediate values go into hoisted temporaries assigned
// by the comma expressions in pre, which must run before the halves are
// read.
type pairGen struct {
	fe  *funcEmitter
	pre []string
}

func (g *pairGen) temp(s string) string {
	t := g.fe.declareName("$p")
	g.fe.temps = append(g.fe.temps, t)
	g.pre = append(g.pre, t+" = "+s)
	return t
}

// simple returns s, or a temporary holding it, so that it can be repeated.
func (g *pairGen) simple(s string) string {
	if jsIdent.MatchString(s) || intLiteral(s) {
		return s
	}
	return g.temp(s)
}

func intLiteral(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// value lowers e for assignment to a split variable of type t: pairable
// expressions on halves, anything else as a BigInt that is then split.
func (g *pairGen) value(e ast.Expr, t types.Type) (string, string) {
	if pairableExpr(g.fe.info, e, g.fe.pe.split) {
		return g.expr(e)
	}
	return g.fromBig(g.fe.valueOf(e, t))
}

func (g *pairGen) fromBig(s string) (string, string) {
	t := g.temp(s)
	return "$rt.pairHi(" + t + ")", "$rt.pairLo(" + t + ")"
}

// constHalves returns the halves of an integer constant as int32 literals.
func constHalves(v constant.Value) (string, string) {
	x, _ := new(big.Int).SetString(constant.ToInt(v).ExactString(), 10)
	m := new(big.Int).Lsh(big.NewInt(1), 64)
	x.Mod(x, m) // two's complement of negative values
	u := x.Uint64()
	return strconv.Itoa(int(int32(uint32(u >> 32)))), strconv.Itoa(int(int32(uint32(u))))
}

func (g *pairGen) expr(e ast.Expr) (string, string) {
	info := g.fe.info
	if tv := info.Types[e]; tv.Value != nil {
		return constHalves(tv.Value)
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return g.expr(x.X)
	case *ast.Ident:
		return g.fe.splitNames(info.Uses[x].(*types.Var))
	case *ast.CallExpr: // a conversion
		arg := x.Args[0]
		at := info.TypeOf(arg)
		if isSplitType(at) {
			return g.expr(arg)
		}
		fi, _ := intKind(at)
		t := g.simple(g.fe.expr(arg))
		switch {
		case fi.bits <= 32 && fi.signed:
			return "(" + t + " >> 31)", "(" + t + " | 0)"
		case fi.bits <= 32:
			return "0", "(" + t + " | 0)"
		}
		// int, uint, uintptr: exact numbers below 2^53.
		return "(Math.floor(" + t + " / 4294967296) | 0)", "(" + t + " | 0)"
	case *ast.UnaryExpr:
		h, l := g.expr(x.X)
		switch x.Op {
		case token.SUB:
			return g.binary(token.SUB, "0", "0", h, l, nil)
		case token.XOR:
			return "(~" + h + ")", "(~" + l + ")"
		}
		return h, l
	case *ast.BinaryExpr:
		ah, al := g.expr(x.X)
		if x.Op == token.SHL || x.Op == token.SHR {
			return g.shift(x.Op, ah, al, shiftConst(info.Types[x.Y].Value), info.TypeOf(x))
		}
		bh, bl := g.expr(x.Y)
		bc := info.Types[x.Y].Value
		if bc == nil && x.Op == token.MUL {
			if ac := info.Types[x.X].Value; ac != nil {
				// A constant factor on the left.
				return g.binary(x.Op, bh, bl, ah, al, ac)
			}
		}
		return g.binary(x.Op, ah, al, bh, bl, bc)
	}
	g.fe.errorf(e.Pos(), "internal: not a pairable expression")
	return "0", "0"
}

func shiftConst(v constant.Value) int {
	n, ok := constant.Uint64Val(constant.ToInt(v))
	if !ok || n > 64 {
		return 64
	}
	return int(n)
}

// binary lowers a op b on halves; bc is b's constant value, if any.
func (g *pairGen) binary(op token.Token, ah, al, bh, bl string, bc constant.Value) (string, string) {
	half := func(a, b string) string {
		switch {
		case b == "0" && op != token.AND:
			return a // a | 0, a ^ 0, a &^ 0
		case b == "0" || a == "0":
			if op == token.AND || op == token.AND_NOT && a == "0" {
				return "0"
			}
		}
		if op == token.AND_NOT {
			return "(" + a + " & ~" + b + ")"
		}
		return "(" + a + " " + op.String() + " " + b + ")"
	}
	switch op {
	case token.AND, token.OR, token.XOR, token.AND_NOT:
		return half(ah, bh), half(al, bl)
	case token.ADD:
		ah, bh = g.simple(ah), g.simple(bh)
		s := g.temp("(" + al + " >>> 0) + (" + bl + " >>> 0)")
		return "((" + ah + " + " + bh + " + (" + s + " > 4294967295 ? 1 : 0)) | 0)", "(" + s + " | 0)"
	case token.SUB:
		ah, bh = g.simple(ah), g.simple(bh)
		s := g.temp("(" + al + " >>> 0) - (" + bl + " >>> 0)")
		return "((" + ah + " - " + bh + " - (" + s + " < 0 ? 1 : 0)) | 0)", "(" + s + " | 0)"
	case token.MUL:
		return g.mul(ah, al, bh, bl, bc)
	}
	g.fe.errorf(token.NoPos, "internal: operator %s on split halves", op)
	return "0", "0"
}

// mul lowers a * b mod 2^64: lo is the low 32 bits of al * bl; hi adds the
// high 32 bits of al * bl (mulhi, from 16-bit limbs whose products are
// exact doubles below 2^32) and the low 32 bits of ah * bl and al * bh.
func (g *pairGen) mul(ah, al, bh, bl string, bc constant.Value) (string, string) {
	al = g.simple(al)
	var mh string
	if bc != nil {
		h, l := constHalves(bc)
		lu := uint32(mustAtoi(l))
		bh, bl = h, strconv.FormatUint(uint64(lu), 10)
		switch {
		case lu == 0 && bh == "0":
			return "0", "0"
		case lu == 0:
			return "Math.imul(" + al + ", " + bh + ")", "0"
		case lu < 1<<16:
			mh = fmt.Sprintf("((((%s >>> 16) * %d) + (((%s & 65535) * %d) >>> 16)) >>> 16)", al, lu, al, lu)
		default:
			b0, b1 := lu&0xffff, lu>>16
			t := g.temp(fmt.Sprintf("(%s >>> 16) * %d + (((%s & 65535) * %d) >>> 16)", al, b0, al, b0))
			w := g.temp(fmt.Sprintf("(%s & 65535) + (%s & 65535) * %d", t, al, b1))
			mh = fmt.Sprintf("((%s >>> 16) * %d + (%s >>> 16) + (%s >>> 16))", al, b1, t, w)
		}
	} else {
		bl = g.simple(bl)
		t := g.temp(fmt.Sprintf("(%s >>> 16) * (%s & 65535) + (((%s & 65535) * (%s & 65535)) >>> 16)", al, bl, al, bl))
		w := g.temp(fmt.Sprintf("(%s & 65535) + (%s & 65535) * (%s >>> 16)", t, al, bl))
		mh = fmt.Sprintf("((%s >>> 16) * (%s >>> 16) + (%s >>> 16) + (%s >>> 16))", al, bl, t, w)
	}
	hi := mh
	if ah != "0" {
		hi += " + Math.imul(" + ah + ", " + bl + ")"
	}
	if bh != "0" {
		hi += " + Math.imul(" + al + ", " + bh + ")"
	}
	return "((" + hi + ") | 0)", "Math.imul(" + al + ", " + bl + ")"
}

func mustAtoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// shift lowers a << k or a >> k for a constant k on halves of type t.
func (g *pairGen) shift(op token.Token, ah, al string, k int, t types.Type) (string, string) {
	if k == 0 {
		return ah, al
	}
	ii, _ := intKind(t)
	if op == token.SHL {
		switch {
		case k >= 64:
			return "0", "0"
		case k < 32:
			ah, al = g.simple(ah), g.simple(al)
			return fmt.Sprintf("((%s << %d) | (%s >>> %d))", ah, k, al, 32-k), fmt.Sprintf("(%s << %d)", al, k)
		case k == 32:
			return "(" + al + " | 0)", "0"
		default:
			return fmt.Sprintf("(%s << %d)", al, k-32), "0"
		}
	}
	if ii.signed {
		ah = g.simple(ah)
		switch {
		case k >= 64:
			return "(" + ah + " >> 31)", "(" + ah + " >> 31)"
		case k < 32:
			al = g.simple(al)
			return fmt.Sprintf("(%s >> %d)", ah, k), fmt.Sprintf("((%s >>> %d) | (%s << %d))", al, k, ah, 32-k)
		case k == 32:
			return "(" + ah + " >> 31)", "(" + ah + " | 0)"
		default:
			return "(" + ah + " >> 31)", fmt.Sprintf("(%s >> %d)", ah, k-32)
		}
	}
	switch {
	case k >= 64:
		return "0", "0"
	case k < 32:
		ah, al = g.simple(ah), g.simple(al)
		return fmt.Sprintf("((%s >>> %d) | 0)", ah, k), fmt.Sprintf("((%s >>> %d) | (%s << %d))", al, k, ah, 32-k)
	case k == 32:
		return "0", "(" + ah + " | 0)"
	default:
		return "0", fmt.Sprintf("((%s >>> %d) | 0)", ah, k-32)
	}
}
