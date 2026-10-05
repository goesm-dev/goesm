// unsafe.String, unsafe.Slice, unsafe.StringData and reinterpretation
// through unsafe.Pointer.
//
// goesm has no address space, so a pointer cannot be turned into memory of
// another type. What it can follow is a pointer's provenance: where the
// pointer came from. unsafe.StringData returns a pointer that remembers its
// string, unsafe.SliceData one that remembers its backing array, and
// unsafe.String / unsafe.Slice rebuild the string or slice from them. Header
// structs that mirror the layout of a string, slice or interface (as
// libraries like protobuf declare them) become views of the value they were
// reinterpreted from.

import { Iface } from "./iface.ts";
import { runtimePanic } from "./panic.ts";
import { arrayElemPtr, fieldPtr, fieldPtrTarget, indexPtrTarget } from "./ptr.ts";
import { elemOrigins, Slice } from "./slice.ts";
import { bytesToString, stringToBytes } from "./string.ts";
import { alignOf, ctorTypes, isAggregate, Kind, sizeOf, type Type } from "./types.ts";

// StringDataPtr is unsafe.StringData(s): a *byte to s[i].
class StringDataPtr {
  readonly s: string;
  readonly i: number;
  constructor(s: string, i: number) {
    this.s = s;
    this.i = i;
  }
  get v(): number { return this.s.charCodeAt(this.i); }
  set v(_: number) { runtimePanic("goesm: write through unsafe.StringData (strings are immutable)"); }
}

const stringPtrs = new Map<string, StringDataPtr>();

// stringData is unsafe.StringData(s). The same string gives the same pointer
// while it is cached, so pointers to equal strings compare equal like the
// data pointers of a string and its copies do in Go.
export function stringData(s: string): any {
  if (s.length === 0) return null;
  let p = stringPtrs.get(s);
  if (p === undefined) {
    if (stringPtrs.size > 4096) stringPtrs.clear();
    p = new StringDataPtr(s, 0);
    stringPtrs.set(s, p);
  }
  return p;
}

function lenOK(n: number, what: string): void {
  if (n < 0 || !Number.isInteger(n)) runtimePanic(`unsafe.${what}: len out of range`);
}

// unsafeStringFrom is unsafe.String(p, n) for a pointer p of any provenance.
export function unsafeStringFrom(p: any, n: number): string {
  lenOK(n, "String");
  if (n === 0) return "";
  if (p === null) runtimePanic("unsafe.String: ptr is nil and len is not zero");
  if (p instanceof StringDataPtr) {
    if (p.i + n > p.s.length) runtimePanic("goesm: unsafe.String beyond the string unsafe.StringData came from");
    return p.s.substring(p.i, p.i + n);
  }
  return bytesToString(unsafeSliceFrom(p, n));
}

// unsafeSliceFrom is unsafe.Slice(p, n) for a pointer p of any provenance.
export function unsafeSliceFrom(p: any, n: number): any {
  lenOK(n, "Slice");
  if (p === null) {
    if (n !== 0) runtimePanic("unsafe.Slice: ptr is nil and len is not zero");
    return null;
  }
  if (p instanceof StringDataPtr) {
    // The bytes of a string are immutable, so a copy is indistinguishable
    // from an alias.
    if (p.i + n > p.s.length) runtimePanic("goesm: unsafe.Slice beyond the string unsafe.StringData came from");
    return stringToBytes(p.s.substring(p.i, p.i + n));
  }
  const at = indexPtrTarget(p) ?? (typeof p === "object" ? elemOrigins.get(p) : undefined);
  if (at !== undefined) {
    if (at.i + n > at.a.length) runtimePanic("unsafe.Slice: len out of range (beyond the underlying array)");
    return new Slice(at.a, at.i, n, n);
  }
  if (n === 1) {
    // A pointer to a single variable: the slice aliases it through a view.
    if (typeof p === "object" && "v" in p && !isAggregateObject(p)) return new Slice(cellView(p), 0, 1, 1);
    return new Slice([p], 0, 1, 1); // a pointer to an aggregate is the object itself
  }
  runtimePanic("goesm: unsafe.Slice of a pointer that does not point into an array or string (goesm has no address space)");
}

