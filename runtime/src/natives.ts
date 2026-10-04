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
  GoMap, addressOf, go, GoPanic, Goexit, Iface, Kind, ProgramExit, Slice, Type, assign, chanLen, copy, exitProcess,
  fromJSString, hostNodeFS, ifaceKeyString, implementsIface, isAggregate, load, toJSString, writeConsole, writeStd, writeSyncAll,
  makeSlice, mapLen, numGoroutine, runtimePanic, sizeOf, store, panic, types, append,
  alignOf, arrayElemRef, arrayOf, bytesToString, c64, chanCap, chanOf, close, encodeRune, equal, fieldRef, canonical,
  funcOf, icall, makeChan, makeMap, mapClear, mapDelete, mapLookup, mapOf, mapRange, mapSet, methodKey,
  newPtr, plainPanic, ptrTo, runesToString, select, slice, sliceArray, sliceClear, sliceData,
  sliceElemRef, sliceLit, sliceToArrayPtr, sliceOf, stringToBytes, stringToRunes, getG, setGLSPropagate, ptrAt, topString,
  toPanic, typeArgsName,
} from "./index.ts";
import type { S } from "./index.ts";

// ---- runtime ----

export function native$runtime$Goexit(): never {
  throw new Goexit();
}

export const native$runtime$NumGoroutine = numGoroutine;

export function native$runtime$GetTraceContextFromGLS(): any { return getG().traceContext; }
export function native$runtime$GetBaggageContainerFromGLS(): any { return getG().baggage; }
export function native$runtime$SetTraceContextToGLS(v: any): void { getG().traceContext = v; }
export function native$runtime$SetBaggageContainerToGLS(v: any): void { getG().baggage = v; }
export const native$runtime$setGLSPropagate = setGLSPropagate;

// ---- math and internal/strconv: float bits ----

const scratch = new DataView(new ArrayBuffer(8));

export function native$math$Float64bits(f: number): bigint {
  scratch.setFloat64(0, f);
  return scratch.getBigUint64(0);
}

export function native$math$Float64frombits(b: bigint): number {
  scratch.setBigUint64(0, b);
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

// ---- math: functions with exactly specified results ----
//
// IEEE 754 fixes these results (Sqrt is correctly rounded), so the JS
// builtins return what Go's code would, much faster than its bit
// manipulation on BigInt (natives.Override).

export const native$math$archFloor = Math.floor;
export const native$math$archCeil = Math.ceil;
export const native$math$archTrunc = Math.trunc;
export const native$math$Floor = Math.floor;
export const native$math$Ceil = Math.ceil;
export const native$math$Trunc = Math.trunc;
export const native$math$Sqrt = Math.sqrt;

// Round rounds half away from zero.
export function native$math$Round(x: number): number {
  const t = Math.trunc(x);
  return Math.abs(x - t) >= 0.5 ? t + Math.sign(x) : t;
}

// RoundToEven rounds half to even. (Go's body shifts by a uint difference
// that wraps around, which an int-sized number does not.)
export function native$math$RoundToEven(x: number): number {
  const t = Math.trunc(x);
  const d = Math.abs(x - t);
  return d > 0.5 || (d === 0.5 && t % 2 !== 0) ? t + Math.sign(x) : t;
}

export function native$math$Abs(x: number): number {
  scratch.setFloat64(0, x);
  scratch.setUint8(0, scratch.getUint8(0) & 0x7f);
  return scratch.getFloat64(0);
}

export function native$math$Signbit(x: number): boolean {
  scratch.setFloat64(0, x);
  return (scratch.getUint8(0) & 0x80) !== 0;
}

export function native$math$Copysign(f: number, sign: number): number {
  scratch.setFloat64(0, sign);
  const s = scratch.getUint8(0) & 0x80;
  scratch.setFloat64(0, f);
  scratch.setUint8(0, (scratch.getUint8(0) & 0x7f) | s);
  return scratch.getFloat64(0);
}

export function native$math$Inf(sign: number): number {
  return sign >= 0 ? Infinity : -Infinity;
}

export const native$internal$strconv$float64bits = native$math$Float64bits;
export const native$internal$strconv$float64frombits = native$math$Float64frombits;
export const native$internal$strconv$float32bits = native$math$Float32bits;

// formatBits formats u (negated first if neg) in base, the strconv way:
// appended to dst, or as a string.
export function native$internal$strconv$formatBits(
  dst: S<number>, u: bigint, base: number, neg: boolean, append_: boolean,
): [S<number>, string] {
  if (base < 2 || base === 10 || base > 36) {
    panic(new Iface(types.string, "strconv: illegal AppendInt/FormatInt base"));
  }
  const s = (neg ? "-" + BigInt.asUintN(64, -u).toString(base) : u.toString(base));
  if (!append_) return [null, s];
  const b: number[] = new Array(s.length);
  for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i);
  return [append(dst, b, () => 0), ""];
}
export const native$internal$strconv$float32frombits = native$math$Float32frombits;

// formatDecimal and itoa are strconv's base 10 (patch
// internal/strconv/itoa.go), through String below 2^53 (beyond, it writes
// the shortest digits that read back, not all of them).
export function native$internal$strconv$formatDecimal(u: bigint, neg: boolean): string {
  const s = u < 9007199254740992n ? String(Number(u)) : u.toString();
  return neg ? "-" + s : s;
}

// strings.ToUpper and ToLower of ASCII strings (see the strings patch). A Go
// string holds bytes as UTF-16 code units below 256.
const nonASCIIByte = /[\x80-\xff]/;
export function native$strings$isASCII(s: string): boolean {
  return !nonASCIIByte.test(s);
}
export function native$strings$upperASCII(s: string): string {
  return s.toUpperCase();
}
export function native$strings$lowerASCII(s: string): string {
  return s.toLowerCase();
}

// slices.Sort of integers and strings (see the slices patch). A Go string's
// code units are its bytes, so the default sort's order is Go's; integers
// are sorted as numbers in a typed array.
export function native$slices$sortBuiltin(i: Iface): boolean {
  const t = i.t.elem!, x: S<any> = i.v;
  let tmp: { sort(): unknown; [i: number]: any };
  const n = x === null ? 0 : x.$length;
  switch (t.kind) {
    case Kind.String:
      if (n < 2) return true;
      tmp = (x!.$array as any[]).slice(x!.$offset, x!.$offset + n);
      break;
    case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32:
    case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uintptr:
      if (n < 2) return true;
      if (x!.$array instanceof Uint8Array) {
        x!.$array.subarray(x!.$offset, x!.$offset + n).sort();
        return true;
      }
      tmp = new Float64Array(n);
      break;
    case Kind.Int64:
      if (n < 2) return true;
      tmp = new BigInt64Array(n);
      break;
    case Kind.Uint64:
      if (n < 2) return true;
      tmp = new BigUint64Array(n);
      break;
    default:
      return false;
  }
  const a = x!.$array, off = x!.$offset;
  if (t.kind !== Kind.String) for (let i = 0; i < n; i++) tmp[i] = a[off + i];
  tmp.sort();
  for (let i = 0; i < n; i++) a[off + i] = tmp[i];
  return true;
}

