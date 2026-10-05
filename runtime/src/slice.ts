// Go slices: (backing array, offset, length, capacity). Slicing and append
// share the backing array exactly as in Go, so aliasing is observable the
// same way. A nil slice is JS null.
//
// The backing store is a plain JS array in the PoC. The representation is
// private to this module, which leaves room for TypedArray/ArrayBuffer backed
// slices (needed for unsafe and efficient []byte) later.

import { copy } from "./iface.ts";
import { indexError, runtimePanic, sliceError } from "./panic.ts";
import { arrayElemPtr, arrayViews as views, assign } from "./ptr.ts";
import { isAggregate, Type, zeroByte, tUint8 } from "./types.ts";

// Fields of the runtime's classes are declare'd and set by the constructor:
// a class field without declare is first defined as undefined, which makes V8
// store every value of it tagged (each float64 boxed in a heap number).
export class Slice<T> {
  declare $array: T[];
  declare $offset: number;
  declare $length: number;
  declare $capacity: number;
  constructor($array: T[], $offset: number, $length: number, $capacity: number) {
    this.$array = $array;
    this.$offset = $offset;
    this.$length = $length;
    this.$capacity = $capacity;
  }
}

export type S<T> = Slice<T> | null;

export function sliceLit<T = any>(arr: T[]): Slice<T> {
  return new Slice(arr, 0, arr.length, arr.length);
}

// makeSlice implements make([]T, len, cap); cap is undefined for make([]T, len).
// A slice's backing array is a JS array, so maxSliceLen (the most elements a
// JS array can hold) plays the part of gc's maxAlloc/elemsize. A []byte
// made with zeroByte (tUint8's zero) is backed by a Uint8Array, which
// the engine zeroes and copies in bulk; byte slices of Go arrays and slice
// literals keep JS arrays, and everything indexes both alike.
const maxSliceLen = 2 ** 32 - 1;

export function makeSlice<T = any>(len: number, cap: number | undefined, zero: () => T): Slice<T> {
  if (!(len >= 0 && len <= maxSliceLen) || !Number.isInteger(len)) runtimePanic("makeslice: len out of range");
  cap = cap ?? len;
  if (!(cap >= len && cap <= maxSliceLen)) runtimePanic("makeslice: cap out of range");
  if (zero === (zeroByte as any)) return new Slice(newBytes(cap) as any, 0, len, cap);
  const arr = new Array<T>(cap);
  fillZero(arr, 0, len, zero);
  if (cap > len) {
    if (len > 0 && typeof arr[0] === "object" && arr[0] !== null) spare(arr, zero);
    else if (len === 0 && isObject(zero())) spare(arr, zero);
    else fillZero(arr, len, cap, zero);
  }
  return new Slice(arr, 0, len, cap);
}

const isObject = (z: unknown) => typeof z === "object" && z !== null;

// The elements of a backing array of aggregates (struct and array objects)
// beyond the length of every slice of it are made when a slice first
// reaches them, by slice, append or unsafe.Slice: append(make([]T, 0, n),
// ...) and growing slices of structs then make each element once, where
// zero values for the spare capacity would be made and then overwritten.
// spare marks such an array with the function making its zero elements.
function spare<T>(arr: T[], zero: () => T): void {
  (arr as any).$zero = zero;
}

// reach makes the missing elements arr[from:to] of an array that spare
// marked.
export function reach<T>(arr: T[], from: number, to: number): void {
  const zero = (arr as any).$zero as (() => T) | undefined;
  if (zero === undefined) return;
  for (let i = from; i < to; i++) if (arr[i] === undefined) arr[i] = zero();
}

// newBytes returns n zero bytes. V8 keeps the store of a Uint8Array of at
// most 64 bytes on the JS heap but allocates a larger one outside it, which
// costs 1-3 µs however small; so, like Node's Buffer pool, mid-sized arrays
// are views into a shared slab (16 KiB, at least four arrays), each range
// handed out once and so still zero. A view keeps its whole slab alive.
const slabSize = 16384;
let slab: ArrayBuffer | undefined;
let slabOff = slabSize;

