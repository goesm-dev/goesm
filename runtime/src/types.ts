// Runtime type descriptors.
//
// Every Go type that matters at run time (interface boxing, type assertions,
// map hashing, equality, zero values, generic dictionaries and, later,
// reflect) has exactly one descriptor object. Identity of descriptors is Go
// type identity: named types are created once by their defining package and
// composite types are memoized by structure, so `a === b` iff the Go types are
// identical. This is the hook reflect will build on.

// Kind numbering follows reflect.Kind so that a future reflect implementation
// can expose it directly.
import { complexZero } from "./complex.ts";

export const Kind = {
  Invalid: 0, Bool: 1, Int: 2, Int8: 3, Int16: 4, Int32: 5, Int64: 6,
  Uint: 7, Uint8: 8, Uint16: 9, Uint32: 10, Uint64: 11, Uintptr: 12,
  Float32: 13, Float64: 14, Complex64: 15, Complex128: 16, Array: 17,
  Chan: 18, Func: 19, Interface: 20, Map: 21, Pointer: 22, Slice: 23,
  String: 24, Struct: 25, UnsafePointer: 26,
} as const;

export interface Field {
  name: string;
  pkgPath: string; // "" for exported fields
  type: Type;
  embedded: boolean;
  tag: string;
  prop: string; // JS property name holding the field
}

export interface IMethod {
  name: string;
  pkgPath: string; // "" for exported methods
  type: Type; // func type without receiver
}

export interface MethodImpl {
  fn: (recv: any, ...args: any[]) => any;
  type: Type;
}

let nextID = 1;

// The method table of each type with methods is an empty object whose
// prototype holds the functions: one hidden class per type, so the access
// t.mt[key] at a call site sees one class per dynamic type and engines
// resolve it, as a method of a JS class, to the type's function, which they
// can inline (the function stored in an object shared by every table would
// be a value unknown to the call site).
const noMethods: Record<string, (recv: any, ...args: any[]) => any> = Object.freeze({}) as any;

export class Type {
  id = nextID++;
  kind = 0;
  str = "";            // Go spelling, e.g. "main.User", "[]int"
  name = "";           // for named types
  pkgPath = "";        // for named types
  typeArgs: Type[] = [];
  nOuter = 0; // leading typeArgs of the function declaring a local type
  elem: Type | null = null;
  key: Type | null = null;
  len = 0;
  dir = 0;             // chan: 1 send, 2 recv, 3 both
  fields: Field[] = [];
  imethods: IMethod[] = [];
  params: Type[] = [];
  results: Type[] = [];
  variadic = false;
  // Method set of this type (for named T: value receiver methods; for *T: all).
  methods = new Map<string, MethodImpl>();
  ptr: Type | null = null; // *T, once ptrTo made it
  zeroSized = 0; // for a struct: 0 not known yet, 1 no, 2 zero-sized (see zeroSized)
  // The same methods' functions by key, for calls through interfaces: a
  // property access is cached at each call site (see icall in the lowering).
  // The functions live on a prototype of the type's own (see addMethods).
  mt: Record<string, (recv: any, ...args: any[]) => any> = noMethods;
  // Constructs the zero value.
  zero: () => any = () => null;
  // JS class for struct types (named or not).
  ctor: any = null;
  named = false;
  underlying: Type = this;

  toString(): string { return this.str; }
}

function basic(kind: number, str: string, zero: () => any): Type {
  const t = new Type();
  t.kind = kind;
  t.str = str;
  t.zero = zero;
  return t;
}