// fmt.Sprintf's fast path (see the fmt patch).
export function native$fmt$indexPercent(s: string, from: number): number {
  return s.indexOf("%", from);
}
// fmtInt and fmtUint format an int or a uint (JS numbers) for the verb
// %d or %v (base 10), %x or %X (base 16), or return "" for other verbs and
// with a precision.
export function native$fmt$fmtInt(v: number, verb: number, prec: number): string {
  const base = prec >= 0 ? 0 : verb === 100 || verb === 118 ? 10 : verb === 120 || verb === 88 ? 16 : 0; // d v x X
  if (base === 0) return "";
  const s = Number.isSafeInteger(v) ? v.toString(base) : BigInt.asIntN(64, BigInt(v)).toString(base);
  return verb === 88 ? s.toUpperCase() : s;
}
export function native$fmt$fmtUint(v: number, verb: number, prec: number): string {
  const base = prec >= 0 ? 0 : verb === 100 || verb === 118 ? 10 : verb === 120 || verb === 88 ? 16 : 0;
  if (base === 0) return "";
  const s = Number.isSafeInteger(v) ? v.toString(base) : BigInt.asUintN(64, BigInt(v)).toString(base);
  return verb === 88 ? s.toUpperCase() : s;
}
// fmtFixed is strconv.FormatFloat(v, 'f', prec, 64) for the values
// ftoaDigits formats with toFixed, and "" for the others.
export function native$fmt$fmtFixed(v: number, prec: number): string {
  const x = Math.abs(v);
  if (v === 0 || !(x < 1e21) || prec > 100) return "";
  let t = x.toFixed(prec);
  if (tieScale(x) === prec) t = roundToEven(t);
  return v < 0 ? "-" + t : t;
}
// fmtShortest is strconv.FormatFloat(v, 'g', -1, 64), fmt's %v of a
// float64: the shortest digits that read back as v (toExponential's), in
// %e form if the exponent is below -4 or at least max(digits, 6)... as
// strconv's fmtEFG decides.
export function native$fmt$fmtShortest(v: number): string {
  if (v === 0) return Object.is(v, -0) ? "-0" : "0";
  if (v !== v) return "NaN";
  if (v === Infinity) return "+Inf";
  if (v === -Infinity) return "-Inf";
  const t = Math.abs(v).toExponential();
  const e = t.indexOf("e");
  const exp = +t.slice(e + 1);
  const d = t.charAt(0) + t.slice(2, e); // the digits
  const nd = d.length, dp = exp + 1;
  let eprec = 6;
  if (eprec > nd && nd >= dp) eprec = nd;
  let r: string;
  if (exp < -4 || exp >= eprec) {
    const ae = Math.abs(exp);
    r = d.charAt(0) + (nd > 1 ? "." + d.slice(1) : "") + (exp < 0 ? "e-" : "e+") + (ae < 10 ? "0" + ae : ae);
  } else if (dp <= 0) {
    r = "0." + "0".repeat(-dp) + d;
  } else if (dp >= nd) {
    r = d + "0".repeat(dp - nd);
  } else {
    r = d.slice(0, dp) + "." + d.slice(dp);
  }
  return v < 0 ? "-" + r : r;
}

// strings.Split with a separator and strings.Join (see the strings patch).
export function native$strings$splitAll(s: string, sep: string): S<string> {
  return sliceLit(s.split(sep));
}
export function native$strings$joinAll(elems: S<string>, sep: string): string {
  if (elems === null || elems.$length === 0) return "";
  const a = elems.$array as string[];
  if (elems.$offset === 0 && elems.$length === a.length) return a.join(sep);
  return a.slice(elems.$offset, elems.$offset + elems.$length).join(sep);
}

// strings.Builder's WriteByte (see the strings patch).
export function native$strings$byteString(c: number): string {
  return String.fromCharCode(c);
}

export function native$internal$strconv$itoa(i: number): string {
  return Number.isSafeInteger(i) ? String(i) : BigInt.asIntN(64, BigInt(i)).toString();
}

// ftoaDigits writes the decimal digits of the float64 val to buf for
// strconv's fmtEFG (patch internal/strconv/ftoa.go): the shortest digits
// that read back as val if prec < 0, else val correctly rounded to prec
// digits for fmt ('e': prec+1 significant digits, 'g': prec, 'f': prec
// after the point). It returns the decimal point position and the number of
// digits without trailing zeros, or ok == false (zero, NaN, ±Inf, or beyond
// toFixed's and toExponential's range), where the Go code formats val.
//
// The engine's Number formatting gives the same digits as Go's ftoa64 (on
// uint64, BigInts under goesm), except on exact ties: it rounds them up,
// Go to even.
export function native$internal$strconv$ftoaDigits(
  buf: S<number>, val: number, fmt: number, prec: number,
): [number, number, boolean] {
  if (val === 0 || !Number.isFinite(val)) return [0, 0, false];
  const x = Math.abs(val);
  let t: string, dp: number;
  if (prec < 0) {
    t = x.toExponential();
    const e = t.indexOf("e");
    dp = +t.slice(e + 1) + 1;
    t = t.slice(0, e);
  } else if (fmt === 102) { // 'f'
    if (prec > 100 || x >= 1e21) return [0, 0, false];
    t = x.toFixed(prec);
    if (tieScale(x) === prec) t = roundToEven(t);
    const p = t.indexOf(".");
    dp = p < 0 ? t.length : p;
  } else {
    const n = fmt === 101 || fmt === 69 ? prec + 1 : Math.max(prec, 1); // 'e', 'E'
    if (n > 101) return [0, 0, false];
    t = x.toExponential(n - 1);
    const e = t.indexOf("e");
    const exp = +t.slice(e + 1);
    t = t.slice(0, e);
    if (tieScale(x) === n - 1 - exp) t = roundToEven(t);
    dp = exp + 1;
  }
  const a = buf!.$array, o = buf!.$offset;
  let nd = 0;
  for (let i = 0; i < t.length; i++) {
    const c = t.charCodeAt(i);
    if (c === 46) continue; // '.'
    if (nd === 0 && c === 48) { dp--; continue; } // leading '0'
    a[o + nd++] = c;
  }
  while (nd > 0 && a[o + nd - 1] === 48) nd--;
  return nd === 0 ? [0, 0, true] : [dp, nd, true];
}

// tieScale returns the s for which x·10^s lies exactly halfway between two
// integers, or NaN if there is none. With x = m·2^e for an odd m, that is
// when 2·m·2^e·10^s is an odd integer: e+s+1 = 0, and 5^-s divides m if
// s < 0.
function tieScale(x: number): number {
  scratch.setFloat64(0, x);
  const hi = scratch.getUint32(0), lo = scratch.getUint32(4);
  const be = (hi >>> 20) & 0x7ff;
  let m = (hi & 0xfffff) * 2 ** 32 + lo + (be === 0 ? 0 : 2 ** 52);
  let e = (be === 0 ? 1 : be) - 1075;
  const tz = lo !== 0 ? 31 - Math.clz32(lo & -lo) : 32 + 31 - Math.clz32(m / 2 ** 32 & -(m / 2 ** 32));
  m /= 2 ** tz;
  e += tz;
  const s = -e - 1;
  return s >= 0 || (s >= -22 && m % 5 ** -s === 0) ? s : NaN;
}

// roundToEven turns the digits t of a tie that the engine rounded up (its
// last digit is odd then, unless the rounding carried) into the ones
// rounded to even.
function roundToEven(t: string): string {
  const c = t.charCodeAt(t.length - 1);
  return (c & 1) === 1 ? t.slice(0, -1) + String.fromCharCode(c - 1) : t;
}

// ---- math/bits (64-bit) ----
//
// The Go code is exact on BigInts too, but works bit by bit or in 32-bit
// halves; these use the two 32-bit halves and the engine's clz32 or BigInt
// multiplication and division directly.

const M64 = (1n << 64n) - 1n;
const lo32 = (x: bigint) => Number(x & 0xffffffffn);
const hi32 = (x: bigint) => Number(x >> 32n);
const ctz32 = (x: number) => 31 - Math.clz32(x & -x);
const pop32 = (x: number) => {
  x -= (x >>> 1) & 0x55555555;
  x = (x & 0x33333333) + ((x >>> 2) & 0x33333333);
  return (Math.imul((x + (x >>> 4)) & 0x0f0f0f0f, 0x01010101) >>> 24);
};
const rev32 = (x: number) => {
  x = ((x >>> 1) & 0x55555555) | ((x & 0x55555555) << 1);
  x = ((x >>> 2) & 0x33333333) | ((x & 0x33333333) << 2);
  x = ((x >>> 4) & 0x0f0f0f0f) | ((x & 0x0f0f0f0f) << 4);
  return revBytes32(x);
};
const revBytes32 = (x: number) => ((x << 24) | ((x & 0xff00) << 8) | ((x >>> 8) & 0xff00) | (x >>> 24)) >>> 0;
const join = (hi: number, lo: number) => (BigInt(hi >>> 0) << 32n) | BigInt(lo >>> 0);

export function native$math$bits$Len64(x: bigint): number {
  const h = hi32(x);
  return h !== 0 ? 64 - Math.clz32(h) : 32 - Math.clz32(lo32(x));
}

export function native$math$bits$LeadingZeros64(x: bigint): number {
  return 64 - native$math$bits$Len64(x);
}

