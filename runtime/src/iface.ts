// Interface values, equality and hashing.
//
// A non-nil Go interface value is an Iface pairing the dynamic type
// descriptor with the dynamic value; a nil interface is JS null. Because the
// dynamic type travels with the value, `MyInt(1)` and `int(1)` stored in an
// `any` stay distinguishable, an interface holding a nil *T is not nil, and
// method dispatch goes through the type's method table instead of relying on
// JS structural typing.

import { Kind, Type, implementsIface, isAggregate, sizeOf, types } from "./types.ts";
import { ceq } from "./complex.ts";
import { GoPanic, runtimePanic, typeAssertionErrorType } from "./panic.ts";

export class Iface {
  t: Type;
  v: any;
  constructor(t: Type, v: any) {
    this.t = t;
    this.v = v;
  }
}

// box converts a value of static type t to an interface value. Aggregates must
// already be copied by the caller (the lowering emits the copy).
export function box(t: Type, v: any): Iface | null {
  if (t.kind === Kind.Interface) return v;
  return new Iface(t, v);
}

// copy returns a Go copy of v of type t. Used where the static type is a type
// parameter so the lowering cannot know whether v is an aggregate.
export function copy(t: Type, v: any): any {
  if (t.kind === Kind.Struct) return v.$clone(t);
  if (t.kind === Kind.Array) {
    const e = t.elem!;
    return isAggregate(e) ? v.map((x: any) => copy(e, x)) : v.slice();
  }
  return v;
}

export function icall(x: Iface | null, key: string, ...args: any[]): any {
  if (x === null) runtimePanic("invalid memory address or nil pointer dereference");
  const m = x.t.methods.get(key);
  if (!m) runtimePanic(`method ${key} not found on ${x.t.str}`);
  return m.fn(x.v, ...args);
}

export function typeIs(x: Iface | null, t: Type): boolean {
  if (x === null) return false;
  if (t.kind === Kind.Interface) return implementsIface(x.t, t);
  return x.t === t;
}

function assertionError(x: Iface | null, src: Type, t: Type): never {
  const have = x === null ? "nil" : x.t.str;
  let msg = `interface conversion: ${src.str === "interface {}" ? "interface {}" : src.str} is ${have}, not ${t.str}`;
  if (x !== null && t.kind === Kind.Interface) {
    const missing = t.imethods.find((m) => !x.t.methods.has(m.pkgPath ? `${m.pkgPath}.${m.name}` : m.name));
    msg = `interface conversion: ${x.t.str} is not ${t.str}: missing method ${missing ? missing.name : "?"}`;
  } else if (x !== null && x.t.str === t.str) {
    msg += x.t.pkgPath !== t.pkgPath ? " (types from different packages)" : " (types from different scopes)";
  }
  throw new GoPanic(new Iface(typeAssertionErrorType, msg));
}

export function assert(x: Iface | null, src: Type, t: Type): any {
  if (!typeIs(x, t)) assertionError(x, src, t);
  return t.kind === Kind.Interface ? x : copy(t, x!.v);
}

// unboxAs is the value of x, known to have type t, as a t: a type switch's
// variable in a case of type parameter type.
export function unboxAs(t: Type, x: Iface | null): any {
  return t.kind === Kind.Interface ? x : copy(t, x!.v);
}

export function assertOk(x: Iface | null, t: Type): [any, boolean] {
  if (!typeIs(x, t)) return [t.zero(), false];
  return [t.kind === Kind.Interface ? x : copy(t, x!.v), true];
}

function comparable(t: Type): boolean {
  switch (t.kind) {
    case Kind.Slice: case Kind.Map: case Kind.Func:
      return false;
    case Kind.Array:
      return comparable(t.elem!);
    case Kind.Struct:
      return t.fields.every((f) => comparable(f.type));
  }
  return true;
}

