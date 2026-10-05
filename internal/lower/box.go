package lower

import (
	"fmt"
	"go/types"
	"strings"
)

// Box classes. An interface value is an object with the dynamic type t and
// the value v ($rt.Iface), and an interface method call is a call of its
// method "$" + key (icall). For a type that has methods called through
// interfaces, the lowering emits the class of its interface values, whose
// methods call the type's functions directly: each call site sees one class
// per dynamic type, which engines resolve and inline as they do a method of
// a JS class. The interface values of other types (generic or unnamed
// types, the runtime's) are plain Ifaces, whose "$" + key methods (set by
// $rt.addMethods) call through the type's method table.
//
// The box of a struct type is flat where it can be: a struct object of a
// subclass of the type's class, with t, and v referring to itself. Boxing a
// struct value then allocates one object, not two. A struct with a field
// named t or v keeps a box holding the value.

// boxInfo describes the box class of a non-interface type.
type boxInfo struct {
	name    string // the class name, in the defining package
	flat    bool   // the box is the struct object itself
	methods []*types.Selection
}

type boxKey struct {
	named *types.Named
	ptr   bool
}

// boxOf returns the box class of T, a package-level non-generic defined type
// or a pointer to one, or nil if it has none.
func (pe *pkgEmitter) boxOf(T types.Type) *boxInfo {
	return pe.prog.BoxOf(T)
}

// BoxOf is boxOf, cached. It depends only on DynMethod and CalledMethod of
// the methods of T, which Facts covers.
func (p *Program) BoxOf(T types.Type) *boxInfo {
	base, ptr := types.Unalias(T), false
	if pt, ok := base.(*types.Pointer); ok {
		base, ptr = types.Unalias(pt.Elem()), true
	}
	named, ok := base.(*types.Named)
	if !ok {
		return nil
	}
	k := boxKey{named, ptr}
	if b, ok := p.boxes.Load(k); ok {
		return b.(*boxInfo)
	}
	b := p.newBoxInfo(named, ptr)
	p.boxes.Store(k, b)
	return b
}

func (p *Program) newBoxInfo(named *types.Named, ptr bool) *boxInfo {
	if named.TypeParams().Len() > 0 || named.TypeArgs().Len() > 0 || isIface(named) {
		return nil
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return nil // local types are named per function
	}
	if _, ok := named.Underlying().(*types.Pointer); ok {
		return nil
	}
	var T types.Type = named
	b := &boxInfo{name: jsName(obj.Name()) + "$$box"}
	if ptr {
		T = types.NewPointer(named)
		b.name = jsName(obj.Name()) + "$$pbox"
	}
	ms := types.NewMethodSet(T)
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		fn := sel.Obj().(*types.Func)
		if fn.Signature().TypeParams().Len() > 0 || !p.DynMethod(fn) || !p.CalledMethod(fn) {
			continue
		}
		b.methods = append(b.methods, sel)
	}
	if len(b.methods) == 0 {
		return nil
	}
	if st, ok := named.Underlying().(*types.Struct); ok && !ptr {
		b.flat = true
		for i := 0; i < st.NumFields(); i++ {
			if f := fieldProp(st, i); f == "t" || f == "v" {
				b.flat = false
			}
		}
	}
	return b
}

// boxMethodProp is the property of an interface value's method key, as
// accessed (".$M" or "[...]") and as declared in a class.
func boxMethodProp(key string) (access, decl string) {
	if jsIdent.MatchString(key) {
		return ".$" + key, "$" + key
	}
	q := jsString("$" + key)
	return "[" + q + "]", "[" + q + "]"
}

// emitBoxes emits the box classes of named and *named, defined in this
// package, and registers them in the type's initializer w (non-generic
// types only). A flat box's class is emitted with the struct class
// (emitStructClass).
func (pe *pkgEmitter) emitBoxes(w *writer, name string, named *types.Named) {
	for _, T := range []types.Type{named, types.NewPointer(named)} {
		b := pe.boxOf(T)
		if b == nil {
			continue
		}
		desc := name + "$type"
		if _, ok := T.(*types.Pointer); ok {
			desc = "$rt.ptrTo(" + desc + ")"
		}
		if !b.flat {
			cw := newWriter(pe.tab)
			cw.ln("class %s {", b.name)
			cw.indent++
			cw.ln("declare t: $rt.Type;")
			cw.ln("declare v: any;")
			cw.ln("constructor(t: $rt.Type, v: any) { this.t = t; this.v = v; }")
			pe.boxMethods(cw, T, b, "this.v")
			cw.indent--
			cw.ln("}")
			pe.classes.append(cw)
		}
		w.ln("$rt.setBox(%s, %s, %t);", desc, b.name, b.flat)
	}
}

// boxMethods emits the methods of box b of type T, whose receiver is recv.
func (pe *pkgEmitter) boxMethods(w *writer, T types.Type, b *boxInfo, recv string) {
	for _, sel := range b.methods {
		fn := sel.Obj().(*types.Func)
		key := methodKey(fn)
		_, decl := boxMethodProp(key)
		n := fn.Signature().Params().Len()
		var ps, as []string
		for i := 0; i < n; i++ {
			ps = append(ps, fmt.Sprintf("a%d: any", i))
			as = append(as, fmt.Sprintf(", a%d", i))
		}
		args := strings.Join(as, "")
		callee := pe.methodEntry(T, sel, tpScope{})
		if callee != pe.methodFuncName(fn) {
			// A wrapper (a promoted method, or a value method of a
			// pointer): the table holds it.
			callee = fmt.Sprintf("this.t.mt[%s]", jsString(key))
		}
		w.ln("%s(%s): any { return %s(%s%s); }", decl, strings.Join(ps, ", "), callee, recv, args)
	}
}

// boxValue boxes s, a value of type from (copied) that has box b, into an
// interface.
func (fe *funcEmitter) boxValue(s string, from types.Type, b *boxInfo) string {
	base, _ := derefType(types.Unalias(from))
	named := types.Unalias(base).(*types.Named)
	if named.Obj().Pkg() != fe.pe.pkg.Types {
		return fmt.Sprintf("$rt.boxOf(%s, %s)", fe.desc(from), s)
	}
	if !b.flat {
		return fmt.Sprintf("new %s(%s, %s)", b.name, fe.desc(from), s)
	}
	// A composite literal constructs the box itself.
	cls := fe.pe.namedTypeName(named.Obj())
	marks := ""
	for len(s) > 0 && s[0] == markStart {
		j := strings.IndexByte(s, markEnd)
		marks, s = marks+s[:j+1], s[j+1:]
	}
	if args, ok := ctorArgs(s, cls); ok {
		return marks + "new " + b.name + "(" + args + ")"
	}
	s = marks + s
	return b.name + ".$of(" + s + ")"
}

// ctorArgs returns the arguments of s if s is exactly "new cls(args)".
func ctorArgs(s, cls string) (string, bool) {
	prefix := "new " + cls + "("
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
		return "", false
	}
	depth := 0
	for i := len(prefix) - 1; i < len(s); i++ {
		switch c := s[i]; c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 && i != len(s)-1 {
				return "", false
			}
		case '"', '\'':
			for i++; i < len(s) && s[i] != c; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '`', '/':
			return "", false // template literals, regexps and comments
		}
	}
	if depth != 0 {
		return "", false
	}
	return s[len(prefix) : len(s)-1], true
}