function cellView(p: { v: any }): any[] {
  return new Proxy([undefined] as any[], {
    get: (t, k) => (k === "0" ? p.v : Reflect.get(t, k)),
    set: (t, k, v) => {
      if (k === "0") p.v = v;
      else Reflect.set(t, k, v);
      return true;
    },
  });
}

function pointerShaped(t: Type): boolean {
  switch (t.kind) {
    case Kind.Pointer: case Kind.UnsafePointer: case Kind.Map: case Kind.Chan: case Kind.Func:
      return true;
  }
  return false;
}

// The non-zero-size fields of a struct, which carry its layout.
function words(t: Type): { prop: string; type: Type }[] {
  return t.fields.filter((f) => !zeroSize(f.type)).map((f) => ({ prop: f.prop, type: f.type }));
}

function zeroSize(t: Type): boolean {
  switch (t.kind) {
    case Kind.Array: return t.len === 0 || zeroSize(t.elem!);
    case Kind.Struct: return t.fields.every((f) => zeroSize(f.type));
  }
  return false;
}

// ifaceData is the data word of an interface value: the value itself when it
// is pointer-shaped or an aggregate (whose pointer is the object), else a
// pointer to the boxed value.
const boxes = new WeakMap<Iface, any>();
function ifaceData(i: Iface | null): any {
  if (i === null) return null;
  if (pointerShaped(i.t) || isAggregate(i.t)) return i.v;
  let b = boxes.get(i);
  if (b === undefined) {
    b = { v: i.v };
    boxes.set(i, b);
  }
  return b;
}

function fromData(t: Type, d: any): any {
  if (pointerShaped(t) || isAggregate(t)) return d;
  return d === null ? t.zero() : d.v;
}

// view is an object of To's struct class whose fields are accessors.
function view(to: Type, accessors: Record<string, PropertyDescriptor>): any {
  const o = Object.create(to.ctor.prototype);
  for (const f of to.fields) {
    if (!(f.prop in accessors)) {
      const z = f.type.zero();
      accessors[f.prop] = { get: () => z, set: () => {} };
    }
  }
  for (const k of Object.keys(accessors)) Object.defineProperty(o, k, { ...accessors[k], enumerable: true });
  if (to.ifaceSelf) o.v = o; // what the constructor sets (see Iface)
  return o;
}

