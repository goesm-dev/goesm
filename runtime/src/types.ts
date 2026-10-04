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
export const types = {
  bool: basic(Kind.Bool, "bool", () => false),
  int: basic(Kind.Int, "int", zeroNum),
  int8: basic(Kind.Int8, "int8", zeroNum),
  int16: basic(Kind.Int16, "int16", zeroNum),
  int32: basic(Kind.Int32, "int32", zeroNum),
  int64: basic(Kind.Int64, "int64", zeroBig),
  uint: basic(Kind.Uint, "uint", zeroNum),
  uint8: basic(Kind.Uint8, "uint8", zeroByte),
  uint16: basic(Kind.Uint16, "uint16", zeroNum),
  uint32: basic(Kind.Uint32, "uint32", zeroNum),
  uint64: basic(Kind.Uint64, "uint64", zeroBig),
  uintptr: basic(Kind.Uintptr, "uintptr", zeroNum),
  float32: basic(Kind.Float32, "float32", zeroNum),
  float64: basic(Kind.Float64, "float64", zeroNum),
  complex64: basic(Kind.Complex64, "complex64", () => complexZero),
  complex128: basic(Kind.Complex128, "complex128", () => complexZero),
  string: basic(Kind.String, "string", () => ""),
  unsafePointer: basic(Kind.UnsafePointer, "unsafe.Pointer", () => null),
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
  return memoized(`*${elem.id}`, () => {
    const t = new Type();
    t.kind = Kind.Pointer;
    t.elem = elem;
    t.str = `*${elem.str}`;
    return t;
  });
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

// structOf describes an unnamed struct type. ctor is the class generated for
// it; identical struct types from different packages share one descriptor.
export function structOf(fields: Field[], ctor: any): Type {
  const key = `struct{${fields.map((f) => `${f.embedded ? "~" : ""}${f.pkgPath}.${f.name}:${f.type.id}:${JSON.stringify(f.tag)}`).join(";")}}`;
  return memoized(key, () => {
    const t = new Type();
    t.kind = Kind.Struct;
    t.fields = fields;
    t.ctor = ctor;
    registerCtor(ctor, t);
    t.str = `struct { ${fields.map((f) => (f.embedded ? f.type.str : `${f.name} ${f.type.str}`)).join("; ")} }`;
    t.zero = () => zeroStruct(t);
    return t;
  });
}

// The struct type of the objects a class makes, for unsafe pointer
// arithmetic (runtime/src/unsafe.ts), which needs their layout. Types that
// share a class (a named type and its underlying struct) have one layout; a
// class whose types differ in layout (instances of a generic type) has none.
export const ctorTypes = new WeakMap<object, Type | null>();

function registerCtor(ctor: any, t: Type): void {
  if (ctor === null || ctor === undefined) return;
  const old = ctorTypes.get(ctor);
  if (old === undefined) ctorTypes.set(ctor, t);
  else if (old !== null && old !== t && !sameFields(old, t)) ctorTypes.set(ctor, null);
}

function sameFields(a: Type, b: Type): boolean {
  return a.fields.length === b.fields.length && a.fields.every((f, i) => f.type === b.fields[i].type);
}

function zeroStruct(t: Type): any {
  const o = new t.ctor();
  for (const f of t.fields) o[f.prop] = f.type.zero();
  return o;
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
    t.zero = () => zeroStruct(t);
  } else {
    t.zero = u.zero;
  }
}

// addMethods registers the method set of t. Keys are method names, qualified
// with the package path for unexported methods.
export function addMethods(t: Type, methods: Record<string, [(recv: any, ...args: any[]) => any, Type]>): void {
  for (const k of Object.keys(methods)) {
    const [fn, type] = methods[k];
    t.methods.set(k, { fn, type });
  }
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
  return (...targs: Type[]) => {
    const key = targs.map((t) => t.id).join(",");
    let t = cache.get(key);
    if (!t) {
      t = named(pkgPath, name, targs, pkgName, nOuter, gen);
      cache.set(key, t);
      init(t, ...targs);
    }
    return t;
  };
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
export const errorType: Type = named("", "error");
setUnderlying(errorType, interfaceOf([{ name: "Error", pkgPath: "", type: funcOf([], [types.string], false) }]));

// sizeOf and alignOf are unsafe.Sizeof / unsafe.Alignof under GOARCH=wasm
// (go/types' sizes for the target), for operands whose type is a type
// parameter; other operands are constants folded by go/types.
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
