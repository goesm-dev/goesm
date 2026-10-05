package lower

import (
	"fmt"
	"go/types"
	"reflect"
	"regexp"
	"strings"
)

// json.Marshal of a value whose static type the lowering knows goes through
// an encoder generated for that type: a function building the JS value whose
// JSON.stringify is the encoding, the way one would write it by hand:
//
//	function $jenc3(v: any, d: number): any {
//		return { "id": $rt.jsonInt(v.ID), "name": $rt.jsonStr(v.Name) };
//	}
//
// The runtime's encoder walks type descriptors and builds the same objects
// through a copy of a template, several times slower. The generated
// encoders cover the types the runtime's covers that need no Go code:
// booleans, numbers but float32, strings, structs with plain json tags,
// slices but []byte, maps with string keys and pointers, none of whose types
// has methods. The $rt.json helpers throw to hand a value the encoding
// differs for (an int past 2^53, -0, a string that is not ASCII) to the
// runtime's encoder, which keeps Go's output.

var jsonNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var jsonIndexLikeRE = regexp.MustCompile(`^(?:\d+|__proto__)$`)

// jsonField is a field of a struct that a generated encoder writes.
type jsonField struct {
	name      string // the JSON name
	prop      string
	typ       types.Type
	omitEmpty bool
}