// reinterpret is (*To)(unsafe.Pointer(p)) for p of type *From, with From and
// To the layouts lower.reinterpretable accepts.
export function reinterpret(p: any, from: Type, to: Type): any {
  if (p === null || from === to) return p;
  if (from.kind === Kind.Struct && to.kind === Kind.Struct) {
    const fw = words(from), tw = words(to);
    // The object serves as is unless its class would give it From as its
    // interface type where To's values are their own interface values.
    if (fw.every((w, i) => w.prop === tw[i].prop) && (!to.ifaceSelf || from.ctor === to.ctor)) return p;
    const acc: Record<string, PropertyDescriptor> = {};
    tw.forEach((w, i) => {
      const k = fw[i].prop;
      acc[w.prop] = { get: () => p[k], set: (x: any) => { p[k] = x; } };
    });
    return view(to, acc);
  }
  if (from.kind === Kind.Interface) {
    // p points to an interface variable: {type, data}.
    if (to.kind === Kind.Array) {
      return new Proxy([null, null], {
        get: (t, k) => (k === "0" ? p.v?.t ?? null : k === "1" ? ifaceData(p.v) : Reflect.get(t, k)),
      });
    }
    const [tw, dw] = words(to);
    return view(to, {
      [tw.prop]: {
        get: () => (p.v === null ? null : p.v.t),
        set: (t: Type | null) => { p.v = t === null ? null : new Iface(t, p.v === null ? t.zero() : p.v.v); },
      },
      [dw.prop]: {
        get: () => ifaceData(p.v),
        set: (d: any) => { if (p.v !== null) p.v = new Iface(p.v.t, fromData(p.v.t, d)); },
      },
    });
  }
  if (from.kind === Kind.Slice && to.kind === Kind.String) {
    // *(*string)(unsafe.Pointer(&b)) of a []byte b, the zero-copy idiom of
    // older code: each read takes the bytes as they are then.
    return { get v() { return bytesToString(p.v); }, set v(x: string) { p.v = stringToBytes(x); } };
  }
  if (from.kind === Kind.String && to.kind === Kind.Slice) {
    // *(*[]byte)(unsafe.Pointer(&s)): Go forbids writing to the bytes, so a
    // copy cannot be told apart.
    return { get v() { return stringToBytes(p.v); }, set v(_: any) { readOnly(); } };
  }
  if (from.kind === Kind.String) {
    // p points to a string variable: {data, len}.
    const [dw, lw] = words(to);
    let data: any = null;
    return view(to, {
      [dw.prop]: { get: () => stringData(p.v), set: (d: any) => { data = d; } },
      [lw.prop]: { get: () => p.v.length, set: (n: number) => { p.v = unsafeStringFrom(data, Number(n)); } },
    });
  }
  if (from.kind === Kind.Slice) {
    // p points to a slice variable: {data, len, cap}.
    const [dw, lw, cw] = words(to);
    const len = () => (p.v === null ? 0 : p.v.$length);
    const cap = () => (p.v === null ? 0 : p.v.$capacity);
    return view(to, {
      [dw.prop]: { get: () => (p.v === null || cap() === 0 ? null : dataOf(p.v, from.elem!)), set: () => { readOnly(); } },
      [lw.prop]: { get: len, set: () => { readOnly(); } },
      [cw.prop]: { get: cap, set: () => { readOnly(); } },
    });
  }
  if (from.kind === Kind.Struct && (to.kind === Kind.Slice || to.kind === Kind.String)) {
    // A header struct read as the string or slice it describes.
    const [dw, lw] = words(from);
    const get = (): any => {
      const d = p[dw.prop], n = Number(p[lw.prop]);
      if (dw.type.kind === Kind.String) {
        const s = (d as string).substring(0, n);
        return to.kind === Kind.String ? s : stringToBytes(s);
      }
      return to.kind === Kind.String ? unsafeStringFrom(d, n) : unsafeSliceFrom(d, n);
    };
    return { get v() { return get(); }, set v(_: any) { readOnly(); } };
  }
  runtimePanic(`goesm: reinterpreting ${from.str} as ${to.str} through unsafe.Pointer is not supported`);
}

function dataOf(s: Slice<any>, elem: Type): any {
  if (isAggregate(elem)) {
    const e = s.$array[s.$offset];
    elemOrigins.set(e, { a: s.$array, i: s.$offset });
    return e;
  }
  return arrayElemPtr(s.$array, s.$offset);
}

function readOnly(): never {
  runtimePanic("goesm: writing a slice header through unsafe.Pointer is not supported");
}

// ---- Pointer arithmetic ----
//
// unsafe.Pointer(uintptr(p) + off) and unsafe.Add(p, off), as libraries such
// as protobuf use them to reach struct fields by their offsets: a pointer
// into the middle of an aggregate is the aggregate and a byte offset (an
// Addr), laid out as go/types lays out GOARCH=wasm (the offsets
// reflect.StructField reports). Converting it to a *T (ptrAt) finds the
// variable of type T at that offset: a field or element pointer, or the
// nested struct or array itself. A struct or array reached this way
// remembers where it lies in its parent, so that offsets beyond it, and a
// pointer to its parent at offset 0, are found again.

class Addr {
  readonly base: object;
  readonly off: number;
  constructor(base: object, off: number) {
    this.base = base;
    this.off = off;
  }
}

const addrs = new WeakMap<object, Map<number, Addr>>();

// addr returns the canonical pointer to off bytes into base, so that equal
// addresses are equal pointers.
function addr(base: object, off: number): any {
  if (off === 0) return base;
  let m = addrs.get(base);
  if (m === undefined) addrs.set(base, (m = new Map()));
  let a = m.get(off);
  if (a === undefined) m.set(off, (a = new Addr(base, off)));
  return a;
}

const parents = new WeakMap<object, { o: object; off: number }>();

function setParent(child: any, o: object, off: number): void {
  if (typeof child === "object" && child !== null && !parents.has(child)) parents.set(child, { o, off });
}

function structType(o: any): Type | null | undefined {
  if (typeof o !== "object" || o === null || Array.isArray(o)) return undefined;
  return ctorTypes.get(Object.getPrototypeOf(o)?.constructor);
}