const zeroNum = () => 0;
// zeroByte is the zero value function of uint8 (byte). Slices made with it
// (make, append, []byte(s)) are backed by a Uint8Array instead of an Array.
export const zeroByte = (): number => 0;
const zeroBig = () => 0n;
// The basic types, as separate constants so that bundlers drop the unused
// ones; types holds them all for code that looks one up by name. The kinds
// are literals: a bundler keeps a call whose arguments read a property.
export const tBool: Type = /* @__PURE__ */ basic(1 /* Bool */, "bool", () => false);
export const tInt: Type = /* @__PURE__ */ basic(2 /* Int */, "int", zeroNum);
export const tInt8: Type = /* @__PURE__ */ basic(3 /* Int8 */, "int8", zeroNum);
export const tInt16: Type = /* @__PURE__ */ basic(4 /* Int16 */, "int16", zeroNum);
export const tInt32: Type = /* @__PURE__ */ basic(5 /* Int32 */, "int32", zeroNum);
export const tInt64: Type = /* @__PURE__ */ basic(6 /* Int64 */, "int64", zeroBig);
export const tUint: Type = /* @__PURE__ */ basic(7 /* Uint */, "uint", zeroNum);
export const tUint8: Type = /* @__PURE__ */ basic(8 /* Uint8 */, "uint8", zeroByte);
export const tUint16: Type = /* @__PURE__ */ basic(9 /* Uint16 */, "uint16", zeroNum);
export const tUint32: Type = /* @__PURE__ */ basic(10 /* Uint32 */, "uint32", zeroNum);
export const tUint64: Type = /* @__PURE__ */ basic(11 /* Uint64 */, "uint64", zeroBig);
export const tUintptr: Type = /* @__PURE__ */ basic(12 /* Uintptr */, "uintptr", zeroNum);
export const tFloat32: Type = /* @__PURE__ */ basic(13 /* Float32 */, "float32", zeroNum);
export const tFloat64: Type = /* @__PURE__ */ basic(14 /* Float64 */, "float64", zeroNum);
export const tComplex64: Type = /* @__PURE__ */ basic(15 /* Complex64 */, "complex64", () => complexZero);
export const tComplex128: Type = /* @__PURE__ */ basic(16 /* Complex128 */, "complex128", () => complexZero);
export const tString: Type = /* @__PURE__ */ basic(24 /* String */, "string", () => "");
export const tUnsafePointer: Type = /* @__PURE__ */ basic(26 /* UnsafePointer */, "unsafe.Pointer", () => null);
export const types = {
  bool: tBool,
  int: tInt,
  int8: tInt8,
  int16: tInt16,
  int32: tInt32,
  int64: tInt64,
  uint: tUint,
  uint8: tUint8,
  uint16: tUint16,
  uint32: tUint32,
  uint64: tUint64,
  uintptr: tUintptr,
  float32: tFloat32,
  float64: tFloat64,
  complex64: tComplex64,
  complex128: tComplex128,
  string: tString,
  unsafePointer: tUnsafePointer,
};

const memo = new Map<string, Type>();
function memoized(key: string, make: () => Type): Type {
  let t = memo.get(key);
  if (!t) {
    t = make();
    memo.set(key, t);
  }
  return t;
}

export function sliceOf(elem: Type): Type {
  return memoized(`[]${elem.id}`, () => {
    const t = new Type();
    t.kind = Kind.Slice;
    t.elem = elem;
    t.str = `[]${elem.str}`;
    return t;
  });
}

export function arrayOf(elem: Type, len: number): Type {
  return memoized(`[${len}]${elem.id}`, () => {
    const t = new Type();
    t.kind = Kind.Array;
    t.elem = elem;
    t.len = len;
    t.str = `[${len}]${elem.str}`;
    // An array of primitive zero values is a copy of a packed template,
    // several times faster than filling a new (holey) Array.
    let tmpl: any[] | undefined;
    t.zero = () => {
      if (tmpl !== undefined) return tmpl.slice();
      const a = new Array(len);
      if (len === 0) return a;
      const z = elem.zero();
      if (typeof z !== "object" || z === null) {
        if (z === undefined) return a.fill(z);
        tmpl = [];
        for (let i = 0; i < len; i++) tmpl.push(z);
        return tmpl.slice();
      }
      a[0] = z;
      for (let i = 1; i < len; i++) a[i] = elem.zero();
      return a;
    };
    return t;
  });
}

export function ptrTo(elem: Type): Type {
  if (elem.ptr !== null) return elem.ptr;
  return (elem.ptr = memoized(`*${elem.id}`, () => {
    const t = new Type();
    t.kind = Kind.Pointer;
    t.elem = elem;
    t.str = `*${elem.str}`;
    return t;
  }));
}

export function mapOf(key: Type, elem: Type): Type {
  return memoized(`map[${key.id}]${elem.id}`, () => {
    const t = new Type();
    t.kind = Kind.Map;
    t.key = key;
    t.elem = elem;
    t.str = `map[${key.str}]${elem.str}`;
    return t;
  });
}

export function chanOf(elem: Type, dir: number): Type {
  return memoized(`chan${dir} ${elem.id}`, () => {
    const t = new Type();
    t.kind = Kind.Chan;
    t.elem = elem;
    t.dir = dir;
    t.str = (dir === 2 ? "<-chan " : dir === 1 ? "chan<- " : "chan ") + elem.str;
    return t;
  });
}

