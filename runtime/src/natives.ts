// Natives: the implementations of standard library functions that have no Go
// body (assembly, linkname, or a goesm replacement in internal/natives that
// leaves the body to the runtime), and of the few whose Go body goesm does
// not lower (internal/natives overrides).
//
// Generated code imports this module as $natives. Each native is an export
// named after the function's types.Func.FullName,
// "native$" + the name with every character other than letters, digits and
// _ replaced by "$": internal/bytealg.IndexByte is
// native$internal$bytealg$IndexByte. goesm reads the export names from this
// file, so a missing native is reported at compile time, and esbuild drops
// the natives a program does not call.
//
// The set is fixed and owned by goesm.

// natives.ts is a separate module (@goesm/runtime/natives) that uses the
// runtime only through its public module, so split builds share one runtime.
import {
  GoMap, Goexit, Iface, Kind, Slice, Type, assign, chanLen, copy, implementsIface, isAggregate, load,
  makeSlice, mapLen, numGoroutine, runtimePanic, sizeOf, store,
} from "./index.ts";
import type { S } from "./index.ts";

// ---- runtime ----

export function native$runtime$Goexit(): never {
  throw new Goexit();
}

export const native$runtime$NumGoroutine = numGoroutine;

// ---- math and internal/strconv: float bits ----
//
// 64-bit integers are JS numbers in the PoC, so the uint64 bits of a float64
// are exact only below 2^53 (see ARCHITECTURE.md, integers).

const scratch = new DataView(new ArrayBuffer(8));

export function native$math$Float64bits(f: number): number {
  scratch.setFloat64(0, f);
  return Number(scratch.getBigUint64(0));
}

export function native$math$Float64frombits(b: number): number {
  scratch.setBigUint64(0, BigInt.asUintN(64, BigInt(Math.trunc(b))));
  return scratch.getFloat64(0);
}

export function native$math$Float32bits(f: number): number {
  scratch.setFloat32(0, f);
  return scratch.getUint32(0);
}

export function native$math$Float32frombits(b: number): number {
  scratch.setUint32(0, b);
  return scratch.getFloat32(0);
}

export const native$internal$strconv$float64bits = native$math$Float64bits;
export const native$internal$strconv$float64frombits = native$math$Float64frombits;
export const native$internal$strconv$float32bits = native$math$Float32bits;
export const native$internal$strconv$float32frombits = native$math$Float32frombits;

// ---- math/bits (64-bit) ----
//
// The Go code's de Bruijn multiplications and 64-bit masks need exact uint64
// arithmetic; BigInt gives it for inputs below 2^53.

const M64 = (1n << 64n) - 1n;
const big = (x: number) => BigInt.asUintN(64, BigInt(Math.trunc(x)));
const num = (x: bigint) => Number(BigInt.asUintN(64, x));

export function native$math$bits$Len64(x: number): number {
  const b = big(x);
  return b === 0n ? 0 : b.toString(2).length;
}

export function native$math$bits$LeadingZeros64(x: number): number {
  return 64 - native$math$bits$Len64(x);
}

export function native$math$bits$TrailingZeros64(x: number): number {
  const b = big(x);
  return b === 0n ? 64 : (b & -b).toString(2).length - 1;
}

export function native$math$bits$OnesCount64(x: number): number {
  let n = 0;
  for (const c of big(x).toString(2)) if (c === "1") n++;
  return n;
}

export function native$math$bits$RotateLeft64(x: number, k: number): number {
  const s = BigInt(((k % 64) + 64) % 64), b = big(x);
  return num((b << s) | (b >> (64n - s)));
}

export function native$math$bits$Reverse64(x: number): number {
  return num(BigInt("0b" + big(x).toString(2).padStart(64, "0").split("").reverse().join("")));
}

export function native$math$bits$ReverseBytes64(x: number): number {
  let b = big(x), r = 0n;
  for (let i = 0; i < 8; i++) {
    r = (r << 8n) | (b & 0xffn);
    b >>= 8n;
  }
  return num(r);
}

export function native$math$bits$Add64(x: number, y: number, carry: number): [number, number] {
  const s = big(x) + big(y) + big(carry);
  return [num(s), Number(s >> 64n)];
}

export function native$math$bits$Sub64(x: number, y: number, borrow: number): [number, number] {
  const d = big(x) - big(y) - big(borrow);
  return [num(d), d < 0n ? 1 : 0];
}

export function native$math$bits$Mul64(x: number, y: number): [number, number] {
  const p = big(x) * big(y);
  return [num(p >> 64n), num(p & M64)];
}

export function native$math$bits$Div64(hi: number, lo: number, y: number): [number, number] {
  const d = big(y), h = big(hi);
  if (d === 0n) runtimePanic("integer divide by zero");
  if (d <= h) runtimePanic("integer overflow");
  const n = (h << 64n) | big(lo);
  return [num(n / d), num(n % d)];
}