export function newBytes(n: number): Uint8Array {
  if (n <= 64 || n > slabSize >>> 2) return new Uint8Array(n);
  if (slabOff + n > slabSize) {
    slab = new ArrayBuffer(slabSize);
    slabOff = 0;
  }
  const a = new Uint8Array(slab!, slabOff, n);
  slabOff = (slabOff + n + 7) & ~7;
  return a;
}

// isBytes reports whether a is a Uint8Array backing a []byte.
const isBytes = (a: unknown): a is Uint8Array => a instanceof Uint8Array;

// fillZero sets arr[from:to] to zero values. A primitive zero value is
// shared (Array.prototype.fill); an aggregate one is a new object per slot.
function fillZero<T>(arr: T[], from: number, to: number, zero: () => T): void {
  if (from >= to) return;
  const z = zero();
  if (typeof z !== "object" || z === null) {
    arr.fill(z, from, to);
    return;
  }
  arr[from] = z;
  for (let i = from + 1; i < to; i++) arr[i] = zero();
}

export function len(s: S<any>): number {
  return s === null ? 0 : s.$length;
}

export function cap(s: S<any>): number {
  return s === null ? 0 : s.$capacity;
}

function indexPanic(i: number, n: number): never {
  indexError(i, n);
}

export function index<T = any>(s: S<T>, i: number): T {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) indexPanic(i, n);
  return s!.$array[s!.$offset + i];
}

export function setIndex<T = any>(s: S<T>, i: number, v: T): void {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) indexPanic(i, n);
  s!.$array[s!.$offset + i] = v;
}

export function arrayIndex(n: number, i: number): number {
  if (i < 0 || i >= n) indexPanic(i, n);
  return i;
}

// slice implements s[lo:hi:max] on a slice.
export function slice<T = any>(s: S<T>, lo?: number, hi?: number, max?: number): S<T> {
  const c = s === null ? 0 : s.$capacity;
  const l = lo ?? 0;
  const h = hi ?? (s === null ? 0 : s.$length);
  const m = max ?? c;
  if (l < 0 || h < l || m < h || m > c) sliceError(l, h, max, c);
  if (s === null) return null;
  if (h > s.$length) reach(s.$array, s.$offset + s.$length, s.$offset + h);
  return new Slice(s.$array, s.$offset + l, h - l, m - l);
}

// sliceArray implements a[lo:hi:max] on an (addressable) array.
export function sliceArray<T = any>(a: T[], lo?: number, hi?: number, max?: number): Slice<T> {
  const c = a.length;
  const l = lo ?? 0;
  const h = hi ?? c;
  const m = max ?? c;
  if (l < 0 || h < l || m < h || m > c) sliceError(l, h, max, c, "length");
  const v = views.get(a);
  if (v !== undefined) return new Slice(v.a, v.off + l, h - l, m - l);
  return new Slice(a, l, h - l, m - l);
}

// sliceToArrayPtr implements the conversion (*[n]T)(s). A pointer to an array
// is the JS array itself, so the result must be an array that aliases s's
// backing array: the backing array itself when s covers all of it (as for
// make([]T, n) or a[:]), otherwise a Proxy that views s's elements.
const viewCache = new WeakMap<object, Map<string, any[]>>();

export function sliceToArrayPtr<T = any>(s: S<T>, n: number): T[] | null {
  const l = s === null ? 0 : s.$length;
  if (l < n) runtimePanic(`cannot convert slice with length ${l} to array or pointer to array with length ${n}`);
  if (s === null) return null;
  const a = s.$array, off = s.$offset;
  if (off === 0 && a.length === n && !isBytes(a)) return a; // a Go array is a JS array
  let m = viewCache.get(a);
  if (m === undefined) viewCache.set(a, (m = new Map()));
  const key = off + ":" + n;
  let p = m.get(key);
  if (p === undefined) {
    const index = (k: string | symbol): number => {
      if (typeof k !== "string") return -1;
      const i = +k;
      return Number.isInteger(i) && i >= 0 && i < n && String(i) === k ? i : -1;
    };
    p = new Proxy(new Array<T>(n), {
      get(t, k, r) {
        const i = index(k);
        return i >= 0 ? a[off + i] : Reflect.get(t, k, r);
      },
      set(t, k, x, r) {
        const i = index(k);
        if (i >= 0) {
          a[off + i] = x;
          return true;
        }
        return Reflect.set(t, k, x, r);
      },
      has(t, k) {
        return index(k) >= 0 || Reflect.has(t, k);
      },
      getOwnPropertyDescriptor(t, k) {
        const i = index(k);
        return i >= 0 ? { value: a[off + i], writable: true, enumerable: true, configurable: true } : Reflect.getOwnPropertyDescriptor(t, k);
      },
      ownKeys(t) {
        return [...Array.from({ length: n }, (_, i) => String(i)), ...Reflect.ownKeys(t)];
      },
    });
    views.set(p, { a, off });
    m.set(key, p);
  }
  return p;
}