export function funcOf(params: Type[], results: Type[], variadic: boolean): Type {
  const key = `func(${params.map((p) => p.id).join(",")}${variadic ? "..." : ""})(${results.map((r) => r.id).join(",")})`;
  return memoized(key, () => {
    const t = new Type();
    t.kind = Kind.Func;
    t.params = params;
    t.results = results;
    t.variadic = variadic;
    const ps = params.map((p, i) => (variadic && i === params.length - 1 ? "..." + p.elem!.str : p.str));
    const rs = results.length === 0 ? "" : results.length === 1 ? " " + results[0].str : ` (${results.map((r) => r.str).join(", ")})`;
    t.str = `func(${ps.join(", ")})${rs}`;
    return t;
  });
}

export function interfaceOf(methods: IMethod[]): Type {
  const sorted = [...methods].sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  const key = `interface{${sorted.map((m) => `${m.pkgPath}.${m.name}:${m.type.id}`).join(";")}}`;
  return memoized(key, () => {
    const t = new Type();
    t.kind = Kind.Interface;
    t.imethods = sorted;
    t.str = sorted.length === 0 ? "interface {}" : `interface { ${sorted.map((m) => m.name + m.type.str.slice(4)).join("; ")} }`;
    return t;
  });
}

// A field as generated code spells it: [name, type, embedded, tag, prop],
// with the last three left out when they are false, "" and name, and the
// package path that of the struct's unexported fields.
// A FieldSpec is a field as [name, type, flags, tag, prop]: flags has 1
// for an embedded field and 2 for an exported one whose name does not
// start with A to Z (the name holds its UTF-8 bytes).
type FieldSpec = [string, Type, number?, string?, string?];

// structOf describes an unnamed struct type. ctor is the class generated for
// it; identical struct types from different packages share one descriptor.
export function structOf(specs: (Field | FieldSpec)[], ctor: any, pkgPath = ""): Type {
  const fields = specs.map((f): Field => {
    if (!Array.isArray(f)) return f;
    const [name, type, flags = 0, tag = "", prop = name] = f;
    const c = name.charCodeAt(0);
    const exported = (c >= 0x41 && c <= 0x5a) || (flags & 2) !== 0;
    return { name, pkgPath: exported ? "" : pkgPath, type, embedded: (flags & 1) !== 0, tag, prop };
  });
  const key = `struct{${fields.map((f) => `${f.embedded ? "~" : ""}${f.pkgPath}.${f.name}:${f.type.id}:${JSON.stringify(f.tag)}`).join(";")}}`;
  return memoized(key, () => {
    const t = new Type();
    t.kind = Kind.Struct;
    t.fields = fields;
    t.ctor = ctor;
    registerCtor(ctor, t);
    t.str = `struct { ${fields.map((f) => (f.embedded ? f.type.str : `${f.name} ${f.type.str}`)).join("; ")} }`;
    t.zero = structZero(t);
    return t;
  });
}

// The struct type of the objects a class makes, for unsafe pointer
// arithmetic (runtime/src/unsafe.ts), which needs their layout. Types that
// share a class (a named type and its underlying struct) have one layout; a
// class whose types differ in layout (instances of a generic type) has none.
export const ctorTypes = new WeakMap<object, Type | null>();

// The defined type of the objects a class makes, when one type has the
// class (instances of a generic type share theirs): the type of a Go
// object JavaScript passes back to Go (runtime/src/jsabi.ts).
export const classTypes = new WeakMap<object, Type | null>();

function registerCtor(ctor: any, t: Type): void {
  if (ctor === null || ctor === undefined) return;
  const old = ctorTypes.get(ctor);
  if (old === undefined) ctorTypes.set(ctor, t);
  else if (old !== null && old !== t && !sameFields(old, t)) ctorTypes.set(ctor, null);
}

function sameFields(a: Type, b: Type): boolean {
  return a.fields.length === b.fields.length && a.fields.every((f, i) => f.type === b.fields[i].type);
}

// structZero returns t's zero function: a clone of a zero value made once,
// through the class's own monomorphic $clone, instead of zeroStruct's
// spread of an array into the constructor each time. The first call makes
// the value, when the field types are complete. The classes of the struct
// types a program never copies have no $clone (internal/lower/nocopy.go).
function structZero(t: Type): () => any {
  let z: any = null;
  return () => {
    z ??= zeroStruct(t);
    return z.$clone === undefined ? zeroStruct(t) : z.$clone(t);
  };
}