export function native$math$bits$Rem64(hi: number, lo: number, y: number): number {
  const d = big(y);
  if (d === 0n) runtimePanic("integer divide by zero");
  return num(((big(hi) << 64n) | big(lo)) % d);
}

// ---- maps, slices ----

// maps.clone(m any) any: a shallow copy (keys and values are assigned, so
// aggregates are copied).
export function native$maps$clone(m: Iface | null): Iface | null {
  if (m === null || m.v === null) return m;
  const src = m.v as GoMap<any, any>, t = m.t;
  const dst = new GoMap<any, any>(src.keyType);
  for (const [h, [k, v]] of src.entries) dst.entries.set(h, [copy(t.key!, k), copy(t.elem!, v)]);
  return new Iface(t, dst);
}

// slices.overlaps compares element addresses in Go: here, backing arrays and
// index ranges.
export function native$slices$overlaps(_e: Type, a: S<any>, b: S<any>): boolean {
  return a !== null && b !== null && a.$length > 0 && b.$length > 0 && a.$array === b.$array &&
    a.$offset < b.$offset + b.$length && b.$offset < a.$offset + a.$length;
}

// ---- internal/abi ----

export function native$internal$abi$NoEscape(p: any): any {
  return p;
}

export function native$internal$abi$Escape(_t: Type, x: any): any {
  return x;
}

// ---- internal/bytealg ----

export function native$internal$bytealg$IndexByte(b: S<number>, c: number): number {
  if (b === null) return -1;
  const a = b.$array, o = b.$offset;
  for (let i = 0; i < b.$length; i++) if (a[o + i] === c) return i;
  return -1;
}

export function native$internal$bytealg$IndexByteString(s: string, c: number): number {
  return s.indexOf(String.fromCharCode(c));
}

export function native$internal$bytealg$Compare(x: S<number>, y: S<number>): number {
  const lx = x === null ? 0 : x.$length, ly = y === null ? 0 : y.$length;
  for (let i = 0; i < Math.min(lx, ly); i++) {
    const a = x!.$array[x!.$offset + i], b = y!.$array[y!.$offset + i];
    if (a !== b) return a < b ? -1 : 1;
  }
  return lx === ly ? 0 : lx < ly ? -1 : 1;
}