export function native$math$bits$TrailingZeros64(x: bigint): number {
  const l = lo32(x);
  if (l !== 0) return ctz32(l);
  const h = hi32(x);
  return h !== 0 ? 32 + ctz32(h) : 64;
}

export function native$math$bits$OnesCount64(x: bigint): number {
  return pop32(lo32(x)) + pop32(hi32(x));
}

export function native$math$bits$RotateLeft64(x: bigint, k: number): bigint {
  const s = BigInt(k & 63);
  return s === 0n ? x : ((x << s) & M64) | (x >> (64n - s));
}

export function native$math$bits$Reverse64(x: bigint): bigint {
  return join(rev32(lo32(x)), rev32(hi32(x)));
}

export function native$math$bits$ReverseBytes64(x: bigint): bigint {
  return join(revBytes32(lo32(x)), revBytes32(hi32(x)));
}

export function native$math$bits$Add64(x: bigint, y: bigint, carry: bigint): [bigint, bigint] {
  const s = x + y + carry;
  return [s & M64, s >> 64n];
}

export function native$math$bits$Sub64(x: bigint, y: bigint, borrow: bigint): [bigint, bigint] {
  const d = x - y - borrow;
  return [d & M64, d < 0n ? 1n : 0n];
}

export function native$math$bits$Mul64(x: bigint, y: bigint): [bigint, bigint] {
  const p = x * y;
  return [p >> 64n, p & M64];
}

export function native$math$bits$Div64(hi: bigint, lo: bigint, y: bigint): [bigint, bigint] {
  if (y === 0n) runtimePanic("integer divide by zero");
  if (y <= hi) runtimePanic("integer overflow");
  const n = (hi << 64n) | lo;
  return [n / y, n % y];
}

export function native$math$bits$Rem64(hi: bigint, lo: bigint, y: bigint): bigint {
  if (y === 0n) runtimePanic("integer divide by zero");
  return ((hi << 64n) | lo) % y;
}

// ---- math/bits (32-bit) ----
//
// Exact on numbers: sums stay below 2^34, and products and dividends are
// taken in 16-bit halves (Hacker's Delight mulhu, divlu).

export function native$math$bits$Add32(x: number, y: number, carry: number): [number, number] {
  const s = x + y + carry;
  return [s >>> 0, s > 0xffffffff ? 1 : 0];
}

export function native$math$bits$Sub32(x: number, y: number, borrow: number): [number, number] {
  const d = x - y - borrow;
  return [d >>> 0, d < 0 ? 1 : 0];
}

export function native$math$bits$Mul32(x: number, y: number): [number, number] {
  const xl = x & 0xffff, xh = x >>> 16, yl = y & 0xffff, yh = y >>> 16;
  const t = xh * yl + ((xl * yl) >>> 16);
  const w = (t & 0xffff) + xl * yh;
  return [xh * yh + (t >>> 16) + (w >>> 16), Math.imul(x, y) >>> 0];
}

function div32(hi: number, lo: number, y: number): [number, number] {
  // hi < y, so each 16-bit step's dividend is below 2^48 and its quotient
  // digit below 2^16.
  const n1 = hi * 0x10000 + (lo >>> 16);
  const q1 = Math.floor(n1 / y);
  const n0 = (n1 - q1 * y) * 0x10000 + (lo & 0xffff);
  const q0 = Math.floor(n0 / y);
  return [q1 * 0x10000 + q0, n0 - q0 * y];
}

export function native$math$bits$Div32(hi: number, lo: number, y: number): [number, number] {
  if (y === 0) runtimePanic("integer divide by zero");
  if (y <= hi) runtimePanic("integer overflow");
  return div32(hi, lo, y);
}

export function native$math$bits$Rem32(hi: number, lo: number, y: number): number {
  if (y === 0) runtimePanic("integer divide by zero");
  return div32(hi % y, lo, y)[1];
}

// ---- multi-precision arithmetic ----
//
// The inner loops of math/big's and crypto/internal/fips140/bigmod's
// multiplications, on 32-bit words (both use 32-bit words under goesm).
// Their Go bodies go through bits.Mul32 and bits.Add32, whose results are
// tuples; here each word product is taken as two exact float products.

const B32 = 4294967296;

// mulAdd32 sets z[i] = x[i]*y + a[i] + carry for i < n (a may be null for
// zeros) and returns the final carry. a[i] and x[i] are read before z[i] is
// written, so z may alias them.
function mulAdd32(
  za: number[], zo: number, n: number, xa: number[], xo: number, y: number,
  aa: number[] | null, ao: number, c: number,
): number {
  const yl = y & 0xffff, yh = y >>> 16;
  for (let i = 0; i < n; i++) {
    const xi = xa[xo + i];
    // x*y = b*2^16 + a with a, b < 2^48; s < 2^49: all exact.
    const a = xi * yl, b = xi * yh, bl = b & 0xffff;
    const s = a + bl * 0x10000 + (aa === null ? 0 : aa[ao + i]) + c;
    const lo = s >>> 0;
    za[zo + i] = lo;
    c = (b - bl) / 0x10000 + (s - lo) / B32;
  }
  return c;
}

const sliceLen = (s: S<number>) => (s === null ? 0 : s.$length);

// math/big.mulAddVWW_g(z, x []Word, y, r Word) (c Word): z = x*y + r.
export function native$math$big$mulAddVWW_g(z: S<number>, x: S<number>, y: number, r: number): number {
  const n = sliceLen(z);
  if (sliceLen(x) !== n) panic(new Iface(types.string, "mulAddVWW len"));
  if (n === 0) return r;
  return mulAdd32(z!.$array, z!.$offset, n, x!.$array, x!.$offset, y, null, 0, r);
}

// math/big.addMulVVWW_g(z, x, y []Word, m, a Word) (c Word): z = x + y*m + a.
export function native$math$big$addMulVVWW_g(z: S<number>, x: S<number>, y: S<number>, m: number, a: number): number {
  const n = sliceLen(z);
  if (sliceLen(x) !== n || sliceLen(y) !== n) panic(new Iface(types.string, "addMulVVWW len"));
  if (n === 0) return a;
  return mulAdd32(z!.$array, z!.$offset, n, y!.$array, y!.$offset, m, x!.$array, x!.$offset, a);
}

// crypto/internal/fips140/bigmod.addMulVVW(z, x []uint32, y uint32)
// (carry uint32): z += x*y.
export function native$crypto$internal$fips140$bigmod$addMulVVW(z: S<number>, x: S<number>, y: number): number {
  const n = sliceLen(z);
  if (n - 1 < 0 || n - 1 >= sliceLen(x)) runtimePanic(n - 1 < 0 ? `index out of range [${n - 1}]` : `index out of range [${n - 1}] with length ${sliceLen(x)}`);
  return mulAdd32(z!.$array, z!.$offset, n, x!.$array, x!.$offset, y, z!.$array, z!.$offset, 0);
}

// ---- maps, slices ----

// maps.clone(m any) any: a shallow copy (keys and values are assigned, so
// aggregates are copied).
export function native$maps$clone(m: Iface | null): Iface | null {
  if (m === null || m.v === null) return m;
  const src = m.v as GoMap<any, any>, t = m.t;
  const dst = new GoMap<any, any>(src.keyType);
  for (const [k, v] of mapRange(src)) mapSet(dst, copy(t.key!, k), copy(t.elem!, v));
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
const add = (w: (x: any) => any) => (p: any, d: any) => (p.v = w(p.v + d));
const and = (w: (x: any) => any) => (p: any, m: any) => { const old = p.v; p.v = w(typeof old === "bigint" ? old & m : Number(BigInt(old) & BigInt(m))); return old; };
const or = (w: (x: any) => any) => (p: any, m: any) => { const old = p.v; p.v = w(typeof old === "bigint" ? old | m : Number(BigInt(old) | BigInt(m))); return old; };
const i32 = (x: number) => x | 0, u32 = (x: number) => x >>> 0, n64 = (x: number) => x;
const i64 = (x: bigint) => BigInt.asIntN(64, x), u64 = (x: bigint) => BigInt.asUintN(64, x);

export function native$sync$atomic$sameType(x: Iface, y: Iface): boolean {
  return x.t === y.t;
}
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
  native$sync$atomic$AddInt64 = add(i64), native$sync$atomic$AddUint64 = add(u64), native$sync$atomic$AddUintptr = add(n64);
export const native$sync$atomic$AndInt32 = and(i32), native$sync$atomic$AndUint32 = and(u32),
  native$sync$atomic$AndInt64 = and(i64), native$sync$atomic$AndUint64 = and(u64), native$sync$atomic$AndUintptr = and(n64);
export const native$sync$atomic$OrInt32 = or(i32), native$sync$atomic$OrUint32 = or(u32),
  native$sync$atomic$OrInt64 = or(i64), native$sync$atomic$OrUint64 = or(u64), native$sync$atomic$OrUintptr = or(n64);

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
  if (t.named) return typeArgsName(t);
  if (t.kind === Kind.UnsafePointer) return "Pointer";
  // Predeclared types are named; composite literal types are not.
  return t.kind <= Kind.Complex128 || t.kind === Kind.String ? t.str : "";
}