// zeroStruct passes the zero fields to the constructor, which takes them in
// order: setting them after the constructor left them undefined would make
// V8 keep the class's fields as tagged values (boxing every float64).
function zeroStruct(t: Type): any {
  const fs = t.fields, a = new Array(fs.length);
  for (let i = 0; i < fs.length; i++) a[i] = fs[i].type.zero();
  return new t.ctor(...a);
}

// named creates the descriptor of a defined (named) type. The underlying type
// is attached later with setUnderlying so that recursive types work.
// named creates a defined type. The first nOuter type arguments are those
// of the generic function declaring a local type, which gc writes before a
// semicolon: S[int,string;bool].
//
// An instance of a local type of a function carries gc's disambiguation
// number gen (the local type's index in the package) in its string, as in
// T[main.U[int]·3], except as the type itself: reflect's String and Name
// leave it out (topString). Strings hold UTF-8 bytes: "\xc2\xb7" is "·".
export function named(pkgPath: string, name: string, typeArgs: Type[] = [], pkgName?: string, nOuter = 0, gen = 0): Type {
  const t = new Type();
  t.named = true;
  t.pkgPath = pkgPath;
  t.name = name;
  t.typeArgs = typeArgs;
  t.nOuter = nOuter;
  const short = pkgPath === "" ? "" : (pkgName ?? pkgPath.slice(pkgPath.lastIndexOf("/") + 1)) + ".";
  t.str = short + typeArgsName(t) + (gen && typeArgs.length ? "\xc2\xb7" + gen : "");
  return t;
}

const pendingTypes: Array<() => void> = [];

// defined is named(pkgPath, name) for a non-generic defined type whose
// underlying type and methods init sets up, at the next flushTypes: a
// module declares its types with pure calls, so that bundlers drop unused
// ones, and flushes them before using them.
export function defined(pkgPath: string, name: string, init: () => void, pkgName?: string): Type {
  pendingTypes.push(init);
  return named(pkgPath, name, [], pkgName);
}

export function flushTypes(): void {
  for (let i = 0; i < pendingTypes.length; i++) pendingTypes[i]();
  pendingTypes.length = 0;
}

// topString is t's string as reflect reports it for t itself.
export function topString(t: Type): string {
  return t.named ? t.str.replace(/\xc2\xb7\d+$/, "") : t.str;
}

// typeArgsName is t's name with its type arguments.
export function typeArgsName(t: Type): string {
  const a = t.typeArgs;
  if (!a.length) return t.name;
  const outer = a.slice(0, t.nOuter).map((x) => x.str).join(",");
  const own = a.slice(t.nOuter).map((x) => x.str).join(",");
  return `${t.name}[${t.nOuter === 0 ? own : own === "" ? outer : outer + ";" + own}]`;
}

export function setUnderlying(t: Type, u: Type, ctor?: any): void {
  t.kind = u.kind;
  t.elem = u.elem;
  t.key = u.key;
  t.len = u.len;
  t.dir = u.dir;
  t.fields = u.fields;
  t.imethods = u.imethods;
  t.params = u.params;
  t.results = u.results;
  t.variadic = u.variadic;
  t.underlying = u.underlying;
  if (u.kind === Kind.Struct) {
    t.ctor = ctor ?? u.ctor;
    registerCtor(t.ctor, t);
    if (t.ctor !== null && t.ctor !== undefined) {
      const old = classTypes.get(t.ctor);
      classTypes.set(t.ctor, old === undefined || old === t ? t : null);
    }
    t.zero = structZero(t);
  } else {
    t.zero = u.zero;
  }
}

// addMethods registers the method set of t. Keys are method names, qualified
// with the package path for unexported methods.
// addMethods registers the methods of t that can be called dynamically
// (through an interface, a type parameter or reflection): goesm leaves the
// others out of the table, so that bundlers drop the unused ones. methods is
// empty for a type all of whose methods were left out.
export function addMethods(t: Type, methods: Record<string, [(recv: any, ...args: any[]) => any, Type]>): void {
  if (t.mt === noMethods) t.mt = Object.create({});
  for (const k of Object.keys(methods)) {
    const [fn, type] = methods[k];
    t.methods.set(k, { fn, type });
    Object.getPrototypeOf(t.mt)[k] = fn;
  }
}

// hasMethods reports whether t has methods in Go, including those its
// method table leaves out.
export function hasMethods(t: Type): boolean {
  return t.mt !== noMethods;
}

// withMethods registers methods promoted into an unnamed struct type (or a
// pointer to one) and returns the descriptor.
export function withMethods(t: Type, methods: Record<string, [(recv: any, ...args: any[]) => any, Type]>): Type {
  addMethods(t, methods);
  return t;
}