// jsonFields returns the fields encoding/json writes for s, or false if one
// needs Go's code: an embedded field, a tag option but omitempty, a name
// that is not plain, that JS orders as an index or that another one's
// matches when case is ignored.
func jsonFields(s *types.Struct) ([]jsonField, bool) {
	var fs []jsonField
	seen := map[string]bool{}
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		tag := s.Tag(i)
		if f.Embedded() || strings.Contains(tag, `\`) {
			return nil, false
		}
		if !f.Exported() {
			continue
		}
		name, omitEmpty := f.Name(), false
		if v, ok := reflect.StructTag(tag).Lookup("json"); ok {
			parts := strings.Split(v, ",")
			if parts[0] == "-" && len(parts) == 1 {
				continue
			}
			if parts[0] != "" {
				name = parts[0]
			}
			for _, o := range parts[1:] {
				if o != "omitempty" {
					return nil, false
				}
				omitEmpty = true
			}
		}
		if !jsonNameRE.MatchString(name) || jsonIndexLikeRE.MatchString(name) || seen[strings.ToLower(name)] {
			return nil, false
		}
		seen[strings.ToLower(name)] = true
		fs = append(fs, jsonField{name: name, prop: fieldProp(s, i), typ: f.Type(), omitEmpty: omitEmpty})
	}
	return fs, true
}

// jsonEncodable reports whether a generated encoder encodes values of t.
func jsonEncodable(t types.Type) bool {
	return jsonEncodableIn(t, map[types.Type]bool{})
}

func jsonEncodableIn(t types.Type, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if seen[t] {
		return true // checked further up
	}
	seen[t] = true
	switch t.(type) {
	case *types.TypeParam, *types.Interface:
		return false
	case *types.Named:
		if types.NewMethodSet(t).Len() > 0 || types.NewMethodSet(types.NewPointer(t)).Len() > 0 {
			return false // a Marshaler, a TextMarshaler or the like
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Bool, types.String, types.Float64,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return true
		}
	case *types.Struct:
		fs, ok := jsonFields(u)
		if !ok {
			return false
		}
		for _, f := range fs {
			if !jsonEncodableIn(f.typ, seen) {
				return false
			}
		}
		return true
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			return false // base64
		}
		return jsonEncodableIn(u.Elem(), seen)
	case *types.Map:
		if b, ok := u.Key().Underlying().(*types.Basic); !ok || b.Kind() != types.String {
			return false
		}
		return jsonEncodableIn(u.Key(), seen) && jsonEncodableIn(u.Elem(), seen)
	case *types.Pointer:
		if _, ok := u.Elem().Underlying().(*types.Array); ok {
			return false
		}
		return jsonEncodableIn(u.Elem(), seen)
	}
	return false
}

// jsonEncComposite reports whether json.Marshal of a value of static type t
// takes a generated encoder: t is a struct, slice, map or pointer type
// whose values it encodes.
func jsonEncComposite(t types.Type) bool {
	if t == nil {
		return false
	}
	switch types.Unalias(t).Underlying().(type) {
	case *types.Struct, *types.Slice, *types.Map, *types.Pointer:
		return !isIface(t) && jsonEncodable(t)
	}
	return false
}

// jsonValue returns the expression of the JS value encoding v, a value of
// type t, at depth d.
func (pe *pkgEmitter) jsonValue(t types.Type, v, d string) string {
	if b, ok := t.Underlying().(*types.Basic); ok {
		switch b.Kind() {
		case types.Int, types.Uint, types.Uintptr:
			return "$rt.jsonInt(" + v + ")"
		case types.Int64, types.Uint64:
			return "$rt.jsonInt64(" + v + ")"
		case types.Float64:
			return "$rt.jsonFloat(" + v + ")"
		case types.String:
			return "$rt.jsonStr(" + v + ")"
		}
		return v // booleans and numbers of up to 32 bits
	}
	return pe.jsonEncoder(t) + "(" + v + ", " + d + ")"
}

// jsonEmpty returns the condition that v, a value of type t, is empty as
// omitempty means it.
func jsonEmpty(t types.Type, v string) string {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Kind() == types.Bool:
			return v + " === false"
		case u.Kind() == types.String:
			return v + ` === ""`
		case isBigKind(u):
			return v + " === 0n"
		}
		return v + " === 0"
	case *types.Slice:
		return v + " === null || " + v + ".$length === 0"
	case *types.Map:
		return v + " === null || " + v + ".entries.size === 0"
	case *types.Pointer:
		return v + " === null"
	}
	return "false" // a struct
}

// jsonEncoder returns (emitting on first use) the generated encoder of the
// composite type t.
func (pe *pkgEmitter) jsonEncoder(t types.Type) string {
	if n, ok := pe.jsonEncs.At(t).(string); ok {
		return n
	}
	name := pe.fresh("jenc")
	pe.jsonEncs.Set(t, name)
	w := newWriter(pe.tab)
	w.ln("function %s(v: any, d: number): any {", name)
	w.indent++
	switch u := t.Underlying().(type) {
	case *types.Struct:
		fs, _ := jsonFields(u)
		omit := false
		for _, f := range fs {
			omit = omit || f.omitEmpty
		}
		if !omit {
			// An object literal: every value has one hidden class.
			var parts []string
			for _, f := range fs {
				parts = append(parts, jsString(f.name)+": "+pe.jsonValue(f.typ, "v."+f.prop, "d"))
			}
			w.ln("return { %s };", strings.Join(parts, ", "))
			break
		}
		w.ln("const o: any = {};")
		for _, f := range fs {
			x := "v." + f.prop
			set := fmt.Sprintf("o[%s] = %s;", jsString(f.name), pe.jsonValue(f.typ, x, "d"))
			if f.omitEmpty {
				w.ln("if (!(%s)) %s", jsonEmpty(f.typ, x), set)
			} else {
				w.ln("%s", set)
			}
		}
		w.ln("return o;")
	case *types.Slice:
		w.ln("if (v === null) return null;")
		w.ln("if (d > 100) $rt.jsonAbort(); // possibly a cycle")
		w.ln("const n = v.$length, arr = v.$array, off = v.$offset, a = new Array(n);")
		w.ln("for (let i = 0; i < n; i++) a[i] = %s;", pe.jsonValue(u.Elem(), "arr[off + i]", "d + 1"))
		w.ln("return a;")
	case *types.Map:
		// In Go's order of the keys, which JS keeps for keys that are not
		// indices (see $rt.jsonKey).
		w.ln("if (v === null) return null;")
		w.ln("if (d > 100) $rt.jsonAbort();")
		w.ln("const e = v.entries, ks = Array.from(e.keys()), o: any = {};")
		w.ln("if (ks.length > 1) ks.sort();")
		w.ln("for (let i = 0; i < ks.length; i++) o[$rt.jsonKey(ks[i])] = %s;", pe.jsonValue(u.Elem(), "e.get(ks[i])", "d + 1"))
		w.ln("return o;")
	case *types.Pointer:
		x := "v.v"
		if _, ok := u.Elem().Underlying().(*types.Struct); ok {
			x = "v" // a pointer to a struct is the struct object
		}
		w.ln("if (v === null) return null;")
		w.ln("if (d > 100) $rt.jsonAbort();")
		w.ln("return %s;", pe.jsonValue(u.Elem(), x, "d + 1"))
	}
	w.indent--
	w.ln("}")
	pe.jsonFuncs.append(w)
	return name
}