export function equal(t: Type, a: any, b: any): boolean {
  switch (t.kind) {
    case Kind.Struct:
      for (const f of t.fields) {
        if (f.name === "_") continue;
        if (!equal(f.type, a[f.prop], b[f.prop])) return false;
      }
      return true;
    case Kind.Array:
      for (let i = 0; i < t.len; i++) if (!equal(t.elem!, a[i], b[i])) return false;
      return true;
    case Kind.Interface:
      return ifaceEq(a, b);
    case Kind.Complex64: case Kind.Complex128:
      return ceq(a, b);
    case Kind.Pointer:
      // Like gc, pointers to zero-size values all share one address.
      if (sizeOf(t.elem!) === 0) return a === null ? b === null : b !== null;
      return a === b;
    case Kind.Slice: case Kind.Map: case Kind.Func:
      if (a !== null && b !== null) runtimePanic(`comparing uncomparable type ${t.str}`);
      return a === b;
  }
  return a === b;
}

export function ifaceEq(a: Iface | null, b: Iface | null): boolean {
  if (a === null || b === null) return a === b;
  if (a.t !== b.t) return false;
  if (!comparable(a.t)) runtimePanic(`comparing uncomparable type ${a.t.str}`);
  return equal(a.t, a.v, b.v);
}

// Map keys. Values whose JS representation already has Go equality under
// SameValueZero (strings, numbers, bools, pointers, channels) are used
// directly; NaN never equals itself so each NaN key is unique; composite keys
// are serialised, with objects (pointers) replaced by stable identities.
const objIDs = new WeakMap<object, number>();
let nextObjID = 1;
function objID(o: object | null): string {
  if (o === null) return "nil";
  let id = objIDs.get(o);
  if (id === undefined) {
    id = nextObjID++;
    objIDs.set(o, id);
  }
  return "#" + id;
}

export function hashKey(t: Type, v: any): any {
  switch (t.kind) {
    case Kind.Float32: case Kind.Float64:
      return v !== v ? Symbol("NaN") : v;
    case Kind.Complex64: case Kind.Complex128:
      return v.re !== v.re || v.im !== v.im ? Symbol("NaN") : "\u0000" + serialize(t, v);
    case Kind.Struct: case Kind.Array: case Kind.Interface:
      return "\u0000" + serialize(t, v);
    case Kind.Pointer:
      return v !== null && sizeOf(t.elem!) === 0 ? zerobaseKey : v;
    case Kind.Slice: case Kind.Map: case Kind.Func:
      runtimePanic(`hash of unhashable type ${t.str}`);
  }
  return v;
}

// ifaceKeyString returns a string that is equal for equal interface values
// (hash/maphash.Comparable); it panics for unhashable dynamic types.
export function ifaceKeyString(x: Iface | null): string {
  if (x === null) return "nil";
  if (!comparable(x.t)) runtimePanic(`hash of unhashable type ${x.t.str}`);
  return `<${x.t.id}>` + serialize(x.t, x.v);
}

const zerobaseKey = Symbol("zerobase");

function serialize(t: Type, v: any): string {
  switch (t.kind) {
    case Kind.Struct:
      return "{" + t.fields.filter((f) => f.name !== "_").map((f) => serialize(f.type, v[f.prop])).join(",") + "}";
    case Kind.Array:
      return "[" + (v as any[]).map((x) => serialize(t.elem!, x)).join(",") + "]";
    case Kind.Interface: {
      if (v === null) return "nil";
      const x = v as Iface;
      if (!comparable(x.t)) runtimePanic(`hash of unhashable type ${x.t.str}`);
      return `<${x.t.id}>` + serialize(x.t, x.v);
    }
    case Kind.String:
      return JSON.stringify(v);
    case Kind.Float32: case Kind.Float64:
      if (v !== v) return "NaN#" + nextObjID++; // NaN != NaN: every such key is distinct
      return String(v === 0 ? 0 : v);
    case Kind.Complex64: case Kind.Complex128: {
      const f = t.kind === Kind.Complex64 ? types.float32 : types.float64;
      return "(" + serialize(f, v.re) + "," + serialize(f, v.im) + ")";
    }
    case Kind.Pointer:
      return v !== null && sizeOf(t.elem!) === 0 ? "zerobase" : objID(v);
    case Kind.Chan: case Kind.UnsafePointer:
      return objID(v);
  }
  return String(v);
}
