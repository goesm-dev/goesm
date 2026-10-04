//go:build goesm

// goesm's patch of package log/slog's Value: the gc implementation keeps a
// string's or group's data pointer in Value.any and its length in num
// (unsafe.StringData, unsafe.String, unsafe.Slice), which goesm has no
// address space for. Here Value.any points to the string or slice itself,
// as a stringbox or groupbox in place of stringptr or groupptr.
package slog

type (
	stringbox *string // used in Value.any when the Value is a string
	groupbox  *[]Attr // used in Value.any when the Value is a []Attr
)

// Kind returns v's Kind.
func (v Value) Kind() Kind {
	switch x := v.any.(type) {
	case Kind:
		return x
	case stringbox:
		return KindString
	case timeLocation, timeTime:
		return KindTime
	case groupbox:
		return KindGroup
	case LogValuer:
		return KindLogValuer
	case kind: // a kind is just a wrapper for a Kind
		return KindAny
	default:
		return KindAny
	}
}

// StringValue returns a new [Value] for a string.
func StringValue(value string) Value {
	return Value{num: uint64(len(value)), any: stringbox(&value)}
}

// GroupValue returns a new [Value] for a list of Attrs.
// The caller must not subsequently mutate the argument slice.
func GroupValue(as ...Attr) Value {
	// Remove empty groups.
	// It is simpler overall to do this at construction than
	// to check each Group recursively for emptiness.
	if n := countEmptyGroups(as); n > 0 {
		as2 := make([]Attr, 0, len(as)-n)
		for _, a := range as {
			if !a.Value.isEmptyGroup() {
				as2 = append(as2, a)
			}
		}
		as = as2
	}
	return Value{num: uint64(len(as)), any: groupbox(&as)}
}

// String returns Value's value as a string, formatted like [fmt.Sprint]. Unlike
// the methods Int64, Float64, and so on, which panic if v is of the
// wrong kind, String never panics.
func (v Value) String() string {
	if sp, ok := v.any.(stringbox); ok {
		return *sp
	}
	var buf []byte
	return string(v.append(buf))
}

func (v Value) str() string {
	return *v.any.(stringbox)
}

// Group returns v's value as a []Attr.
// It panics if v's [Kind] is not [KindGroup].
func (v Value) Group() []Attr {
	if sp, ok := v.any.(groupbox); ok {
		return *sp
	}
	panic("Group: bad kind")
}

func (v Value) group() []Attr {
	return *v.any.(groupbox)
}
