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
import { arrayElemPtr, indexPtrTarget } from "./ptr.ts";
import { elemOrigins, Slice } from "./slice.ts";
import { bytesToString, stringToBytes } from "./string.ts";
import { isAggregate, Kind, type Type } from "./types.ts";

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
    if (typeof p === "object" && "v" in p) return new Slice(cellView(p), 0, 1, 1);
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
  return o;
}

// reinterpret is (*To)(unsafe.Pointer(p)) for p of type *From, with From and
// To the layouts lower.reinterpretable accepts.
export function reinterpret(p: any, from: Type, to: Type): any {
  if (p === null || from === to) return p;
  if (from.kind === Kind.Struct && to.kind === Kind.Struct) {
    const fw = words(from), tw = words(to);
    if (fw.every((w, i) => w.prop === tw[i].prop)) return p;
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