function grow(oldCap: number, needed: number): number {
  // Approximates runtime.growslice (double small slices, 1.25x large ones);
  // the exact capacities differ from gc because size-class rounding is not
  // modelled.
  let c = oldCap;
  const doubled = c + c;
  if (needed > doubled) return needed;
  if (c < 256) return doubled;
  while (c < needed) c += (c + 3 * 256) >> 2;
  return c;
}

// append implements the append builtin. et, the element type, is passed
// when elements may be aggregates (structs / arrays are objects): they are
// then copied, since a slice element is a value.
export function append<T = any>(s: S<T>, vals: T[], zero: () => T, et?: Type): S<T> {
  if (vals.length === 0) return s;
  const agg = et !== undefined && isAggregate(et);
  // Copy aggregate inputs first: they may alias the destination slots
  // (append(a[:1], a[:2]...)).
  if (agg) vals = vals.map((v) => copy(et!, v));
  const n = s === null ? 0 : s.$length;
  const c = s === null ? 0 : s.$capacity;
  const newLen = n + vals.length;
  if (s !== null && newLen <= c) {
    for (let i = 0; i < vals.length; i++) {
      const j = s.$offset + n + i;
      const o = s.$array[j];
      // An element not made yet (see spare) takes the copy.
      if (agg && o !== undefined) assign(et!, o, vals[i]);
      else s.$array[j] = vals[i];
    }
    return new Slice(s.$array, s.$offset, newLen, c);
  }
  const r = grown(s, newLen, zero, et);
  for (let i = 0; i < vals.length; i++) r.$array[n + i] = vals[i];
  return r;
}

// grown returns the backing array of append(s, ...) for a length of newLen
// that exceeds s's capacity, holding s's elements; et as in append.
function grown<T>(s: S<T>, newLen: number, zero: () => T, et?: Type): Slice<T> {
  if (newLen > maxSliceLen) runtimePanic("growslice: len out of range");
  const n = s === null ? 0 : s.$length;
  const newCap = Math.min(grow(s === null ? 0 : s.$capacity, newLen), maxSliceLen);
  if (zero === (zeroByte as any)) {
    const b = newBytes(newCap);
    if (n > 0) copyInto(b, 0, s!.$array, s!.$offset, n);
    return new Slice(b as any, 0, newLen, newCap);
  }
  const arr = new Array<T>(newCap);
  const agg = et !== undefined && isAggregate(et);
  for (let i = 0; i < n; i++) {
    const v = s!.$array[s!.$offset + i];
    arr[i] = agg ? copy(et!, v) : v;
  }
  if (agg) spare(arr, zero);
  else fillZero(arr, newLen, newCap, zero);
  return new Slice(arr, 0, newLen, newCap);
}

// append1 is append(s, v) for a non-aggregate v (the common case), without
// the array of values append takes.
export function append1<T = any>(s: S<T>, v: T, zero: () => T): Slice<T> {
  if (s !== null && s.$length < s.$capacity) {
    s.$array[s.$offset + s.$length] = v;
    return new Slice(s.$array, s.$offset, s.$length + 1, s.$capacity);
  }
  const r = grown(s, (s === null ? 0 : s.$length) + 1, zero);
  r.$array[r.$length - 1] = v;
  return r;
}

