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

import { indexError, plainPanic, runtimePanic } from "./panic.ts";
import { Slice } from "./slice.ts";
import { Type, isAggregate } from "./types.ts";

export class Cell<T> {
  declare v: T;
  constructor(v: T) {
    this.v = v;
  }
}

export function cell<T = any>(v: T): Cell<T> {
  return new Cell(v);
}

class FieldPtr {
  private declare o: any;
  private declare k: string;
  constructor(o: any, k: string) {
    this.o = o;
    this.k = k;
  }
  get v(): any { return this.o[this.k]; }
  set v(x: any) { this.o[this.k] = x; }
}

class IndexPtr {
  private declare a: any[];
  private declare i: number;
  constructor(a: any[], i: number) {
    this.a = a;
    this.i = i;
  }
  get v(): any { return this.a[this.i]; }
  set v(x: any) { this.a[this.i] = x; }
}

// fieldPtrTarget returns the object and property a field pointer refers to.
export function fieldPtrTarget(p: unknown): { o: any; k: string } | undefined {
  return p instanceof FieldPtr ? { o: (p as any).o, k: (p as any).k } : undefined;
}

// indexPtrTarget returns the array and index an element pointer refers to.
export function indexPtrTarget(p: unknown): { a: any[]; i: number } | undefined {
  return p instanceof IndexPtr ? { a: (p as any).a, i: (p as any).i } : undefined;
}

// The pointers made into an object are kept on it, under a symbol (which JS
// code does not see), so that equal addresses are equal pointers.
const ptrsOf = Symbol("ptrs");

function ptrs(o: any): Map<string | number, any> {
  let m: Map<string | number, any> | undefined = o[ptrsOf];
  if (m === undefined) o[ptrsOf] = m = new Map();
  return m;
}

function cached(o: object, k: string | number, p: any): any {
  const m = ptrs(o);
  const q = m.get(k);
  if (q !== undefined) return q;
  m.set(k, p);
  return p;
}

export function fieldPtr(o: any, k: string): any {
  if (o === null) runtimePanic("invalid memory address or nil pointer dereference");
  const m = ptrs(o);
  let p = m.get(k);
  if (p === undefined) m.set(k, (p = new FieldPtr(o, k)));
  return p;
}

// arrayViews maps the arrays that view part of another array (see
// sliceToArrayPtr) to what they view.
export const arrayViews = new WeakMap<object, { a: any[]; off: number }>();

export function arrayElemPtr(a: any[], i: number): any {
  if (i < 0 || i >= a.length) indexError(i, a.length);
  const v = arrayViews.get(a);
  if (v !== undefined) {
    a = v.a;
    i += v.off;
  }
  const m = ptrs(a);
  let p = m.get(i);
  if (p === undefined) m.set(i, (p = new IndexPtr(a, i)));
  return p;
}

export function sliceElemPtr(s: Slice<any> | null, i: number): any {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) indexError(i, n);
  return arrayElemPtr(s!.$array, s!.$offset + i);
}

// fieldRef, arrayElemRef and sliceElemRef are &o.k, &a[i] and &s[i] without
// the cache that keeps pointer identity, for pointers that are only loaded
// from and stored to (reflect's addressable Values); canonical returns the
// cached pointer equal to one of them, for when it becomes a Go value.
export function fieldRef(o: any, k: string): any {
  if (o === null) runtimePanic("invalid memory address or nil pointer dereference");
  return new FieldPtr(o, k);
}

export function arrayElemRef(a: any[], i: number): any {
  if (i < 0 || i >= a.length) indexError(i, a.length);
  const v = arrayViews.get(a);
  if (v !== undefined) {
    a = v.a;
    i += v.off;
  }
  return new IndexPtr(a, i);
}

export function sliceElemRef(s: Slice<any> | null, i: number): any {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) indexError(i, n);
  return arrayElemRef(s!.$array, s!.$offset + i);
}

export function canonical(p: any): any {
  if (p instanceof FieldPtr) return cached((p as any).o, (p as any).k, p);
  if (p instanceof IndexPtr) return cached((p as any).a, (p as any).i, p);
  return p;
}