function isAggregateObject(o: any): boolean {
  return Array.isArray(o) || structType(o) !== undefined;
}

interface Slot {
  prop: string;
  type: Type;
  off: number;
  size: number;
}

const layouts = new WeakMap<Type, Slot[]>();

// layout returns the non-zero-size fields of struct type t with their
// offsets, as reflect.StructField.Offset gives them.
function layout(t: Type): Slot[] {
  let l = layouts.get(t);
  if (l === undefined) {
    l = [];
    let off = 0;
    for (const f of t.fields) {
      const a = alignOf(f.type);
      off = Math.ceil(off / a) * a;
      const size = sizeOf(f.type);
      if (size > 0) l.push({ prop: f.prop, type: f.type, off, size });
      off += size;
    }
    layouts.set(t, l);
  }
  return l;
}

function badArith(): never {
  runtimePanic("goesm: unsafe pointer arithmetic outside the struct or array the pointer points into");
}

// locate returns the outermost known aggregate p points into and the offset.
function locate(p: any): { base: any; off: number } | undefined {
  let base: any, off: number;
  if (p instanceof Addr) {
    base = p.base;
    off = p.off;
  } else if (isAggregateObject(p)) {
    base = p;
    off = 0;
  } else {
    const f = fieldPtrTarget(p);
    const t = f && structType(f.o);
    const slot = t ? layout(t).find((s) => s.prop === f!.k) : undefined;
    if (slot === undefined) return undefined;
    base = f!.o;
    off = slot.off;
  }
  for (let par = parents.get(base); par !== undefined; par = parents.get(base)) {
    off += par.off;
    base = par.o;
  }
  return { base, off };
}

// ptrAdd is unsafe.Pointer(uintptr(p) + d) and unsafe.Add(p, d).
export function ptrAdd(p: any, d: number): any {
  d = Number(d);
  if (d === 0) return p;
  if (p === null) runtimePanic("goesm: unsafe pointer arithmetic on nil");
  if (p instanceof StringDataPtr) return new StringDataPtr(p.s, p.i + d);
  const at = locate(p);
  if (at === undefined) runtimePanic("goesm: unsafe pointer arithmetic on a pointer that does not point into a struct or array (goesm has no address space)");
  if (at.off + d < 0) badArith();
  return addr(at.base, at.off + d);
}

// unsafePtrEq is p == q for unsafe.Pointers, which can be the same address
// as different objects: &x.f and the pointer to f's offset into x.
export function unsafePtrEq(p: any, q: any): boolean {
  if (p === q) return true;
  if (p === null || q === null || typeof p !== "object" || typeof q !== "object") return false;
  if (!(p instanceof Addr) && !(q instanceof Addr)) return false;
  const a = innermost(p), b = innermost(q);
  return a !== undefined && b !== undefined && a.base === b.base && a.off === b.off;
}

// innermost locates p in the innermost struct or array that holds it.
function innermost(p: any): { base: any; off: number } | undefined {
  const at = locate(p);
  if (at === undefined) return undefined;
  let { base, off } = at;
  for (;;) {
    if (Array.isArray(base)) {
      const e0 = base[0];
      if (Array.isArray(e0) || !isAggregateObject(e0)) break;
      const size = sizeOf(structType(e0)!);
      if (size === 0) break;
      const i = Math.floor(off / size);
      if (i >= base.length) break;
      setParent(base[i], base, i * size);
      base = base[i];
      off -= i * size;
      continue;
    }
    const st = structType(base);
    const slot = st ? layout(st).find((s) => off >= s.off && off < s.off + s.size) : undefined;
    if (slot === undefined || !isAggregate(slot.type)) break;
    setParent(base[slot.prop], base, slot.off);
    base = base[slot.prop];
    off -= slot.off;
  }
  return { base, off };
}

function layoutMatches(o: any, t: Type): boolean {
  if (Array.isArray(o)) return t.kind === Kind.Array;
  const st = structType(o);
  if (st === t) return true;
  if (st === undefined || st === null || t.kind !== Kind.Struct) return false;
  const a = layout(st), b = layout(t);
  return a.length === b.length && a.every((s, i) => s.off === b[i].off && (s.type === b[i].type || (pointerShaped(s.type) && pointerShaped(b[i].type))));
}