// Strings hold one byte per UTF-16 code unit, so JS order is byte order.
export function native$internal$bytealg$abigen_runtime_cmpstring(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

export function native$internal$bytealg$MakeNoZero(n: number): Slice<number> {
  return makeSlice(n, n, () => 0);
}

// ---- sync/atomic ----
//
// Goroutines share one thread and never preempt each other, so the atomic
// operations are plain loads and stores through the pointer (a Cell or a
// field pointer: p.v).

const ld = (p: any) => p.v;
const st = (p: any, v: any) => { p.v = v; };
const swap = (p: any, v: any) => { const old = p.v; p.v = v; return old; };
const cas = (p: any, old: any, v: any) => {
  if (p.v !== old) return false;
  p.v = v;
  return true;
};
const add = (w: (x: number) => number) => (p: any, d: number) => (p.v = w(p.v + d));
const and = (w: (x: number) => number) => (p: any, m: number) => { const old = p.v; p.v = w(Number(BigInt(old) & BigInt(m))); return old; };
const or = (w: (x: number) => number) => (p: any, m: number) => { const old = p.v; p.v = w(Number(BigInt(old) | BigInt(m))); return old; };
const i32 = (x: number) => x | 0, u32 = (x: number) => x >>> 0, n64 = (x: number) => x;

export const native$sync$atomic$LoadInt32 = ld, native$sync$atomic$LoadInt64 = ld, native$sync$atomic$LoadUint32 = ld,
  native$sync$atomic$LoadUint64 = ld, native$sync$atomic$LoadUintptr = ld, native$sync$atomic$LoadPointer = ld;
export const native$sync$atomic$StoreInt32 = st, native$sync$atomic$StoreInt64 = st, native$sync$atomic$StoreUint32 = st,
  native$sync$atomic$StoreUint64 = st, native$sync$atomic$StoreUintptr = st, native$sync$atomic$StorePointer = st;
export const native$sync$atomic$SwapInt32 = swap, native$sync$atomic$SwapInt64 = swap, native$sync$atomic$SwapUint32 = swap,
  native$sync$atomic$SwapUint64 = swap, native$sync$atomic$SwapUintptr = swap, native$sync$atomic$SwapPointer = swap;
export const native$sync$atomic$CompareAndSwapInt32 = cas, native$sync$atomic$CompareAndSwapInt64 = cas,
  native$sync$atomic$CompareAndSwapUint32 = cas, native$sync$atomic$CompareAndSwapUint64 = cas,
  native$sync$atomic$CompareAndSwapUintptr = cas, native$sync$atomic$CompareAndSwapPointer = cas;
export const native$sync$atomic$AddInt32 = add(i32), native$sync$atomic$AddUint32 = add(u32),
  native$sync$atomic$AddInt64 = add(n64), native$sync$atomic$AddUint64 = add(n64), native$sync$atomic$AddUintptr = add(n64);
export const native$sync$atomic$AndInt32 = and(i32), native$sync$atomic$AndUint32 = and(u32),
  native$sync$atomic$AndInt64 = and(n64), native$sync$atomic$AndUint64 = and(n64), native$sync$atomic$AndUintptr = and(n64);
export const native$sync$atomic$OrInt32 = or(i32), native$sync$atomic$OrUint32 = or(u32),
  native$sync$atomic$OrInt64 = or(n64), native$sync$atomic$OrUint64 = or(n64), native$sync$atomic$OrUintptr = or(n64);

// ---- internal/reflectlite ----
//
// A reflectlite *rtype is a runtime type descriptor; a Value's ptr is the JS
// value itself, or (flagAddr) a pointer to it.

export function native$internal$reflectlite$ifaceType(i: Iface | null): Type | null {
  return i === null ? null : i.t;
}

export function native$internal$reflectlite$ifaceValue(i: Iface | null): any {
  return i === null ? null : i.v;
}

export function native$internal$reflectlite$asIface(x: any): Iface | null {
  return x;
}

export function native$internal$reflectlite$typeName(t: Type): string {
  if (t.named) return t.name + (t.typeArgs.length ? `[${t.typeArgs.map((a) => a.str).join(",")}]` : "");
  if (t.kind === Kind.UnsafePointer) return "Pointer";
  // Predeclared types are named; composite literal types are not.
  return t.kind <= Kind.Complex128 || t.kind === Kind.String ? t.str : "";
}

export function native$internal$reflectlite$typePkgPath(t: Type): string {
  return t.named ? t.pkgPath : "";
}

export const native$internal$reflectlite$typeSize = sizeOf;

export function native$internal$reflectlite$typeKind(t: Type): number {
  return t.kind;
}

export function native$internal$reflectlite$typeString(t: Type): string {
  return t.str;
}

export function native$internal$reflectlite$typeComparable(t: Type): boolean {
  switch (t.kind) {
    case Kind.Slice: case Kind.Map: case Kind.Func:
      return false;
    case Kind.Array:
      return native$internal$reflectlite$typeComparable(t.elem!);
    case Kind.Struct:
      return t.fields.every((f) => native$internal$reflectlite$typeComparable(f.type));
  }
  return true;
}

export function native$internal$reflectlite$typeElem(t: Type): Type | null {
  return t.elem;
}

export function native$internal$reflectlite$implements(iface: Type, t: Type): boolean {
  if (iface.kind !== Kind.Interface) return false;
  if (t.kind === Kind.Interface) {
    return iface.imethods.every((m) => t.imethods.some((n) => n.name === m.name && n.pkgPath === m.pkgPath && n.type === m.type));
  }
  return implementsIface(t, iface);
}

export function native$internal$reflectlite$directlyAssignable(dst: Type, src: Type): boolean {
  if (dst === src) return true;
  if ((dst.named && src.named) || dst.kind !== src.kind) return false;
  if (dst.kind === Kind.Chan && src.dir === 3 && dst.elem === src.elem) return true;
  return dst.underlying === src.underlying;
}

export function native$internal$reflectlite$load(t: Type, p: any): any {
  return load(t, p);
}

export function native$internal$reflectlite$store(t: Type, p: any, x: any): void {
  store(t, p, x);
}

// convert returns x (of type src) as a value of type dst for assignment:
// boxed into an interface, and a copy for aggregates.
export function native$internal$reflectlite$convert(dst: Type, src: Type, x: any): any {
  return dst.kind === Kind.Interface && src.kind !== Kind.Interface ? new Iface(src, copy(src, x)) : copy(src, x);
}

export function native$internal$reflectlite$length(t: Type, x: any): number {
  switch (t.kind) {
    case Kind.Array: return t.len;
    case Kind.Slice: return x === null ? 0 : x.$length;
    case Kind.String: return x.length;
    case Kind.Map: return mapLen(x);
    case Kind.Chan: return chanLen(x);
  }
  return 0;
}

// swapper swaps slice elements by value: aggregates are copied in place, so
// pointers to elements keep pointing at their slots.
export function native$internal$reflectlite$swapper(t: Type, s: Slice<any> | null): (i: number, j: number) => void {
  const e = t.elem!;
  return (i: number, j: number) => {
    const n = s === null ? 0 : s.$length;
    if (i < 0 || i >= n) runtimePanic(`index out of range [${i}] with length ${n}`);
    if (j < 0 || j >= n) runtimePanic(`index out of range [${j}] with length ${n}`);
    const a = s!.$array, o = s!.$offset;
    if (isAggregate(e)) {
      const tmp = copy(e, a[o + i]);
      assign(e, a[o + i], a[o + j]);
      assign(e, a[o + j], tmp);
    } else {
      const tmp = a[o + i];
      a[o + i] = a[o + j];
      a[o + j] = tmp;
    }
  };
}