// &o.k, &a[i] and &s[i] for an element of a type parameter's type t: a
// pointer to an aggregate is the object itself.
export function tpFieldAddr(t: Type, o: any, k: string): any {
  if (!isAggregate(t)) return fieldPtr(o, k);
  if (o === null) runtimePanic("invalid memory address or nil pointer dereference");
  return o[k];
}

export function tpArrayElemAddr(t: Type, a: any[], i: number): any {
  const p = arrayElemPtr(a, i);
  return isAggregate(t) ? a[i] : p;
}

export function tpSliceElemAddr(t: Type, s: Slice<any> | null, i: number): any {
  const p = sliceElemPtr(s, i);
  return isAggregate(t) ? s!.$array[s!.$offset + i] : p;
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
const pointers = new Map<number, WeakRef<object>>();
const forgetAddress = /* @__PURE__ */ new FinalizationRegistry<number>((a) => pointers.delete(a));
let nextAddress = 0xc000010000;

// addressOf stands in for uintptr(unsafe.Pointer(p)): a stable number per
// pointer object, distinct for distinct pointers. There is no memory behind
// it: arithmetic on it means nothing, but unsafe.Pointer of the number
// (fromAddress) is the pointer again.
export function addressOf(p: any): number {
  if (p === null || p === undefined || (typeof p !== "object" && typeof p !== "function")) return 0;
  let a = addresses.get(p);
  if (a === undefined) {
    a = nextAddress;
    nextAddress += 0x1000;
    addresses.set(p, a);
    pointers.set(a, new WeakRef(p));
    forgetAddress.register(p, a);
  }
  return a;
}

// fromAddress is unsafe.Pointer(a) for a uintptr a: the pointer addressOf
// gave a, or nil for 0.
export function fromAddress(a: number): any {
  if (a === 0) return null;
  const p = pointers.get(a)?.deref();
  if (p === undefined) runtimePanic("goesm: unsafe.Pointer of a uintptr that is not the address of a live pointer (goesm has no address space)");
  return p;
}

// embedFS builds the embed.FS value of a //go:embed variable: entries are
// [name, data] in embed's search order, directories named "dir/".
export function embedFS(fsType: Type, entries: [string, string][]): any {
  const fsys = fsType.zero();
  const filesField = fsType.fields[0]; // files *[]file
  const fileType = filesField.type.elem!.elem!;
  const [nameProp, dataProp] = [fileType.fields[0].prop, fileType.fields[1].prop];
  const files = entries.map(([name, data]) => {
    const f = fileType.zero();
    f[nameProp] = name;
    f[dataProp] = data;
    return f;
  });
  fsys[filesField.prop] = new Cell(new Slice(files, 0, files.length, files.length));
  return fsys;
}

// //go:linkname symbols (see internal/lower/linkname.go): the functions
// packages provide to pulls in other packages, and the result-less calls
// made before the providing package was initialized.
const linkSyms = new Map<string, (...a: any[]) => any>();
const linkQueue = new Map<string, any[][]>();

export function linkProvide(sym: string, fn: (...a: any[]) => any): void | Promise<void> {
  linkSyms.set(sym, fn);
  const q = linkQueue.get(sym);
  if (q === undefined) return;
  linkQueue.delete(sym);
  const run = (i: number): void | Promise<void> => {
    for (; i < q.length; i++) {
      const r = fn(...q[i]);
      if (r instanceof Promise) return r.then(() => run(i + 1));
    }
  };
  return run(0);
}

export function linkCall(sym: string, args: any[], deferrable: boolean): any {
  const fn = linkSyms.get(sym);
  if (fn !== undefined) return fn(...args);
  if (deferrable) {
    let q = linkQueue.get(sym);
    if (q === undefined) linkQueue.set(sym, (q = []));
    q.push(args);
    return undefined;
  }
  plainPanic(`goesm: ${sym} called through //go:linkname before its package was initialized`);
}