// appendNew1 is append(s, v) for a value v of element type et (an aggregate
// or a type parameter) made for the call alone: a literal, or a copy the
// caller made. It is not copied again.
export function appendNew1<T = any>(s: S<T>, v: T, zero: () => T, et: Type): Slice<T> {
  if (s !== null && s.$length < s.$capacity) {
    const j = s.$offset + s.$length;
    const o = s.$array[j];
    if (o !== undefined && isAggregate(et)) assign(et, o, v);
    else s.$array[j] = v;
    return new Slice(s.$array, s.$offset, s.$length + 1, s.$capacity);
  }
  const r = grown(s, (s === null ? 0 : s.$length) + 1, zero, et);
  r.$array[r.$length - 1] = v;
  return r;
}

// appendSlice is append(dst, src...) for a slice src of non-aggregates. When
// src shares dst's backing array, copyInto moves the elements as memmove
// does (append(a[:1], a[:2]...), append(a[:i], a[i+1:]...)).
export function appendSlice<T = any>(dst: S<T>, src: S<T>, zero: () => T): S<T> {
  if (src === null || src.$length === 0) return dst;
  const n = dst === null ? 0 : dst.$length;
  const m = src.$length;
  let r: Slice<T>;
  if (dst !== null && n + m <= dst.$capacity) r = new Slice(dst.$array, dst.$offset, n + m, dst.$capacity);
  else r = grown(dst, n + m, zero);
  copyInto(r.$array, r.$offset + n, src.$array, src.$offset, m);
  return r;
}

// copyInto copies src[so:so+n] to dst[do:] for non-aggregate elements, in
// bulk between Uint8Arrays, and correctly for overlapping ranges.
function copyInto(dst: any, d: number, src: any, so: number, n: number): void {
  if (isBytes(dst) && isBytes(src)) {
    dst.set(so === 0 && n === src.length ? src : src.subarray(so, so + n), d);
    return;
  }
  if (dst === src && d > so) {
    for (let i = n - 1; i >= 0; i--) dst[d + i] = src[so + i];
    return;
  }
  for (let i = 0; i < n; i++) dst[d + i] = src[so + i];
}

// appendString is append(b, s...) for a []byte b and a string s.
export function appendString(b: S<number>, s: string): S<number> {
  const m = s.length;
  if (m === 0) return b;
  const n = b === null ? 0 : b.$length;
  let r: Slice<number>;
  if (b !== null && n + m <= b.$capacity) r = new Slice(b.$array, b.$offset, n + m, b.$capacity);
  else r = grown(b, n + m, zeroByte);
  const a = r.$array, o = r.$offset + n;
  for (let i = 0; i < m; i++) a[o + i] = s.charCodeAt(i);
  return r;
}

// sliceToArray implements the conversion [n]T(s), which needs len(s) >= n.
export function sliceToArray<T = any>(s: S<T>, n: number): T[] {
  const l = s === null ? 0 : s.$length;
  if (l < n) runtimePanic(`cannot convert slice with length ${l} to array or pointer to array with length ${n}`);
  return toArray(slice(s, 0, n));
}