// ptrAt is (*T)(p) for an unsafe.Pointer p.
export function ptrAt(p: any, t: Type): any {
  if (p === null || typeof p !== "object") return p;
  if (!(p instanceof Addr)) {
    if (isAggregateObject(p)) {
      if (isAggregate(t) && layoutMatches(p, t)) return p;
    } else if (t.kind !== Kind.Slice || fieldPtrTarget(p) === undefined) {
      return p; // a pointer to a variable: as is
    }
  }
  const at = locate(p)!;
  return resolve(at.base, at.off, t, p);
}

function resolve(base: any, off: number, t: Type, orig: any): any {
  for (;;) {
    if (off === 0 && isAggregate(t) && layoutMatches(base, t)) return base;
    if (Array.isArray(base)) {
      const e0 = base[0];
      const nested = isAggregateObject(e0);
      const size = nested ? (Array.isArray(e0) ? 0 : sizeOf(structType(e0)!)) : sizeOf(t);
      if (size === 0) break;
      const i = Math.floor(off / size), sub = off - i * size;
      if (i >= base.length) badArith();
      if (!nested) {
        if (sub !== 0) break;
        return arrayElemPtr(base, i);
      }
      setParent(base[i], base, i * size);
      base = base[i];
      off = sub;
      continue;
    }
    const st = structType(base);
    if (!st) break;
    const slot = layout(st).find((s) => off >= s.off && off < s.off + s.size);
    if (slot === undefined) badArith();
    const sub = off - slot.off;
    if (isAggregate(slot.type)) {
      setParent(base[slot.prop], base, slot.off);
      base = base[slot.prop];
      off = sub;
      continue;
    }
    if (sub !== 0 || isAggregate(t)) break;
    const fp = fieldPtr(base, slot.prop);
    return wrapsPointers(t, slot.type) ? wrappedSlicePtr(fp, t.elem!) : fp;
  }
  if (!(orig instanceof Addr)) return orig; // an aggregate read as another type: unchanged
  runtimePanic(`goesm: unsafe.Pointer into the middle of a value read as ${t.str} (goesm has no address space)`);
}

// A struct whose only non-zero-size field is pointer-shaped has the layout of
// a pointer, but goesm represents it as an object: a slice of pointers read
// as a slice of such structs (protobuf's []pointer) is a view that wraps and
// unwraps the elements.
function wrapsPointers(t: Type, field: Type): boolean {
  if (t.kind !== Kind.Slice || field.kind !== Kind.Slice || t.elem!.kind !== Kind.Struct) return false;
  const w = words(t.elem!);
  return w.length === 1 && pointerShaped(w[0].type) && pointerShaped(field.elem!);
}

const wrappedArrays = new WeakMap<any[], any[]>();
const unwrapped = new WeakMap<any[], any[]>();

function wrappedSlicePtr(fp: { v: any }, w: Type): any {
  const prop = words(w)[0].prop;
  const wrapArray = (a: any[]): any[] => {
    let v = wrappedArrays.get(a);
    if (v === undefined) {
      v = new Proxy(a, {
        get: (t, k, r) => {
          if (typeof k === "string" && /^\d+$/.test(k)) {
            const o = w.zero();
            o[prop] = t[Number(k)] ?? null;
            return o;
          }
          return Reflect.get(t, k, r);
        },
        set: (t, k, x) => {
          if (typeof k === "string" && /^\d+$/.test(k)) t[Number(k)] = x === undefined ? null : x[prop];
          else Reflect.set(t, k, x);
          return true;
        },
      });
      wrappedArrays.set(a, v);
      unwrapped.set(v, a);
    }
    return v;
  };
  return {
    get v(): any {
      const s: Slice<any> | null = fp.v;
      return s === null ? null : new Slice(wrapArray(s.$array), s.$offset, s.$length, s.$capacity);
    },
    set v(s: Slice<any> | null) {
      if (s === null) {
        fp.v = null;
        return;
      }
      const a = unwrapped.get(s.$array) ?? s.$array.map((x: any) => (x === undefined || x === null ? null : x[prop]));
      fp.v = new Slice(a, s.$offset, s.$length, s.$capacity);
    },
  };
}
