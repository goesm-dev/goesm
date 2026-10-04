// Pointers.
//
// Representation (chosen by the lowering from the pointee's static type):
//   *Struct, *Array  -> the struct object / JS array itself. Struct and array
//                       values are mutable JS objects that are copied on Go
//                       value copies, so the object identity *is* the address.
//   anything else    -> an object with a `v` accessor: a Cell for variables
//                       whose address is taken, or a FieldPtr / IndexPtr for
//                       &s.f / &a[i]. `*p` is `p.v`, `*p = x` is `p.v = x`.
//
// Pointer identity is preserved (&x == &x, &s.f == &s.f) by caching the
// derived pointer objects. A pointer is not an integer here; unsafe.Pointer
// arithmetic would need the planned ArrayBuffer-backed memory model.

import { plainPanic, runtimePanic } from "./panic.ts";
import { Slice } from "./slice.ts";
import { Type, isAggregate } from "./types.ts";

export class Cell<T> {
  v: T;
  constructor(v: T) {
    this.v = v;
  }
}

export function cell<T = any>(v: T): Cell<T> {
  return new Cell(v);
}

class FieldPtr {
  private o: any;
  private k: string;
  constructor(o: any, k: string) {
    this.o = o;
    this.k = k;
  }
  get v(): any { return this.o[this.k]; }
  set v(x: any) { this.o[this.k] = x; }
}

class IndexPtr {
  private a: any[];
  private i: number;
  constructor(a: any[], i: number) {
    this.a = a;
    this.i = i;
  }
  get v(): any { return this.a[this.i]; }
  set v(x: any) { this.a[this.i] = x; }
}

const fieldPtrs = new WeakMap<object, Map<string | number, any>>();

function cached(o: object, k: string | number, make: () => any): any {
  let m = fieldPtrs.get(o);
  if (m === undefined) {
    m = new Map();
    fieldPtrs.set(o, m);
  }
  let p = m.get(k);
  if (p === undefined) {
    p = make();
    m.set(k, p);
  }
  return p;
}

export function fieldPtr(o: any, k: string): any {
  if (o === null) runtimePanic("invalid memory address or nil pointer dereference");
  return cached(o, k, () => new FieldPtr(o, k));
}

export function arrayElemPtr(a: any[], i: number): any {
  if (i < 0 || i >= a.length) runtimePanic(`index out of range [${i}] with length ${a.length}`);
  return cached(a, i, () => new IndexPtr(a, i));
}

export function sliceElemPtr(s: Slice<any> | null, i: number): any {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) runtimePanic(`index out of range [${i}] with length ${n}`);
  return arrayElemPtr(s!.$array, s!.$offset + i);
}

// Generic helpers for code whose pointee type is a type parameter.
export function newPtr(t: Type): any {
  return isAggregate(t) ? t.zero() : new Cell(t.zero());
}

// newPtrOf is new(v) for a value v of type t (already copied).
export function newPtrOf(t: Type, v: any): any {
  return isAggregate(t) ? v : new Cell(v);
}

export function load(t: Type, p: any): any {
  if (p === null) runtimePanic("invalid memory address or nil pointer dereference");
  return isAggregate(t) ? p : p.v;
}

export function store(t: Type, p: any, v: any): void {
  if (p === null) runtimePanic("invalid memory address or nil pointer dereference");
  if (isAggregate(t)) assign(t, p, v);
  else p.v = v;
}

// tpAddr is &x for a variable x of a type parameter's type t whose address is
// taken: x is a Cell, and a pointer to an aggregate is the object itself.
export function tpAddr(t: Type, x: Cell<any>): any {
  return isAggregate(t) ? x.v : x;
}

// tpSet assigns v to such a variable, in place for an aggregate so that
// pointers to it observe the change.
export function tpSet(t: Type, x: Cell<any>, v: any): void {
  if (isAggregate(t)) assign(t, x.v, v);
  else x.v = v;
}

// assign copies aggregate value src into the existing object dst in place, so
// pointers to dst observe the change.
export function assign(t: Type, dst: any, src: any): void {
  if (t.kind === 25 /* Struct */) dst.$set(src, t);
  else for (let i = 0; i < src.length; i++) {
    if (isAggregate(t.elem!)) assign(t.elem!, dst[i], src[i]);
    else dst[i] = src[i];
  }
}

// deref is the implicit dereference of a *struct / *array, which is the
// aggregate object itself: it only checks for nil.
export function deref<T = any>(p: T | null): T {
  if (p === null) runtimePanic("invalid memory address or nil pointer dereference");
  return p;
}

// derefMethod is the receiver dereference of a value method called through a
// method expression or method table with a *T receiver; a nil pointer panics
// like Go's panicwrap.
export function derefMethod<T = any>(p: T | null, msg: string): T {
  if (p === null) plainPanic(msg);
  return p;
}

// nilFunc stands in for a nil function value at a call: calling it is the
// nil dereference panic (after the arguments were evaluated, as in Go).
export function nilFunc(): never {
  runtimePanic("invalid memory address or nil pointer dereference");
}

// zeroSizePtrEq compares pointers to zero-size values (struct{}, [0]int).
// Like gc, which gives them all the address runtime.zerobase, two non-nil
// pointers are equal.
export function zeroSizePtrEq(a: unknown, b: unknown): boolean {
  return a === null ? b === null : b !== null;
}

const addresses = new WeakMap<object, number>();
let nextAddress = 0xc000010000;

// addressOf stands in for uintptr(unsafe.Pointer(p)): a stable number per
// pointer object, distinct for distinct pointers. There is no memory behind
// it, so arithmetic on it means nothing.
export function addressOf(p: any): number {
  if (p === null || p === undefined || (typeof p !== "object" && typeof p !== "function")) return 0;
  let a = addresses.get(p);
  if (a === undefined) {
    a = nextAddress;
    nextAddress += 0x1000;
    addresses.set(p, a);
  }
  return a;
}
