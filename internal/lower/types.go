package lower

import (
	"fmt"
	"go/types"
	"math"
	"strconv"
	"strings"
)

var jsReserved = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`break case catch class const continue debugger default delete do else
		enum export extends false finally for function if import in instanceof new null return super
		switch this throw true try typeof var void while with yield let static implements interface
		package private protected public await async eval arguments undefined NaN Infinity of
		globalThis`) {
		jsReserved[w] = true
	}
}

// jsName maps a Go identifier to a JS binding name. Go identifiers never
// contain '$', so every goesm-introduced name uses '$' and cannot collide.
func jsName(name string) string {
	if jsReserved[name] {
		return name + "$"
	}
	return name
}

func jsPropName(name string) string {
	switch name {
	case "constructor", "__proto__":
		return name + "$"
	}
	return name
}

// fieldProp is the JS property holding field i of struct s.
func fieldProp(s *types.Struct, i int) string {
	name := s.Field(i).Name()
	if name == "_" {
		return fmt.Sprintf("$blank%d", i)
	}
	return jsPropName(name)
}

// jsString renders Go string bytes as a JS string literal in goesm's string
// representation (one UTF-16 code unit per byte).
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c >= 0x20 && c < 0x7f:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\x%02x`, c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func formatFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case math.IsNaN(f):
		return "NaN"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// tpScope maps type parameters in scope to the JS names of their runtime
// type descriptors (generic code receives descriptors as dictionary
// parameters). inline disables hoisting of descriptor expressions, needed in
// code that may run before hoisted constants are initialised.
type tpScope struct {
	names  map[*types.TypeParam]string
	inline bool
	// ts are type parameters declared as TypeScript type parameters of
	// the enclosing class (they have no dictionary variable).
	ts map[*types.TypeParam]bool
}

func (s tpScope) with(tp *types.TypeParam, name string) tpScope {
	m := map[*types.TypeParam]string{}
	for k, v := range s.names {
		m[k] = v
	}
	m[tp] = name
	return tpScope{names: m, inline: s.inline, ts: s.ts}
}

var basicDesc = map[types.BasicKind]string{
	types.Bool: "bool", types.Int: "int", types.Int8: "int8", types.Int16: "int16", types.Int32: "int32",
	types.Int64: "int64", types.Uint: "uint", types.Uint8: "uint8", types.Uint16: "uint16",
	types.Uint32: "uint32", types.Uint64: "uint64", types.Uintptr: "uintptr", types.Float32: "float32",
	types.Float64: "float64", types.Complex64: "complex64", types.Complex128: "complex128",
	types.String: "string", types.UnsafePointer: "unsafePointer", types.UntypedComplex: "complex128",
	types.UntypedBool: "bool", types.UntypedInt: "int", types.UntypedRune: "int32",
	types.UntypedFloat: "float64", types.UntypedString: "string",
}

// typeDesc returns a JS expression evaluating to the runtime descriptor of t.
func (pe *pkgEmitter) typeDesc(t types.Type, tp tpScope) string {
	t = types.Unalias(t)
	if b, ok := t.(*types.Basic); ok {
		if n, ok := basicDesc[b.Kind()]; ok {
			return "$rt.types." + n
		}
		pe.errorf(0, "unsupported basic type %s", b)
		return "$rt.types.int"
	}
	if tpar, ok := t.(*types.TypeParam); ok {
		if n, ok := tp.names[tpar]; ok {
			return n
		}
		pe.errorf(tpar.Obj().Pos(), "internal: type parameter %s not in scope", tpar)
		return "undefined"
	}
	if named, ok := t.(*types.Named); ok && named.TypeArgs().Len() == 0 && len(outerTypeParams(named.Obj())) == 0 {
		return pe.namedDesc(named)
	}
	if tp.inline || hasTypeParam(t) {
		d := pe.buildDesc(t, tp)
		if promotedMethods(t) {
			d = "$rt.withMethods(" + d + ", {" + strings.Join(pe.methodEntries(t, nil, tp), ", ") + "})"
		}
		return d
	}
	if name, ok := pe.typeConsts.At(t).(string); ok {
		return name
	}
	expr := pe.buildDesc(t, tp)
	name := pe.fresh("t")
	pe.typeConsts.Set(t, name)
	pe.consts.ln("const %s = %s;", name, expr)
	if promotedMethods(t) {
		// After the const, so method signatures may refer to t itself.
		entries := pe.methodEntries(t, nil, tp)
		pe.consts.ln("$rt.addMethods(%s, {%s});", name, strings.Join(entries, ", "))
	}
	return name
}

func (pe *pkgEmitter) namedDesc(named *types.Named) string {
	obj := named.Obj()
	if obj.Pkg() == nil {
		if obj.Name() == "error" {
			return "$rt.errorType"
		}
		pe.errorf(0, "unsupported universe type %s", obj.Name())
		return "undefined"
	}
	if obj.Pkg() == pe.pkg.Types {
		return pe.namedTypeName(obj) + "$type"
	}
	return pe.qualify(obj.Pkg(), jsName(obj.Name())+"$type")
}

func (pe *pkgEmitter) buildDesc(t types.Type, tp tpScope) string {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		var args []string
		for _, p := range outerTypeParams(t.Origin().Obj()) {
			args = append(args, pe.typeDesc(p, tp))
		}
		for i := 0; i < t.TypeArgs().Len(); i++ {
			args = append(args, pe.typeDesc(t.TypeArgs().At(i), tp))
		}
		return pe.namedDesc(t.Origin()) + "(" + strings.Join(args, ", ") + ")"
	case *types.Pointer:
		return "$rt.ptrTo(" + pe.typeDesc(t.Elem(), tp) + ")"
	case *types.Slice:
		return "$rt.sliceOf(" + pe.typeDesc(t.Elem(), tp) + ")"
	case *types.Array:
		return fmt.Sprintf("$rt.arrayOf(%s, %d)", pe.typeDesc(t.Elem(), tp), t.Len())
	case *types.Map:
		return "$rt.mapOf(" + pe.typeDesc(t.Key(), tp) + ", " + pe.typeDesc(t.Elem(), tp) + ")"
	case *types.Chan:
		dir := 3
		switch t.Dir() {
		case types.SendOnly:
			dir = 1
		case types.RecvOnly:
			dir = 2
		}
		return fmt.Sprintf("$rt.chanOf(%s, %d)", pe.typeDesc(t.Elem(), tp), dir)
	case *types.Signature:
		var ps, rs []string
		for i := 0; i < t.Params().Len(); i++ {
			ps = append(ps, pe.typeDesc(t.Params().At(i).Type(), tp))
		}
		for i := 0; i < t.Results().Len(); i++ {
			rs = append(rs, pe.typeDesc(t.Results().At(i).Type(), tp))
		}
		return fmt.Sprintf("$rt.funcOf([%s], [%s], %v)", strings.Join(ps, ", "), strings.Join(rs, ", "), t.Variadic())
	case *types.Interface:
		var ms []string
		for i := 0; i < t.NumMethods(); i++ {
			m := t.Method(i)
			ms = append(ms, fmt.Sprintf("{ name: %s, pkgPath: %s, type: %s }", jsString(m.Name()), jsString(methodPkgPath(m)), pe.typeDesc(m.Type(), tp)))
		}
		return "$rt.interfaceOf([" + strings.Join(ms, ", ") + "])"
	case *types.Struct:
		return pe.structDesc(t, pe.anonStructClass(t), tp)
	}
	pe.errorf(0, "unsupported type %s", t)
	return "undefined"
}

func methodPkgPath(f *types.Func) string {
	if f.Exported() || f.Pkg() == nil {
		return ""
	}
	return goPkgPath(f.Pkg())
}

// goPkgPath is the package path the gc runtime reports (reflect's PkgPath,
// unexported method and field keys, panic messages): a main package is
// "main" whatever its import path.
func goPkgPath(p *types.Package) string {
	if p.Name() == "main" {
		return "main"
	}
	return p.Path()
}

func methodKey(f *types.Func) string {
	if p := methodPkgPath(f); p != "" {
		return p + "." + f.Name()
	}
	return f.Name()
}

func (pe *pkgEmitter) structDesc(s *types.Struct, ctor string, tp tpScope) string {
	var fs []string
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		pkgPath := ""
		if !f.Exported() && f.Pkg() != nil {
			pkgPath = goPkgPath(f.Pkg())
		}
		fs = append(fs, fmt.Sprintf("{ name: %s, pkgPath: %s, type: %s, embedded: %v, tag: %s, prop: %s }",
			jsString(f.Name()), jsString(pkgPath), pe.typeDesc(f.Type(), tp), f.Embedded(), jsString(s.Tag(i)), jsString(fieldProp(s, i))))
	}
	return "$rt.structOf([" + strings.Join(fs, ", ") + "], " + ctor + ")"
}

// anonStructClass returns (emitting on first use) the class for an unnamed
// struct type.
func (pe *pkgEmitter) anonStructClass(s *types.Struct) string {
	if n, ok := pe.anonStructs.At(s).(string); ok {
		return n
	}
	name := pe.fresh("S")
	pe.anonStructs.Set(s, name)
	pe.emitStructClass(name, s, nil, false)
	return name
}

// zeroOf returns a JS expression producing a fresh zero value of t.
func (pe *pkgEmitter) zeroOf(t types.Type, tp tpScope) string {
	t = types.Unalias(t)
	if tpar, ok := t.(*types.TypeParam); ok {
		return pe.typeDesc(tpar, tp) + ".zero()"
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "false"
		case u.Info()&types.IsString != 0:
			return `""`
		case u.Info()&types.IsComplex != 0:
			return "$rt.complexZero"
		case isBigKind(u):
			return "0n"
		case u.Info()&types.IsNumeric != 0:
			return "0"
		}
		return "null"
	case *types.Struct:
		var args []string
		for i := 0; i < u.NumFields(); i++ {
			args = append(args, pe.zeroOf(u.Field(i).Type(), tp))
		}
		return "new " + pe.structClass(t) + "(" + strings.Join(args, ", ") + ")"
	case *types.Array:
		if u.Len() <= 8 {
			z := make([]string, u.Len())
			for i := range z {
				z[i] = pe.zeroOf(u.Elem(), tp)
			}
			return "[" + strings.Join(z, ", ") + "]"
		}
		return pe.typeDesc(t, tp) + ".zero()"
	}
	return "null"
}

// zeroFn returns a JS function expression producing zero values of t.
func (pe *pkgEmitter) zeroFn(t types.Type, tp tpScope) string {
	if tpar, ok := types.Unalias(t).(*types.TypeParam); ok {
		return pe.typeDesc(tpar, tp) + ".zero"
	}
	return "() => " + pe.zeroOf(t, tp)
}

// structClass returns the JS class constructing values of struct type t.
func (pe *pkgEmitter) structClass(t types.Type) string {
	if named, ok := types.Unalias(t).(*types.Named); ok {
		obj := named.Origin().Obj()
		if obj.Pkg() == pe.pkg.Types {
			return pe.namedTypeName(obj)
		}
		return pe.qualify(obj.Pkg(), jsName(obj.Name()))
	}
	return pe.anonStructClass(t.Underlying().(*types.Struct))
}

// copyExpr returns an expression copying the aggregate value s of type t.
func (pe *pkgEmitter) copyExpr(s string, t types.Type, tp tpScope) string {
	t = types.Unalias(t)
	if _, ok := t.(*types.TypeParam); ok {
		if basicTypeParam(t) {
			return s
		}
		return "$rt.copy(" + pe.typeDesc(t, tp) + ", " + s + ")"
	}
	switch t.Underlying().(type) {
	case *types.Struct:
		if isGenericType(t) {
			return s + ".$clone(" + pe.typeDesc(t, tp) + ")"
		}
		return s + ".$clone()"
	case *types.Array:
		return "$rt.copy(" + pe.typeDesc(t, tp) + ", " + s + ")"
	}
	return s
}

// tsType renders a TypeScript annotation. Annotations document the IR for
// readers and debuggers; they are erased by esbuild and never checked.
func (pe *pkgEmitter) tsType(t types.Type, tp tpScope) string {
	return pe.tsTypeIn(t, tp, nil)
}

// tsTypeIn renders t; in holds the named types being rendered, so a type
// defined in terms of itself (type S []S) is any at the recursion.
func (pe *pkgEmitter) tsTypeIn(t types.Type, tp tpScope, in map[string]bool) string {
	t = types.Unalias(t)
	if n, ok := t.(*types.Named); ok {
		if _, isStruct := n.Underlying().(*types.Struct); !isStruct {
			key := types.TypeString(n, nil)
			if in[key] {
				return "any"
			}
			m := map[string]bool{key: true}
			for k := range in {
				m[k] = true
			}
			in = m
		}
	}
	switch u := t.(type) {
	case *types.TypeParam:
		// Under erasure a type parameter is typed by what its constraint
		// allows: its core type, number or string for a numeric or string
		// union, else the TypeScript type parameter declared with the same
		// name (any outside its function or class, e.g. in a local type).
		if c := tsConstraint(pe, u, tp); c != "" {
			return c
		}
		if _, ok := tp.names[u]; ok || tp.ts[u] {
			return jsName(u.Obj().Name())
		}
		return "any"
	case *types.Named:
		if _, ok := u.Underlying().(*types.Struct); ok && u.TypeArgs().Len() == 0 {
			return pe.structClass(u)
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			return "boolean"
		case u.Info()&types.IsString != 0:
			return "string"
		case u.Info()&types.IsComplex != 0:
			return "$rt.Complex"
		case isBigKind(u):
			return "bigint"
		case u.Info()&types.IsNumeric != 0:
			return "number"
		}
	case *types.Slice:
		return "$rt.S<" + pe.tsTypeIn(u.Elem(), tp, in) + ">"
	case *types.Map:
		return "$rt.M<" + pe.tsTypeIn(u.Key(), tp, in) + ", " + pe.tsTypeIn(u.Elem(), tp, in) + ">"
	case *types.Chan:
		return "$rt.Chan<" + pe.tsTypeIn(u.Elem(), tp, in) + "> | null"
	case *types.Interface:
		return "$rt.Iface | null"
	case *types.Array:
		return pe.tsTypeIn(u.Elem(), tp, in) + "[]"
	}
	return "any"
}

// tsConstraint is the TypeScript type of values of type parameter p: its
// core type, or number / string for a union of only numeric / only string
// types; "" if there is none.
func tsConstraint(pe *pkgEmitter, p *types.TypeParam, tp tpScope) string {
	if c := under(p); c != p.Underlying() {
		if t := pe.tsType(c, tp); t != "any" {
			return t
		}
		return ""
	}
	iface, _ := p.Constraint().Underlying().(*types.Interface)
	kinds := map[string]bool{}
	for i := 0; iface != nil && i < iface.NumEmbeddeds(); i++ {
		var terms []types.Type
		switch e := iface.EmbeddedType(i).(type) {
		case *types.Union:
			for j := 0; j < e.Len(); j++ {
				terms = append(terms, e.Term(j).Type())
			}
		default:
			terms = append(terms, e)
		}
		for _, term := range terms {
			b, ok := term.Underlying().(*types.Basic)
			if !ok {
				return ""
			}
			kinds[pe.tsType(b, tp)] = true
		}
	}
	if len(kinds) == 1 && (kinds["number"] || kinds["string"]) {
		for k := range kinds {
			return k
		}
	}
	return ""
}

// isBig reports whether values of t are BigInts: int64 and uint64 (and
// types defined from them). Type parameters are not: their operations
// dispatch on the type argument at run time.
func isBig(t types.Type) bool {
	if isTypeParam(t) {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && isBigKind(b)
}

func isBigKind(b *types.Basic) bool {
	return b.Kind() == types.Int64 || b.Kind() == types.Uint64
}

// isIface reports whether t is an interface type proper. go/types reports
// type parameters as interfaces too, but their values are not boxed: a
// type parameter is represented as its type argument's representation.
func isIface(t types.Type) bool {
	if _, ok := types.Unalias(t).(*types.TypeParam); ok {
		return false
	}
	return types.IsInterface(t)
}

// under returns the underlying type, or for a type parameter its core type
// (the single underlying type of its type set) when there is one.
func under(t types.Type) types.Type {
	tp, ok := types.Unalias(t).(*types.TypeParam)
	if !ok {
		return t.Underlying()
	}
	iface := tp.Constraint().Underlying().(*types.Interface)
	var core types.Type
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		var terms []types.Type
		switch e := iface.EmbeddedType(i).(type) {
		case *types.Union:
			for j := 0; j < e.Len(); j++ {
				terms = append(terms, e.Term(j).Type())
			}
		default:
			terms = append(terms, e)
		}
		for _, term := range terms {
			u := under(term)
			if _, isIface := u.(*types.Interface); isIface {
				continue
			}
			if core == nil {
				core = u
			} else if !types.Identical(core, u) {
				return t.Underlying()
			}
		}
	}
	if core == nil {
		return t.Underlying()
	}
	return core
}