export function native$internal$reflectlite$typePkgPath(t: Type): string {
  if (t.kind === Kind.UnsafePointer) return "unsafe";
  return t.named ? t.pkgPath : "";
}

export const native$internal$reflectlite$typeSize = sizeOf;

export function native$internal$reflectlite$typeKind(t: Type): number {
  return t.kind;
}

export function native$internal$reflectlite$typeString(t: Type): string {
  return topString(t);
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

// ---- reflect ----
//
// Like reflectlite: a reflect *rtype is a runtime type descriptor and a
// Value's ptr is the JS value itself, or (flagAddr) a goesm pointer to it.
// int64 and uint64 cross as bigint, the other integers as numbers.

export const native$reflect$ifaceType = native$internal$reflectlite$ifaceType;
export const native$reflect$ifaceValue = native$internal$reflectlite$ifaceValue;
export const native$reflect$asIface = native$internal$reflectlite$asIface;
export const native$reflect$typeName = native$internal$reflectlite$typeName;
export const native$reflect$typePkgPath = native$internal$reflectlite$typePkgPath;
export const native$reflect$typeSize = sizeOf;
export const native$reflect$typeAlign = alignOf;
export const native$reflect$typeKind = native$internal$reflectlite$typeKind;
export const native$reflect$typeString = native$internal$reflectlite$typeString;
export const native$reflect$typeComparable = native$internal$reflectlite$typeComparable;
export const native$reflect$typeElem = native$internal$reflectlite$typeElem;
export const native$reflect$implements = native$internal$reflectlite$implements;
export const native$reflect$directlyAssignable = native$internal$reflectlite$directlyAssignable;
export const native$reflect$load = native$internal$reflectlite$load;
export const native$reflect$store = native$internal$reflectlite$store;
export const native$reflect$assignConvert = native$internal$reflectlite$convert;
export const native$reflect$length = native$internal$reflectlite$length;
export const native$reflect$swapper = native$internal$reflectlite$swapper;
export const native$reflect$copyValue = copy;
export const native$reflect$equal = equal;
export const native$reflect$newPtr = newPtr;

export function native$reflect$box(t: Type, x: any): Iface | null {
  return t.kind === Kind.Interface ? x : new Iface(t, copy(t, x));
}

export function native$reflect$typeKey(t: Type): Type | null { return t.key; }
export function native$reflect$typeLen(t: Type): number { return t.len; }
export function native$reflect$typeNumField(t: Type): number { return t.fields.length; }
export function native$reflect$typeNumIn(t: Type): number { return t.params.length; }
export function native$reflect$typeIn(t: Type, i: number): Type { return t.params[i]; }
export function native$reflect$typeNumOut(t: Type): number { return t.results.length; }
export function native$reflect$typeOut(t: Type, i: number): Type { return t.results[i]; }
export function native$reflect$typeVariadic(t: Type): boolean { return t.variadic; }

// goesm numbers channel directions send 1, receive 2; reflect the reverse.
function chanDir(d: number): number {
  return d === 1 ? 2 : d === 2 ? 1 : d;
}

export function native$reflect$typeChanDir(t: Type): number { return chanDir(t.dir); }

export function native$reflect$typeField(t: Type, i: number): [string, string, Type, string, boolean, number] {
  let off = 0;
  for (let j = 0; j < i; j++) {
    const ft = t.fields[j].type;
    const a = alignOf(ft);
    off = Math.ceil(off / a) * a + sizeOf(ft);
  }
  const f = t.fields[i];
  const a = alignOf(f.type);
  off = Math.ceil(off / a) * a;
  return [f.name, f.pkgPath, f.type, f.tag, f.embedded, off];
}

interface ReflectMethod { name: string; pkgPath: string; type: Type; key: string }

const methodLists = new WeakMap<Type, ReflectMethod[]>();

// methodList is t's methods in reflect's order (by name): an interface's
// methods, or the exported methods of t's method set.
function methodList(t: Type): ReflectMethod[] {
  let ms = methodLists.get(t);
  if (ms === undefined) {
    ms = [];
    if (t.kind === Kind.Interface) {
      for (const m of t.imethods) ms.push({ name: m.name, pkgPath: m.pkgPath, type: m.type, key: methodKey(m.name, m.pkgPath) });
    } else {
      for (const [key, impl] of t.methods) {
        if (!key.includes(".")) ms.push({ name: key, pkgPath: "", type: impl.type, key });
      }
    }
    ms.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
    methodLists.set(t, ms);
  }
  return ms;
}

export function native$reflect$typeNumMethod(t: Type): number { return methodList(t).length; }

export function native$reflect$typeMethod(t: Type, i: number): [string, string, Type] {
  const m = methodList(t)[i];
  return [m.name, m.pkgPath, m.type];
}

export function native$reflect$methodValue(t: Type, x: any, i: number): (...a: any[]) => any {
  const m = methodList(t)[i];
  if (t.kind === Kind.Interface) return (...a: any[]) => icall(x, m.key, ...a);
  const fn = t.methods.get(m.key)!.fn;
  const recv = copy(t, x);
  return (...a: any[]) => fn(recv, ...a);
}

export function native$reflect$methodExpr(t: Type, i: number): (r: any, ...a: any[]) => any {
  const fn = t.methods.get(methodList(t)[i].key)!.fn;
  return (r: any, ...a: any[]) => fn(r, ...a);
}

const isIntKind = (k: number) => k >= Kind.Int && k <= Kind.Uintptr;
const isFloatKind = (k: number) => k === Kind.Float32 || k === Kind.Float64;
const isComplexKind = (k: number) => k === Kind.Complex64 || k === Kind.Complex128;

export function native$reflect$convertible(dst: Type, src: Type): boolean {
  if (native$internal$reflectlite$directlyAssignable(dst, src) || native$internal$reflectlite$implements(dst, src)) return true;
  const dk = dst.kind, sk = src.kind;
  if ((isIntKind(dk) || isFloatKind(dk)) && (isIntKind(sk) || isFloatKind(sk))) return true;
  if (isComplexKind(dk) && isComplexKind(sk)) return true;
  const plainElem = (t: Type) => !t.elem!.named || t.elem!.pkgPath === "";
  if (dk === Kind.String) {
    if (isIntKind(sk)) return true;
    if (sk === Kind.Slice && plainElem(src) && (src.elem!.kind === Kind.Uint8 || src.elem!.kind === Kind.Int32)) return true;
  }
  if (sk === Kind.String && dk === Kind.Slice && plainElem(dst) && (dst.elem!.kind === Kind.Uint8 || dst.elem!.kind === Kind.Int32)) return true;
  if (sk === Kind.Slice && dk === Kind.Array && dst.elem === src.elem) return true;
  if (sk === Kind.Slice && dk === Kind.Pointer && dst.elem!.kind === Kind.Array && dst.elem!.elem === src.elem) return true;
  if (dst.underlying === src.underlying) return true;
  if (dk === Kind.Pointer && sk === Kind.Pointer && !dst.named && !src.named && dst.elem!.underlying === src.elem!.underlying) return true;
  return false;
}

export function native$reflect$convertValue(dst: Type, src: Type, x: any): any {
  const dk = dst.kind, sk = src.kind;
  if (dk === Kind.Interface) return sk === Kind.Interface ? x : new Iface(src, copy(src, x));
  if (isIntKind(dk) && isIntKind(sk)) return intOf(dst, BigInt(x));
  if (isIntKind(dk) && isFloatKind(sk)) return intOf(dst, floatToBigInt(x));
  if (isFloatKind(dk) && (isIntKind(sk) || isFloatKind(sk))) {
    const f = Number(x);
    return dk === Kind.Float32 ? Math.fround(f) : f;
  }
  if (isComplexKind(dk) && isComplexKind(sk)) return dk === Kind.Complex64 ? c64(x) : x;
  if (dk === Kind.String) {
    if (isIntKind(sk)) {
      const r = BigInt(x);
      return encodeRune(r < 0n || r > 0x10ffffn ? 0xfffd : Number(r));
    }
    if (sk === Kind.Slice) return src.elem!.kind === Kind.Int32 ? runesToString(x) : bytesToString(x);
    return x;
  }
  if (dk === Kind.Slice && sk === Kind.String) return dst.elem!.kind === Kind.Int32 ? stringToRunes(x) : stringToBytes(x);
  if (dk === Kind.Array && sk === Kind.Slice) {
    const a = new Array(dst.len);
    for (let i = 0; i < dst.len; i++) a[i] = copy(dst.elem!, x.$array[x.$offset + i]);
    return a;
  }
  if (dk === Kind.Pointer && sk === Kind.Slice) {
    if (x === null) return null;
    if (x.$array instanceof Uint8Array && x.$length >= dst.elem!.len) return sliceToArrayPtr(x, dst.elem!.len);
    if (x.$offset === 0 && x.$array.length === dst.elem!.len) return x.$array;
    plainPanic("reflect: converting a slice to an array pointer that does not share its whole backing array is not supported by goesm");
  }
  return copy(src, x);
}

// intOf wraps the integer x to the width of integer type t.
function intOf(t: Type, x: bigint): any {
  switch (t.kind) {
    case Kind.Int64: return BigInt.asIntN(64, x);
    case Kind.Uint64: return BigInt.asUintN(64, x);
    case Kind.Int: return Number(BigInt.asIntN(64, x));
    case Kind.Uint: case Kind.Uintptr: return Number(BigInt.asUintN(64, x));
    case Kind.Int32: return Number(BigInt.asIntN(32, x));
    case Kind.Int16: return Number(BigInt.asIntN(16, x));
    case Kind.Int8: return Number(BigInt.asIntN(8, x));
    case Kind.Uint32: return Number(BigInt.asUintN(32, x));
    case Kind.Uint16: return Number(BigInt.asUintN(16, x));
    case Kind.Uint8: return Number(BigInt.asUintN(8, x));
  }
  return Number(x);
}

function floatToBigInt(f: number): bigint {
  return Number.isFinite(f) ? BigInt(Math.trunc(f)) : 0n;
}

export function native$reflect$valueInt(_t: Type, x: any): bigint { return BigInt(x); }
export function native$reflect$valueUint(_t: Type, x: any): bigint { return BigInt(x); }
export function native$reflect$makeInt(t: Type, x: bigint): any { return intOf(t, x); }
export function native$reflect$makeUint(t: Type, x: bigint): any { return intOf(t, x); }

function identity<T>(x: T): T { return x; }
export const native$reflect$asBool = identity;
export const native$reflect$asFloat = identity;
export const native$reflect$asComplex = identity;
export const native$reflect$asString = identity;
export const native$reflect$asBytes = identity;
export const native$reflect$fromBool = identity;
export const native$reflect$fromFloat = identity;
export const native$reflect$fromComplex = identity;
export const native$reflect$fromString = identity;
export const native$reflect$fromBytes = identity;
export const native$reflect$fromUint8 = identity;

export function native$reflect$zero(t: Type): any { return t.zero(); }

export function native$reflect$isNil(x: any): boolean { return x === null || x === undefined; }

export function native$reflect$capacity(t: Type, x: any): number {
  if (t.kind === Kind.Chan) return chanCap(x);
  return x === null ? 0 : x.$capacity;
}

export function native$reflect$fieldAddr(t: Type, p: any, i: number): any {
  const f = t.fields[i];
  if (p === null) runtimePanic("invalid memory address or nil pointer dereference");
  return isAggregate(f.type) ? p[f.prop] : fieldRef(p, f.prop);
}

export function native$reflect$fieldValue(t: Type, x: any, i: number): any {
  return x[t.fields[i].prop];
}

export function native$reflect$elemAddr(t: Type, x: any, i: number): any {
  const agg = isAggregate(t.elem!);
  if (t.kind === Kind.Array) return agg ? x[i] : arrayElemRef(x, i);
  return agg ? x.$array[x.$offset + i] : sliceElemRef(x, i);
}

// canonPtr makes an addressable Value's pointer (fieldAddr, elemAddr) a Go
// pointer value with the identity &x has.
export const native$reflect$canonPtr = canonical;

export function native$reflect$elemValue(_t: Type, x: any, i: number): any { return x[i]; }

export function native$reflect$sliceValue(t: Type, x: any, i: number, j: number, k: number): any {
  return t.kind === Kind.Array ? sliceArray(x, i, j, k) : slice(x, i, j, k);
}

export function native$reflect$sameSlice(x: any, y: any): boolean {
  return x === y || (x !== null && y !== null && x.$array === y.$array && x.$offset === y.$offset);
}

export function native$reflect$makeSlice(t: Type, n: number, c: number): any {
  return makeSlice(n, c, t.elem!.zero);
}

export function native$reflect$appendValue(t: Type, s: any, x: any): any {
  return append(s, [x], t.elem!.zero, t.elem!);
}

export function native$reflect$grow(t: Type, s: any, n: number): any {
  const len = s === null ? 0 : s.$length;
  const zeros = new Array(n);
  for (let i = 0; i < n; i++) zeros[i] = t.elem!.zero();
  const r = append(s, zeros, t.elem!.zero, t.elem!);
  return slice(r, 0, len, r!.$capacity);
}

export function native$reflect$clearValue(t: Type, x: any): void {
  if (t.kind === Kind.Map) mapClear(x);
  else sliceClear(x, t.elem!.zero, t.elem!);
}

export function native$reflect$makeMap(t: Type): any { return makeMap(t.key!); }

export function native$reflect$mapIndex(m: any, k: any): [any, boolean] {
  return mapLookup(m, k, () => undefined);
}

export function native$reflect$mapSet(m: any, k: any, x: any): void { mapSet(m, k, x); }
export function native$reflect$mapDelete(m: any, k: any): void { mapDelete(m, k); }

export function native$reflect$mapKeys(m: any): S<any> {
  const keys: any[] = [];
  for (const [k] of mapRange(m)) keys.push(k);
  return keys.length === 0 ? null : sliceLit(keys);
}

export function native$reflect$mapIter(m: any): any { return mapRange(m); }

export function native$reflect$mapNext(it: Generator<[any, any]>): [any, any, boolean] {
  const r = it.next();
  return r.done ? [null, null, false] : [r.value[0], r.value[1], true];
}

export function native$reflect$makeChan(t: Type, n: number): any { return makeChan(n, t.elem!.zero); }

export function native$reflect$trySend(ch: any, x: any): boolean {
  return select([[ch, true, x]], true)[0] === 0;
}

export function native$reflect$tryRecv(ch: any): [any, boolean, boolean] {
  const [i, v, ok] = select([[ch, false, undefined]], true);
  return i === 0 ? [v, ok, true] : [undefined, false, false];
}

export function native$reflect$chanClose(ch: any): void { close(ch); }

export function native$reflect$callFunc(t: Type, fn: (...a: any[]) => any, args: S<any>): S<any> {
  const a = args === null ? [] : args.$array.slice(args.$offset, args.$offset + args.$length);
  const r = fn(...a);
  if (r instanceof Promise) {
    plainPanic("reflect: Call of a function that blocks is not supported by goesm");
  }
  const n = t.results.length;
  return n === 0 ? null : sliceLit(n === 1 ? [r] : r);
}

export function native$reflect$makeFunc(t: Type, impl: (args: S<any>) => S<any>): (...a: any[]) => any {
  const n = t.results.length;
  return (...a: any[]) => {
    const r = impl(sliceLit(a));
    if (n === 0) return undefined;
    const out = r!.$array.slice(r!.$offset, r!.$offset + r!.$length);
    return n === 1 ? out[0] : out;
  };
}

// pointerID stands in for an address: a stable number per object.
export function native$reflect$pointerID(t: Type, x: any): number {
  if (t.kind === Kind.Slice) {
    if (x === null) return 0;
    return addressOf(x.$array) + x.$offset * sizeOf(t.elem!);
  }
  return addressOf(x);
}

export function native$reflect$unsafePointer(t: Type, x: any): any {
  if (t.kind === Kind.Slice) return sliceData(x, isAggregate(t.elem!));
  return x;
}

export const native$reflect$pointerAt = ptrAt;
export function native$reflect$ptrTo(t: Type): Type { return ptrTo(t); }
export function native$reflect$sliceOf(t: Type): Type { return sliceOf(t); }
export function native$reflect$mapOf(k: Type, e: Type): Type { return mapOf(k, e); }
export function native$reflect$arrayOf(n: number, e: Type): Type { return arrayOf(e, n); }
export function native$reflect$chanOf(dir: number, e: Type): Type { return chanOf(e, chanDir(dir)); }

export function native$reflect$funcOf(ins: S<Type>, outs: S<Type>, variadic: boolean): Type {
  const arr = (s: S<Type>) => (s === null ? [] : s.$array.slice(s.$offset, s.$offset + s.$length));
  return funcOf(arr(ins), arr(outs), variadic);
}

// ---- syscall/js ----
//
// goesm's syscall/js (internal/natives/goroot/syscall/js) holds JavaScript
// values as they are. Go's nil (the zero Value) is undefined, so JavaScript
// null needs a sentinel.

const jsNull = { toString: () => "null" };

function toRef(x: unknown): any {
  return x === undefined ? null : x === null ? jsNull : x;
}

function fromRef(r: any): any {
  return r === null ? undefined : r === jsNull ? null : r;
}

function refArgs(args: S<any>): any[] {
  const out: any[] = [];
  if (args !== null) for (let i = 0; i < args.$length; i++) out.push(fromRef(args.$array[args.$offset + i]));
  return out;
}

// A JavaScript exception becomes a js.Error; a Go panic or Goexit raised by
// Go code called back from JavaScript keeps unwinding.
function jsThrown(e: unknown): [any, boolean] {
  if (e instanceof GoPanic || e instanceof Goexit || e instanceof ProgramExit) throw e;
  return [toRef(e), false];
}

export function native$syscall$js$nullRef(): any {
  return jsNull;
}

export function native$syscall$js$globalRef(): any {
  return globalThis;
}

export function native$syscall$js$boolVal(b: boolean): any {
  return b;
}

export function native$syscall$js$floatVal(f: number): any {
  return f;
}

export function native$syscall$js$stringVal(s: string): any {
  return toJSString(s);
}

export function native$syscall$js$valueEqual(v: any, w: any): boolean {
  return fromRef(v) === fromRef(w);
}

export function native$syscall$js$valueIsNaN(v: any): boolean {
  return typeof v === "number" && v !== v;
}

export function native$syscall$js$valueType(v: any): number {
  const x = fromRef(v);
  if (x === undefined) return 0;
  if (x === null) return 1;
  switch (typeof x) {
    case "boolean": return 2;
    case "number": return 3;
    case "string": return 4;
    case "symbol": return 5;
    case "function": return 7;
  }
  return 6; // objects and bigints, as in wasm_exec.js
}

export function native$syscall$js$valueGet(v: any, p: string): any {
  const o = fromRef(v);
  const name = toJSString(p);
  if (o === globalThis) {
    if (name === "fs") return hostFS();
    if (name === "process" && (globalThis as any).process === undefined) return hostProcess();
    if (name === "path" && (globalThis as any).path === undefined) return hostPath();
  }
  return toRef(Reflect.get(o, name));
}

export function native$syscall$js$valueSet(v: any, p: string, x: any): void {
  Reflect.set(fromRef(v), toJSString(p), fromRef(x));
}

export function native$syscall$js$valueDelete(v: any, p: string): void {
  Reflect.deleteProperty(fromRef(v), toJSString(p));
}

export function native$syscall$js$valueIndex(v: any, i: number): any {
  return toRef(Reflect.get(fromRef(v), i));
}

export function native$syscall$js$valueSetIndex(v: any, i: number, x: any): void {
  Reflect.set(fromRef(v), i, fromRef(x));
}

export function native$syscall$js$valueLength(v: any): number {
  return Number(fromRef(v).length);
}

export function native$syscall$js$valueCall(v: any, m: string, args: S<any>): [any, boolean] {
  try {
    const o = fromRef(v);
    const f = native$syscall$js$valueGet(v, m);
    return [toRef(Reflect.apply(fromRef(f), o, refArgs(args))), true];
  } catch (e) {
    return jsThrown(e);
  }
}

export function native$syscall$js$valueInvoke(v: any, args: S<any>): [any, boolean] {
  try {
    return [toRef(Reflect.apply(fromRef(v), undefined, refArgs(args))), true];
  } catch (e) {
    return jsThrown(e);
  }
}

export function native$syscall$js$valueNew(v: any, args: S<any>): [any, boolean] {
  try {
    return [toRef(Reflect.construct(fromRef(v), refArgs(args))), true];
  } catch (e) {
    return jsThrown(e);
  }
}

export function native$syscall$js$valueFloat(v: any): number {
  return v;
}

export function native$syscall$js$valueTruthy(v: any): boolean {
  return !!fromRef(v);
}

export function native$syscall$js$valueString(v: any): string {
  return fromJSString(String(fromRef(v)));
}

export function native$syscall$js$valueInstanceOf(v: any, t: any): boolean {
  return fromRef(v) instanceof fromRef(t);
}

function isByteArray(x: unknown): x is Uint8Array | Uint8ClampedArray {
  return x instanceof Uint8Array || x instanceof Uint8ClampedArray;
}

export function native$syscall$js$copyBytesToGo(dst: S<number>, src: any): [number, boolean] {
  const a = fromRef(src);
  if (!isByteArray(a)) return [0, false];
  const n = Math.min(dst === null ? 0 : dst.$length, a.length);
  for (let i = 0; i < n; i++) dst!.$array[dst!.$offset + i] = a[i];
  return [n, true];
}

export function native$syscall$js$copyBytesToJS(dst: any, src: S<number>): [number, boolean] {
  const a = fromRef(dst);
  if (!isByteArray(a)) return [0, false];
  const n = Math.min(a.length, src === null ? 0 : src.$length);
  for (let i = 0; i < n; i++) a[i] = src!.$array[src!.$offset + i];
  return [n, true];
}

export function native$syscall$js$makeFunc(fn: (self: any, args: S<any>) => any): any {
  return function (this: any, ...args: any[]): any {
    const a = args.map(toRef);
    let r;
    try {
      r = fn(toRef(this), new Slice(a, 0, a.length, a.length));
    } catch (e) {
      throw toPanic(e); // a nil dereference is a TypeError until here
    }
    // A Go function that blocks was lowered to an async function.
    return r instanceof Promise ? r.then(fromRef, (e) => { throw toPanic(e); }) : fromRef(r);
  };
}

// ---- the host's fs and process, seen through js.Global() ----
//
// Package syscall reaches files through js.Global().Get("fs") with Node's
// callback API, like Go's wasm_exec.js expects. goesm always supplies its own
// fs there, without touching globalThis, even if the host has a global fs
// (as wasm_exec.js users set up): it calls back before returning, which lets
// goesm lower syscall.fsCall as synchronous (internal/natives: syncFuncs), so
// writing to os.Stdout does not make callers async. A host fs with Node's
// asynchronous callbacks would break that.

function enosys(): Error {
  const err = new Error("not implemented");
  (err as any).code = "ENOSYS";
  return err;
}

const fsCalls = [
  "open", "close", "read", "write", "fstat", "stat", "lstat", "readdir", "mkdir", "unlink", "rmdir",
  "chmod", "fchmod", "chown", "fchown", "lchown", "utimes", "rename", "truncate", "ftruncate",
  "readlink", "link", "symlink", "fsync",
];

let theFS: any = null;

// hostPath is node:path for syscall's jsPath.resolve, as wasm_exec_node.js
// provides it; without one (browsers) paths stay as they are.
function hostPath(): any {
  return (globalThis as any).process?.getBuiltinModule?.("path") ?? { resolve: (p: string) => p };
}

function hostFS(): any {
  if (theFS === null) {
    const fs = hostNodeFS();
    theFS = fs ? nodeFS(fs) : consoleFS();
  }
  return theFS;
}

// nodeFS adapts the synchronous API of node:fs (Node, Bun, Deno) to the
// callback API.
function nodeFS(fs: any): any {
  const shim: any = { constants: fs.constants };
  for (const name of fsCalls) {
    shim[name] = (...args: any[]) => {
      const cb = args.pop();
      let r: any;
      try {
        r = name === "write" ? writeSyncAll(fs, args[0], args[1], args[2], args[3], args[4]) : fs[name + "Sync"](...args);
      } catch (e) {
        cb(e);
        return;
      }
      cb(null, r);
    };
  }
  return shim;
}

// consoleFS is a browser's file system: as in wasm_exec.js, standard output
// and standard error go to the console line by line, and everything else
// fails with ENOSYS.
function consoleFS(): any {
  const shim: any = {
    constants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1, O_DIRECTORY: -1 },
    write(fd: number, buf: Uint8Array, off: number, len: number, pos: number | null, cb: (err: any, n?: number) => void) {
      if ((fd !== 1 && fd !== 2) || pos !== null) {
        cb(enosys());
        return;
      }
      writeConsole(fd, buf.subarray(off, off + len));
      cb(null, len);
    },
  };
  for (const name of fsCalls) {
    if (!(name in shim)) shim[name] = (...args: any[]) => args[args.length - 1](enosys());
  }
  return shim;
}

