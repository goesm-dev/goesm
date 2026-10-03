// Go slices: (backing array, offset, length, capacity). Slicing and append
// share the backing array exactly as in Go, so aliasing is observable the
// same way. A nil slice is JS null.
//
// The backing store is a plain JS array in the PoC. The representation is
// private to this module, which leaves room for TypedArray/ArrayBuffer backed
// slices (needed for unsafe and efficient []byte) later.

import { copy } from "./iface.ts";
import { runtimePanic } from "./panic.ts";
import { arrayElemPtr, assign } from "./ptr.ts";
import { isAggregate, Type } from "./types.ts";

export class Slice<T> {
  $array: T[];
  $offset: number;
  $length: number;
  $capacity: number;
  constructor($array: T[], $offset: number, $length: number, $capacity: number) {
    this.$array = $array;
    this.$offset = $offset;
    this.$length = $length;
    this.$capacity = $capacity;
  }
}

export type S<T> = Slice<T> | null;

export function sliceLit<T>(arr: T[]): Slice<T> {
  return new Slice(arr, 0, arr.length, arr.length);
}

// makeSlice implements make([]T, len, cap); cap is undefined for make([]T, len).
export function makeSlice<T>(len: number, cap: number | undefined, zero: () => T): Slice<T> {
  if (len < 0 || !Number.isInteger(len)) runtimePanic("makeslice: len out of range");
  cap = cap ?? len;
  if (cap < len) runtimePanic("makeslice: cap out of range");
  const arr = new Array<T>(cap);
  for (let i = 0; i < cap; i++) arr[i] = zero();
  return new Slice(arr, 0, len, cap);
}

export function len(s: S<any>): number {
  return s === null ? 0 : s.$length;
}

export function cap(s: S<any>): number {
  return s === null ? 0 : s.$capacity;
}

function indexPanic(i: number, n: number): never {
  runtimePanic(`index out of range [${i}] with length ${n}`);
}

export function index<T>(s: S<T>, i: number): T {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) indexPanic(i, n);
  return s!.$array[s!.$offset + i];
}

export function setIndex<T>(s: S<T>, i: number, v: T): void {
  const n = s === null ? 0 : s.$length;
  if (i < 0 || i >= n) indexPanic(i, n);
  s!.$array[s!.$offset + i] = v;
}

export function arrayIndex(n: number, i: number): number {
  if (i < 0 || i >= n) indexPanic(i, n);
  return i;
}

function boundsPanic(lo: number, hi: number, max: number, c: number): never {
  if (max > c) runtimePanic(`slice bounds out of range [::${max}] with capacity ${c}`);
  if (hi > max) runtimePanic(`slice bounds out of range [:${hi}] with capacity ${c}`);
  runtimePanic(`slice bounds out of range [${lo}:${hi}]`);
}

// slice implements s[lo:hi:max] on a slice.
export function slice<T>(s: S<T>, lo?: number, hi?: number, max?: number): S<T> {
  const c = s === null ? 0 : s.$capacity;
  const l = lo ?? 0;
  const h = hi ?? (s === null ? 0 : s.$length);
  const m = max ?? c;
  if (l < 0 || h < l || m < h || m > c) boundsPanic(l, h, m, c);
  if (s === null) return null;
  return new Slice(s.$array, s.$offset + l, h - l, m - l);
}

// sliceArray implements a[lo:hi:max] on an (addressable) array.
export function sliceArray<T>(a: T[], lo?: number, hi?: number, max?: number): Slice<T> {
  const c = a.length;
  const l = lo ?? 0;
  const h = hi ?? c;
  const m = max ?? c;
  if (l < 0 || h < l || m < h || m > c) boundsPanic(l, h, m, c);
  return new Slice(a, l, h - l, m - l);
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
export function append<T>(s: S<T>, vals: T[], zero: () => T, et?: Type): S<T> {
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
      if (agg) assign(et!, s.$array[j], vals[i]);
      else s.$array[j] = vals[i];
    }
    return new Slice(s.$array, s.$offset, newLen, c);
  }
  const newCap = grow(c, newLen);
  const arr = new Array<T>(newCap);
  for (let i = 0; i < n; i++) {
    const v = s!.$array[s!.$offset + i];
    arr[i] = agg ? copy(et!, v) : v;
  }
  for (let i = 0; i < vals.length; i++) arr[n + i] = vals[i];
  for (let i = newLen; i < newCap; i++) arr[i] = zero();
  return new Slice(arr, 0, newLen, newCap);
}

// sliceToArray implements the conversion [n]T(s), which needs len(s) >= n.
export function sliceToArray<T>(s: S<T>, n: number): T[] {
  const l = s === null ? 0 : s.$length;
  if (l < n) runtimePanic(`cannot convert slice with length ${l} to array or pointer to array with length ${n}`);
  return toArray(slice(s, 0, n));
}

export function toArray<T>(s: S<T>): T[] {
  if (s === null) return [];
  return s.$array.slice(s.$offset, s.$offset + s.$length);
}

// copy implements the copy builtin. src may be a string (copy([]byte, string)).
export function sliceCopy<T>(dst: S<T>, src: S<T> | string, et?: Type): number {
  if (dst === null || src === null) return 0;
  if (typeof src === "string") {
    const n = Math.min(dst.$length, src.length);
    for (let i = 0; i < n; i++) (dst.$array as any)[dst.$offset + i] = src.charCodeAt(i);
    return n;
  }
  const n = Math.min(dst.$length, src.$length);
  const agg = et !== undefined && isAggregate(et);
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

export function sliceClear<T>(s: S<T>, zero: () => T, et?: Type): void {
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
  return index(x, i);
}

export function lenAny(x: any): number {
  return typeof x === "string" ? x.length : len(x);
}

export function capAny(x: any): number {
  return cap(x);
}

export function sliceAny(x: any, lo?: number, hi?: number): any {
  if (typeof x === "string") {
    const l = lo ?? 0, h = hi ?? x.length;
    if (l < 0 || h < l || h > x.length) runtimePanic(`slice bounds out of range [${l}:${h}] with length ${x.length}`);
    return x.substring(l, h);
  }
  return slice(x, lo, hi);
}

// unsafeSlice implements unsafe.Slice(&x[i], n) (and, through bytesToString,
// unsafe.String) for x a slice or array: a slice of length and capacity n
// over x's backing array starting at element i. checked: the operand was
// &x[i], which panics like x[i] when i is out of range.
export function unsafeSlice<T>(x: S<T> | T[] | null, i: number, n: number, checked: boolean): S<T> {
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
  return new Slice(arr, off + i, n, n);
}

// sliceData implements unsafe.SliceData: a pointer to the first element of
// the backing array (the element object itself for aggregates).
export function sliceData<T>(s: S<T>, aggregate: boolean): any {
  if (s === null || s.$capacity === 0) return null;
  if (aggregate) return s.$array[s.$offset];
  return arrayElemPtr(s.$array, s.$offset);
}