export function toArray<T = any>(s: S<T> | string): T[] {
  if (s === null) return [];
  if (typeof s === "string") {
    // append(b, s...) with s of a type parameter like ~string | ~[]byte.
    const a = new Array<number>(s.length);
    for (let i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
    return a as T[];
  }
  if (isBytes(s.$array)) return Array.from(s.$array.subarray(s.$offset, s.$offset + s.$length)) as T[];
  return s.$array.slice(s.$offset, s.$offset + s.$length);
}

// copy implements the copy builtin. src may be a string (copy([]byte, string)).
export function sliceCopy<T = any>(dst: S<T>, src: S<T> | string, et?: Type): number {
  if (dst === null || src === null) return 0;
  if (typeof src === "string") {
    const n = Math.min(dst.$length, src.length);
    for (let i = 0; i < n; i++) (dst.$array as any)[dst.$offset + i] = src.charCodeAt(i);
    return n;
  }
  const n = Math.min(dst.$length, src.$length);
  const agg = et !== undefined && isAggregate(et);
  if (isBytes(dst.$array) && isBytes(src.$array)) {
    copyInto(dst.$array, dst.$offset, src.$array, src.$offset, n);
    return n;
  }
  const set = (i: number) => {
    const v = src.$array[src.$offset + i];
    if (agg) assign(et!, dst.$array[dst.$offset + i], v); // in place: &dst[i] stays valid
    else dst.$array[dst.$offset + i] = v;
  };
  if (dst.$array === src.$array && dst.$offset > src.$offset) {
    for (let i = n - 1; i >= 0; i--) set(i);
  } else {
    for (let i = 0; i < n; i++) set(i);
  }
  return n;
}

export function sliceClear<T = any>(s: S<T>, zero: () => T, et?: Type): void {
  if (s === null) return;
  const agg = et !== undefined && isAggregate(et);
  for (let i = 0; i < s.$length; i++) {
    // Aggregates are zeroed in place: &s[i] is the element object.
    if (agg) assign(et!, s.$array[s.$offset + i], zero());
    else s.$array[s.$offset + i] = zero();
  }
}

// Operations on values whose static type is a type parameter without a core
// type (e.g. ~string | ~[]byte): dispatch on the representation.
export function indexAny(x: any, i: number): any {
  if (typeof x === "string") {
    if (i < 0 || i >= x.length) indexPanic(i, x.length);
    return x.charCodeAt(i);
  }
  if (Array.isArray(x)) return x[arrayIndex(x.length, i)]; // an array or *array
  return index(x, i);
}

export function setIndexAny(x: any, i: number, v: any): void {
  if (Array.isArray(x)) x[arrayIndex(x.length, i)] = v;
  else setIndex(x, i, v);
}

export function lenAny(x: any): number {
  return typeof x === "string" || Array.isArray(x) ? x.length : len(x);
}

export function capAny(x: any): number {
  return Array.isArray(x) ? x.length : cap(x);
}

export function sliceAny(x: any, lo?: number, hi?: number): any {
  if (typeof x === "string") {
    const l = lo ?? 0, h = hi ?? x.length;
    if (l < 0 || h < l || h > x.length) sliceError(l, h, undefined, x.length, "length");
    return x.substring(l, h);
  }
  return slice(x, lo, hi);
}

// unsafeSlice implements unsafe.Slice(&x[i], n) (and, through bytesToString,
// unsafe.String) for x a slice or array: a slice of length and capacity n
// over x's backing array starting at element i. checked: the operand was
// &x[i], which panics like x[i] when i is out of range.
export function unsafeSlice<T = any>(x: S<T> | T[] | null, i: number, n: number, checked: boolean): S<T> {
  if (n < 0) runtimePanic("unsafe.Slice: len out of range");
  let arr: T[], off: number, len: number, c: number;
  if (x === null) {
    if (checked) indexPanic(i, 0);
    if (n !== 0) runtimePanic("unsafe.Slice: ptr is nil and len is not zero");
    return null;
  } else if (x instanceof Slice) {
    [arr, off, len, c] = [x.$array, x.$offset, x.$length, x.$capacity];
  } else {
    [arr, off, len, c] = [x, 0, x.length, x.length];
  }
  if (checked && (i < 0 || i >= len)) indexPanic(i, len);
  if (i + n > c) runtimePanic("unsafe.Slice: len out of range (beyond the underlying array)");
  reach(arr, off + i, off + i + n);
  return new Slice(arr, off + i, n, n);
}

// sliceData implements unsafe.SliceData: a pointer to the first element of
// the backing array (the element object itself for aggregates).
export function sliceData<T = any>(s: S<T>, aggregate: boolean): any {
  if (s === null || s.$capacity === 0) return null;
  if (aggregate) {
    reach(s.$array, s.$offset, s.$offset + 1);
    const e: any = s.$array[s.$offset];
    elemOrigins.set(e, { a: s.$array, i: s.$offset });
    return e;
  }
  return arrayElemPtr(s.$array, s.$offset);
}

// elemOrigins remembers where the aggregate elements that unsafe.SliceData
// returned live, so that unsafe.Slice can rebuild a slice from them.
export const elemOrigins = new WeakMap<object, { a: any[]; i: number }>();