let theProcess: any = null;

function hostProcess(): any {
  theProcess ??= {
    getuid: () => -1, getgid: () => -1, geteuid: () => -1, getegid: () => -1,
    getgroups: () => { throw enosys(); },
    pid: -1, ppid: -1,
    umask: () => { throw enosys(); },
    cwd: () => { throw enosys(); },
    chdir: () => { throw enosys(); },
  };
  return theProcess;
}

// ---- syscall and os: the process ----

const gproc = (globalThis as any).process;

function goStrings(xs: string[]): Slice<string> {
  const a = xs.map(fromJSString);
  return new Slice(a, 0, a.length, a.length);
}

// The environment is the host's at start-up; package syscall keeps its own
// copy, so Setenv does not change the host's.
export function native$syscall$runtime_envs(): Slice<string> {
  const env = gproc?.env;
  return goStrings(env ? Object.keys(env).map((k) => k + "=" + env[k]) : []);
}

export function native$syscall$runtimeSetenv(_k: string, _v: string): void {}
export function native$syscall$runtimeUnsetenv(_k: string): void {}
export function native$syscall$runtimeClearenv(): void {}

// ---- internal/godebug and runtime/debug ----

// GODEBUG settings come from the host's environment at start-up; goesm has
// no default GODEBUG of its own, and no runtime metrics.
export function native$internal$godebug$writeStderr(b: Slice<number> | null): void {
  if (b === null) return;
  let s = "";
  for (let i = 0; i < b.$length; i++) s += String.fromCharCode(b.$array[b.$offset + i]);
  writeStd(2, s);
}
// crypto/internal/fips140's service indicator and crypto/fips140's bypass
// flag are per-goroutine in gc; goroutines never run in parallel here, and
// both are only set and read within one synchronous call.
let fipsIndicator = 0, fipsBypass = false;
export function native$crypto$internal$fips140$getIndicator(): number { return fipsIndicator; }
export function native$crypto$internal$fips140$setIndicator(x: number): void { fipsIndicator = x; }
export function native$crypto$internal$fips140$fatal(msg: string): never {
  writeStd(2, "fatal error: " + msg + "\n");
  return exitProcess(2);
}
export function native$crypto$internal$fips140$alias$AnyOverlap(x: Slice<number> | null, y: Slice<number> | null): boolean {
  return x !== null && y !== null && x.$length > 0 && y.$length > 0 && x.$array === y.$array &&
    x.$offset < y.$offset + y.$length && y.$offset < x.$offset + x.$length;
}
// weak pointers: a weak handle per pointer (weak.Make(p) == weak.Make(p)),
// holding a WeakRef where the host has one.
const weakHandles = new WeakMap<object, { ref: { deref(): any } }>();
export function native$weak$runtime_registerWeakPointer(p: any): any {
  let h = weakHandles.get(p);
  if (h === undefined) {
    const W = (globalThis as any).WeakRef;
    h = { ref: W ? new W(p) : { deref: () => p } };
    weakHandles.set(p, h);
  }
  return h;
}
export function native$weak$runtime_makeStrongFromWeak(h: any): any {
  return h.ref.deref() ?? null;
}