// generic memoizes instantiations of a generic named type by the identity of
// its type arguments. The instance is cached before init runs so recursive
// references (type List[T] struct{ next *List[T] }) resolve to itself.
export function generic(
  pkgPath: string,
  name: string,
  init: (t: Type, ...targs: Type[]) => void,
  pkgName?: string,
  nOuter = 0,
  gen = 0,
): (...targs: Type[]) => Type {
  const cache = new Map<string, Type>();
  const byArg = new Map<Type, Type>(); // the instantiations with one type argument
  const inst = (targs: Type[]): Type => {
    if (targs.length === 1) {
      const t = byArg.get(targs[0]);
      if (t !== undefined) return t;
    }
    const key = targs.map((t) => t.id).join(",");
    let t = cache.get(key);
    if (!t) {
      t = named(pkgPath, name, targs, pkgName, nOuter, gen);
      cache.set(key, t);
      init(t, ...targs);
    }
    if (targs.length === 1) byArg.set(targs[0], t);
    return t;
  };
  if (init.length !== 2) return (...targs: Type[]) => inst(targs);
  // Generic code looks its types up on every call: with one type argument
  // (init is (t, targ) => ...) the lookup allocates nothing, and the last
  // one is remembered.
  let lastArg: Type | undefined;
  let last: Type | undefined;
  return ((t0: Type): Type => {
    if (t0 === lastArg) return last!;
    const t = byArg.get(t0) ?? inst([t0]);
    lastArg = t0;
    last = t;
    return t;
  }) as (...targs: Type[]) => Type;
}

export function methodKey(name: string, pkgPath: string): string {
  return pkgPath === "" ? name : `${pkgPath}.${name}`;
}

// implementsIface reports whether type t (a dynamic type) implements interface
// type iface, comparing method names and signature identity.
export function implementsIface(t: Type, iface: Type): boolean {
  for (const m of iface.imethods) {
    const impl = t.methods.get(methodKey(m.name, m.pkgPath));
    if (!impl || impl.type !== m.type) return false;
  }
  return true;
}

export function isAggregate(t: Type): boolean {
  return t.kind === Kind.Struct || t.kind === Kind.Array;
}

// The predeclared error interface.
export const errorType: Type = /* @__PURE__ */ (() => {
  const t = named("", "error");
  setUnderlying(t, interfaceOf([{ name: "Error", pkgPath: "", type: funcOf([], [tString], false) }]));
  return t;
})();

// sizeOf and alignOf are unsafe.Sizeof / unsafe.Alignof under GOARCH=wasm
// (go/types' sizes for the target), for operands whose type is a type
// parameter; other operands are constants folded by go/types.
// zeroSized is sizeOf(t) === 0, without computing the size: it stops at
// the first field or element that has one.
export function zeroSized(t: Type): boolean {
  switch (t.kind) {
    case Kind.Array:
      return t.len === 0 || zeroSized(t.elem!);
    case Kind.Struct:
      if (t.zeroSized === 0) {
        t.zeroSized = 2;
        for (const f of t.fields) {
          if (!zeroSized(f.type)) {
            t.zeroSized = 1;
            break;
          }
        }
      }
      return t.zeroSized === 2;
  }
  return false;
}

export function sizeOf(t: Type): number {
  switch (t.kind) {
    case Kind.Bool: case Kind.Int8: case Kind.Uint8: return 1;
    case Kind.Int16: case Kind.Uint16: return 2;
    case Kind.Int32: case Kind.Uint32: case Kind.Float32: return 4;
    case Kind.String: case Kind.Interface: case Kind.Complex128: return 16;
    case Kind.Slice: return 24;
    case Kind.Array: return t.len * sizeOf(t.elem!);
    case Kind.Struct: {
      let off = 0, max = 1;
      for (const f of t.fields) {
        const a = alignOf(f.type);
        max = Math.max(max, a);
        off = Math.ceil(off / a) * a + sizeOf(f.type);
      }
      return Math.ceil(off / max) * max;
    }
  }
  return 8;
}

export function alignOf(t: Type): number {
  switch (t.kind) {
    case Kind.Array: return alignOf(t.elem!);
    case Kind.Struct: return t.fields.reduce((m, f) => Math.max(m, alignOf(f.type)), 1);
    case Kind.String: case Kind.Interface: case Kind.Slice: return 8;
    case Kind.Complex64: return 4;
    case Kind.Complex128: return 8;
  }
  return Math.min(sizeOf(t), 8);
}
