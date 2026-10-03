// Go slices: (backing array, offset, length, capacity). Slicing and append
// share the backing array exactly as in Go, so aliasing is observable the
// same way. A nil slice is JS null.
//
// The backing store is a plain JS array in the PoC. The representation is
// private to this module, which leaves room for TypedArray/ArrayBuffer backed
// slices (needed for unsafe and efficient []byte) later.

import { runtimePanic } from "./panic";

export class Slice<T> {
  constructor(
    public $array: T[],
    public $offset: number,
    public $length: number,
    public $capacity: number,
  ) {}
}

export type S<T> = Slice<T> | null;

export function sliceLit<T>(arr: T[]): Slice<T> {
  return new Slice(arr, 0, arr.length, arr.length);
}

export function makeSlice<T>(len: number, cap: number, zero: () => T): Slice<T> {
  if (len < 0 || !Number.isInteger(len)) runtimePanic("makeslice: len out of range");
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

export function append<T>(s: S<T>, vals: T[], zero: () => T): S<T> {
  if (vals.length === 0) return s;
  const n = s === null ? 0 : s.$length;
  const c = s === null ? 0 : s.$capacity;
  const newLen = n + vals.length;
  if (s !== null && newLen <= c) {
    for (let i = 0; i < vals.length; i++) s.$array[s.$offset + n + i] = vals[i];
    return new Slice(s.$array, s.$offset, newLen, c);
  }
  const newCap = grow(c, newLen);
  const arr = new Array<T>(newCap);
  for (let i = 0; i < n; i++) arr[i] = s!.$array[s!.$offset + i];
  for (let i = 0; i < vals.length; i++) arr[n + i] = vals[i];
  for (let i = newLen; i < newCap; i++) arr[i] = zero();
  return new Slice(arr, 0, newLen, newCap);
}

export function toArray<T>(s: S<T>): T[] {
  if (s === null) return [];
  return s.$array.slice(s.$offset, s.$offset + s.$length);
}

// copy implements the copy builtin. src may be a string (copy([]byte, string)).
export function sliceCopy<T>(dst: S<T>, src: S<T> | string): number {
  if (dst === null || src === null) return 0;
  if (typeof src === "string") {
    const n = Math.min(dst.$length, src.length);
    for (let i = 0; i < n; i++) (dst.$array as any)[dst.$offset + i] = src.charCodeAt(i);
    return n;
  }
  const n = Math.min(dst.$length, src.$length);
  if (dst.$array === src.$array && dst.$offset > src.$offset) {
    for (let i = n - 1; i >= 0; i--) dst.$array[dst.$offset + i] = src.$array[src.$offset + i];
  } else {
    for (let i = 0; i < n; i++) dst.$array[dst.$offset + i] = src.$array[src.$offset + i];
  }
  return n;
}

export function sliceClear<T>(s: S<T>, zero: () => T): void {
  if (s === null) return;
  for (let i = 0; i < s.$length; i++) s.$array[s.$offset + i] = zero();
}