// crypto/internal/boring/sig's markers are empty assembly functions that
// only label the binary.
export function native$crypto$internal$boring$sig$BoringCrypto(): void {}
export function native$crypto$internal$boring$sig$FIPSOnly(): void {}
export function native$crypto$internal$boring$sig$StandardCrypto(): void {}
export function native$crypto$fips140$setBypass(): void { fipsBypass = true; }
export function native$crypto$fips140$isBypassed(): boolean { return fipsBypass; }
export function native$crypto$fips140$unsetBypass(): void { fipsBypass = false; }
export function native$internal$godebug$setUpdate(update: (def: string, env: string) => void): void {
  update(fromJSString(""), fromJSString(gproc?.env?.GODEBUG ?? ""));
}
export function native$internal$godebug$registerMetric(_name: string, _read: () => bigint): void {}
export function native$internal$godebug$setNewIncNonDefault(_f: (name: string) => () => void): void {}

// The runtime/debug knobs have nothing to tune in a JS host: each setter
// records its value and returns the previous one.
const debugKnobs = { gcPercent: 100, maxStack: 1000000000, panicOnFault: false, maxThreads: 10000, memoryLimit: 0x7fffffffffffffffn };
export function native$runtime$debug$setGCPercent(p: number): number {
  const old = debugKnobs.gcPercent;
  debugKnobs.gcPercent = p;
  return old;
}
export function native$runtime$debug$setMaxStack(n: number): number {
  const old = debugKnobs.maxStack;
  debugKnobs.maxStack = n;
  return old;
}
export function native$runtime$debug$setPanicOnFault(b: boolean): boolean {
  const old = debugKnobs.panicOnFault;
  debugKnobs.panicOnFault = b;
  return old;
}
export function native$runtime$debug$setMaxThreads(n: number): number {
  const old = debugKnobs.maxThreads;
  debugKnobs.maxThreads = n;
  return old;
}
export function native$runtime$debug$setMemoryLimit(n: bigint): bigint {
  const old = debugKnobs.memoryLimit;
  if (n >= 0n) debugKnobs.memoryLimit = n; // a negative limit only reads it
  return old;
}
// No garbage collection has run: the pause history is empty, and the last
// GC time, GC count and total pause (the three values after it) are zero.
export function native$runtime$debug$readGCStats(pauses: { v: Slice<bigint> | null }): void {
  pauses.v = new Slice([0n, 0n, 0n], 0, 3, 3);
}
export function native$runtime$debug$freeOSMemory(): void {}
export function native$runtime$debug$SetTraceback(_level: string): void {}
export function native$runtime$debug$WriteHeapDump(_fd: number): void {}
export function native$runtime$debug$modinfo(): string {
  return fromJSString("");
}

export function native$syscall$Getpagesize(): number {
  return 65536;
}

export function native$syscall$Exit(code: number): never {
  return exitProcess(code);
}

export function native$syscall$now(): [bigint, number] {
  const [sec, nsec] = wallNow();
  return [sec, nsec];
}

// os.Args: the program (the script) and its arguments, as with go run.
export function native$os$runtime_args(): Slice<string> {
  const argv: string[] | undefined = gproc?.argv;
  return goStrings(Array.isArray(argv) && argv.length > 1 ? argv.slice(1) : ["js"]);
}

export function native$os$hostExecutable(): string {
  const script = gproc?.argv?.[1];
  return typeof script === "string" && /^(\/|[A-Za-z]:[\\/])/.test(script) ? fromJSString(script) : "";
}

// net/http: the receiver of a (*net.Dialer).Dial or DialContext method
// value, which goesm marks (dialerMethods in internal/lower), or nil.
export function native$net$http$dialerOf(dial: any): any {
  return dial?.$dialer ?? null;
}
export const native$net$http$dialerOfContext = native$net$http$dialerOf;

export function native$os$runtime_beforeExit(_code: number): void {}
export function native$os$sigpipe(): void {}

// os/signal: the host signals of syscall.Signal (GOOS=js numbers them
// SIGCHLD=1, SIGINT, SIGKILL, SIGTRAP, SIGQUIT, SIGTERM), heard through
// process.on, which also keeps them from terminating the process.
const signalNames = ["", "SIGCHLD", "SIGINT", "SIGKILL", "SIGTRAP", "SIGQUIT", "SIGTERM"];
const signalListeners = new Map<number, () => void>();

export function native$os$signal$hostSignal(sig: number, on: boolean, deliver: ((sig: number) => any) | null): void {
  const name = signalNames[sig];
  if (!name || typeof gproc?.on !== "function") return;
  const old = signalListeners.get(sig);
  if (old !== undefined) {
    gproc.off(name, old);
    signalListeners.delete(sig);
  }
  if (!on || deliver === null) return;
  const listener = () => { go(deliver, [sig]); };
  try {
    gproc.on(name, listener);
    signalListeners.set(sig, listener);
  } catch {
    // A signal the host cannot handle (SIGKILL), as with the gc runtime.
  }
}

// runtime.rand, which packages reach by linkname: random uint64s from the
// host's CSPRNG, drawn a block at a time.
const randBuf = new BigUint64Array(64);
let randPos = randBuf.length;
function runtimeRand(): bigint {
  if (randPos === randBuf.length) {
    const c = (globalThis as any).crypto;
    if (c?.getRandomValues) c.getRandomValues(randBuf);
    else for (let i = 0; i < randBuf.length; i++) randBuf[i] = (BigInt(Math.floor(Math.random() * 2 ** 32)) << 32n) | BigInt(Math.floor(Math.random() * 2 ** 32));
    randPos = 0;
  }
  return randBuf[randPos++];
}


// crypto/rand reads the host's CSPRNG (Crypto.getRandomValues) in blocks of
// at most 64 KiB.
export function native$crypto$internal$sysrand$getRandomValues(b: Slice<number> | null): void {
  if (b === null || b.$length === 0) return;
  const c = (globalThis as any).crypto;
  if (!c?.getRandomValues) runtimePanic("crypto/rand: the host has no crypto.getRandomValues");
  const buf = new Uint8Array(b.$length);
  c.getRandomValues(buf);
  for (let i = 0; i < buf.length; i++) b.$array[b.$offset + i] = buf[i];
}

export function native$crypto$internal$sysrand$fatal(msg: string): never {
  writeStd(2, "fatal error: " + msg + "\n");
  return exitProcess(2);
}
// hash/maphash's hashes (runtime.memhash and the map hashers in gc): two
// 32-bit MurmurHash3 lanes seeded with the two halves of the seed. Go strings
// are byte strings, so a string and its bytes hash alike.
function murmurRound(h: number, k: number): number {
  k = Math.imul(k, 0xcc9e2d51);
  k = (k << 15) | (k >>> 17);
  h ^= Math.imul(k, 0x1b873593);
  h = (h << 13) | (h >>> 19);
  return (Math.imul(h, 5) + 0xe6546b64) | 0;
}
function murmurFinal(h: number): number {
  h ^= h >>> 16;
  h = Math.imul(h, 0x85ebca6b);
  h ^= h >>> 13;
  h = Math.imul(h, 0xc2b2ae35);
  return h ^ (h >>> 16);
}
function hash64(n: number, at: (i: number) => number, seed: bigint): bigint {
  let h1 = Number(seed & 0xffffffffn) | 0, h2 = Number(seed >> 32n) | 0;
  let i = 0;
  for (; i + 4 <= n; i += 4) {
    const k = at(i) | (at(i + 1) << 8) | (at(i + 2) << 16) | (at(i + 3) << 24);
    h1 = murmurRound(h1, k);
    h2 = murmurRound(h2, k ^ 0x5bd1e995);
  }
  let k = 0;
  for (let j = 0; i < n; i++, j += 8) k |= at(i) << j;
  h1 = murmurRound(h1 ^ n, k);
  h2 = murmurRound(h2 ^ n, k ^ 0x5bd1e995);
  h1 = (h1 + h2) | 0;
  h2 = (h2 + h1) | 0;
  h1 = murmurFinal(h1);
  h2 = murmurFinal(h2);
  h1 = (h1 + h2) | 0;
  h2 = (h2 + h1) | 0;
  return (BigInt(h2 >>> 0) << 32n) | BigInt(h1 >>> 0);
}
export function native$hash$maphash$hashBytes(b: Slice<number>, seed: bigint): bigint {
  const a = b.$array, off = b.$offset;
  return hash64(b.$length, (i) => a[off + i], seed);
}
export function native$hash$maphash$hashString(s: string, seed: bigint): bigint {
  return hash64(s.length, (i) => s.charCodeAt(i), seed);
}
export function native$hash$maphash$hashComparable(v: Iface | null, seed: bigint): bigint {
  const s = ifaceKeyString(v);
  return hash64(s.length, (i) => s.charCodeAt(i), seed);
}

export const native$os$runtime_rand = runtimeRand;
export const native$math$rand$runtime_rand = runtimeRand;
export const native$math$rand$v2$runtime_rand = runtimeRand;
export const native$hash$maphash$runtime_rand = runtimeRand;
export const native$net$runtime_rand = runtimeRand;
export const native$unique$runtime_rand = runtimeRand;
export const native$internal$sync$runtime_rand = runtimeRand;

// ---- internal/poll ----
//
// goesm's file I/O is synchronous, so the fd mutex is never contended.

export function native$internal$poll$runtime_Semacquire(sema: any): void {
  if (sema.v === 0) runtimePanic("goesm: internal/poll semaphore would block");
  sema.v--;
}

export function native$internal$poll$runtime_Semrelease(sema: any): void {
  sema.v++;
}

// ---- time: clocks ----
//
// The wall clock is Date.now, refined below the millisecond by
// performance.timeOrigin + performance.now while the two agree (the host's
// clock was not set since); the monotonic clock is performance.now, in
// nanoseconds since the program started.

const perf = (globalThis as any).performance;
const monoStart = perf ? perf.now() : Date.now();

function monoNanos(): bigint {
  return BigInt(Math.round(((perf ? perf.now() : Date.now()) - monoStart) * 1e6) + 1);
}

function wallNow(): [bigint, number, bigint] {
  let ms = Date.now();
  if (perf && typeof perf.timeOrigin === "number") {
    const precise = perf.timeOrigin + perf.now();
    if (Math.abs(precise - ms) < 1) ms = precise;
  }
  const sec = Math.floor(ms / 1000);
  return [BigInt(sec), Math.min(Math.round((ms - sec * 1000) * 1e6), 999999999), monoNanos()];
}

export const native$time$now = wallNow;
export const native$time$runtimeNow = wallNow;
export const native$time$runtimeNano = monoNanos;

// ---- time: timers ----
//
// time's timers (internal/natives/patch/time) are armed on the host's
// setTimeout. A pending timer keeps Node and Bun running, so a goroutine
// waiting for one is not a deadlock, as in Go.

const hostTimers = new Map<number, any>();
let nextTimerID = 1;
const maxDelay = 2 ** 31 - 1; // setTimeout's limit, in milliseconds

export function native$time$armTimer(when: bigint, period: bigint, fire: (delta: bigint) => any): number {
  const id = nextTimerID++;
  let target = when;
  const arm = () => {
    const ms = Number(target - monoNanos()) / 1e6;
    hostTimers.set(id, setTimeout(run, Math.min(Math.max(ms, 0), maxDelay)));
  };
  const run = () => {
    const delta = monoNanos() - target;
    if (delta < 0n) return arm(); // a delay beyond setTimeout's limit
    if (period > 0n) {
      target += period * (delta / period + 1n); // drop missed ticks
      arm();
    } else {
      hostTimers.delete(id);
    }
    go(fire, [delta]);
  };
  arm();
  return id;
}

export function native$time$disarmTimer(id: number): void {
  clearTimeout(hostTimers.get(id));
  hostTimers.delete(id);
}

export function native$time$runtimeIsBubbled(): boolean {
  return false;
}
