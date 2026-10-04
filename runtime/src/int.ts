// Integer semantics.
//
// 8/16/32-bit integers are exact: the lowering wraps every arithmetic result
// (x|0, x>>>0, x<<24>>24, ...). int64 and uint64 are BigInts, wrapped inline
// with BigInt.asIntN/asUintN; the helpers here cover what needs a check
// (division, shifts). int, uint and uintptr are JS numbers: exact up to 2^53,
// without 64-bit wrap-around (a documented gap, see ARCHITECTURE.md).

import { runtimePanic } from "./panic.ts";
import { bytesToString, encodeRune, runesToString, stringToBytes, stringToRunes } from "./string.ts";
import { Complex, c64, cadd, cdiv, cmul, cneg, complex, csub } from "./complex.ts";
import { Kind, Type } from "./types.ts";

export function div(a: number, b: number): number {
  if (b === 0) runtimePanic("integer divide by zero");
  return Math.trunc(a / b) + 0; // + 0: -1 / 2 is 0, not -0
}

export function mod(a: number, b: number): number {
  if (b === 0) runtimePanic("integer divide by zero");
  const r = a % b;
  return r === 0 ? 0 : r; // avoid -0
}

// ---- int64 / uint64 (BigInt) ----

export function divBig(a: bigint, b: bigint): bigint {
  if (b === 0n) runtimePanic("integer divide by zero");
  return a / b; // the caller wraps (MinInt64 / -1)
}

export function modBig(a: bigint, b: bigint): bigint {
  if (b === 0n) runtimePanic("integer divide by zero");
  return a % b;
}

// bigShifts[n] is BigInt(n) for the shift counts 0 to 64.
const bigShifts: bigint[] = [];
for (let i = 0; i <= 64; i++) bigShifts.push(BigInt(i));

export function shlBig(x: bigint, n: number, signed: boolean): bigint {
  checkShift(n);
  if (n >= 64) return 0n;
  const r = x << bigShifts[n];
  return signed ? BigInt.asIntN(64, r) : BigInt.asUintN(64, r);
}

export function shrBig(x: bigint, n: number): bigint {
  checkShift(n);
  return x >> bigShifts[n >= 64 ? 64 : n];
}

const two63 = 9223372036854775808;
const two64 = 18446744073709551616;

// floatToBig converts a float to int64 or uint64 like GOARCH=wasm's
// saturating truncation (out-of-range results are implementation-specific
// in Go).
export function floatToBig(f: number, signed: boolean): bigint {
  if (f !== f) return 0n;
  if (signed) {
    if (f >= two63) return 9223372036854775807n;
    if (f <= -two63) return -9223372036854775808n;
  } else {
    if (f >= two64) return 18446744073709551615n;
    if (f <= 0) return 0n;
  }
  return BigInt(Math.trunc(f));
}

// intNumber converts an integer of a type parameter's type to a JS number.
export function intNumber(x: number | bigint): number {
  return typeof x === "bigint" ? Number(x) : x;
}

export const imul = Math.imul;
export const trunc = Math.trunc;
export const floor = Math.floor;
export const fround = Math.fround;

function checkShift(n: number): void {
  if (n < 0) runtimePanic("negative shift amount");
}

// 32-bit and narrower shifts; the caller wraps the result to its width.
export function shl32(x: number, n: number): number {
  checkShift(n);
  return n >= 32 ? 0 : x << n;
}

export function shr32(x: number, n: number, signed: boolean): number {
  checkShift(n);
  if (signed) return x >> Math.min(n, 31);
  return n >= 32 ? 0 : x >>> n;
}

// 64-bit operations on int, uint and uintptr (numbers). Shifts scale by
// powers of two, exact for integers; bitwise operations work on the two
// 32-bit halves. A result beyond 2^53 rounds like Number(BigInt(...)).
const pow2: number[] = [];
for (let i = 0; i < 64; i++) pow2.push(2 ** i);
const two32 = 4294967296;
const maxSafe = 9007199254740992; // 2^53

export function shl64(x: number, n: number, signed: boolean): number {
  checkShift(n);
  if (n >= 64) return 0;
  const r = x * pow2[n];
  if (r < maxSafe && r > -maxSafe) return r; // no wrapping
  const b = BigInt(x) << BigInt(n);
  return Number(signed ? BigInt.asIntN(64, b) : BigInt.asUintN(64, b));
}

export function shr64(x: number, n: number, signed: boolean): number {
  checkShift(n);
  if (n >= 64) return signed && x < 0 ? -1 : 0;
  return Math.floor(x / pow2[n]);
}

// bits64 applies the 32-bit operation f to the halves of a and b. The
// result is negative when an operand is (only signed values are).
function bits64(a: number, b: number, f: (x: number, y: number) => number): number {
  const ah = Math.floor(a / two32), bh = Math.floor(b / two32);
  const lo = f(a - ah * two32, b - bh * two32) >>> 0;
  const h = f(ah, bh);
  return (a < 0 || b < 0 ? h | 0 : h >>> 0) * two32 + lo;
}

const andF = (x: number, y: number) => x & y;
const orF = (x: number, y: number) => x | y;
const xorF = (x: number, y: number) => x ^ y;
const andNotF = (x: number, y: number) => x & ~y;

// In each fast path both operands fit in 32 bits of one signedness, where
// the 32-bit operation is the 64-bit one.
export function and64(a: number, b: number): number {
  if ((a | 0) === a && (b | 0) === b) return a & b;
  if (a >>> 0 === a && b >>> 0 === b) return (a & b) >>> 0;
  return bits64(a, b, andF);
}
export function or64(a: number, b: number): number {
  if ((a | 0) === a && (b | 0) === b) return a | b;
  if (a >>> 0 === a && b >>> 0 === b) return (a | b) >>> 0;
  return bits64(a, b, orF);
}
export function xor64(a: number, b: number): number {
  if ((a | 0) === a && (b | 0) === b) return a ^ b;
  if (a >>> 0 === a && b >>> 0 === b) return (a ^ b) >>> 0;
  return bits64(a, b, xorF);
}
export function andNot64(a: number, b: number): number {
  if ((a | 0) === a && (b | 0) === b) return a & ~b;
  if (a >>> 0 === a && b >>> 0 === b) return (a & ~b) >>> 0;
  return bits64(a, b, andNotF);
}
export function not64(a: number, signed: boolean): number {
  if (signed && a < maxSafe && a > -maxSafe) return -a - 1;
  const r = ~BigInt(a);
  return Number(signed ? r : BigInt.asUintN(64, r));
}

// min/max builtins (Go 1.21) for ordered types; NaN propagates like Go.
export function min(...xs: any[]): any {
  let m = xs[0];
  for (let i = 0; i < xs.length; i++) {
    const x = xs[i];
    if (x !== x) return x;
    if (x < m || (x === 0 && m === 0 && Object.is(x, -0))) m = x;
  }
  return m;
}

export function max(...xs: any[]): any {
  let m = xs[0];
  for (let i = 0; i < xs.length; i++) {
    const x = xs[i];
    if (x !== x) return x;
    if (x > m || (x === 0 && m === 0 && Object.is(m, -0))) m = x;
  }
  return m;
}

// ---- operators on type-parameter-typed operands ----
//
// Under erasure the operand kind of `a + b` with `T Number` is only known
// from the type argument's descriptor, so these helpers switch on it.

function is64(k: number): boolean {
  return k === Kind.Int || k === Kind.Uint || k === Kind.Uintptr;
}

function isBig(k: number): boolean {
  return k === Kind.Int64 || k === Kind.Uint64;
}

function isSigned(k: number): boolean {
  return k >= Kind.Int && k <= Kind.Int64;
}

function isInteger(k: number): boolean {
  return k >= Kind.Int && k <= Kind.Uintptr;
}

// wrapT wraps x to the width of integer kind t (and rounds float32).
export function wrapT(t: Type, x: any): any {
  switch (t.kind) {
    case Kind.Int64: return BigInt.asIntN(64, x);
    case Kind.Uint64: return BigInt.asUintN(64, x);
    case Kind.Int8: return (x << 24) >> 24;
    case Kind.Int16: return (x << 16) >> 16;
    case Kind.Int32: return x | 0;
    case Kind.Uint8: return x & 0xff;
    case Kind.Uint16: return x & 0xffff;
    case Kind.Uint32: return x >>> 0;
    case Kind.Float32: return Math.fround(x);
  }
  return x;
}

export function arithT(t: Type, op: string, a: any, b: any): any {
  const k = t.kind;
  if (k === Kind.String) return a + b;
  if (k === Kind.Complex64 || k === Kind.Complex128) {
    const c = op === "+" ? cadd(a, b) : op === "-" ? csub(a, b) : op === "*" ? cmul(a, b) : cdiv(a, b);
    return k === Kind.Complex64 ? c64(c) : c;
  }
  if (!isInteger(k)) {
    switch (op) {
      case "+": return wrapT(t, a + b);
      case "-": return wrapT(t, a - b);
      case "*": return wrapT(t, a * b);
      case "/": return wrapT(t, a / b);
    }
  }
  if (isBig(k)) {
    switch (op) {
      case "+": return wrapT(t, a + b);
      case "-": return wrapT(t, a - b);
      case "*": return wrapT(t, a * b);
      case "/": return wrapT(t, divBig(a, b));
      case "%": return modBig(a, b);
      case "&": return a & b;
      case "|": return a | b;
      case "^": return a ^ b;
      case "&^": return a & ~b;
    }
  }
  switch (op) {
    case "+": return wrapT(t, a + b);
    case "-": return wrapT(t, a - b);
    case "*": return wrapT(t, is64(k) ? a * b : Math.imul(a, b));
    case "/": return wrapT(t, div(a, b));
    case "%": return wrapT(t, mod(a, b));
    case "&": return is64(k) ? and64(a, b) : wrapT(t, a & b);
    case "|": return is64(k) ? or64(a, b) : wrapT(t, a | b);
    case "^": return is64(k) ? xor64(a, b) : wrapT(t, a ^ b);
    case "&^": return is64(k) ? andNot64(a, b) : wrapT(t, a & ~b);
  }
  throw new Error("goesm: unknown operator " + op);
}

export function shiftT(t: Type, left: boolean, a: any, n: number): any {
  if (isBig(t.kind)) return left ? shlBig(a, n, isSigned(t.kind)) : shrBig(a, n);
  if (is64(t.kind)) return left ? shl64(a, n, isSigned(t.kind)) : shr64(a, n, isSigned(t.kind));
  return wrapT(t, left ? shl32(a, n) : shr32(a, n, isSigned(t.kind)));
}

export function negT(t: Type, x: any): any {
  if (t.kind === Kind.Complex64 || t.kind === Kind.Complex128) return cneg(x);
  return wrapT(t, -x);
}

export function notT(t: Type, x: any): any {
  if (isBig(t.kind)) return wrapT(t, ~x);
  return is64(t.kind) ? not64(x, isSigned(t.kind)) : wrapT(t, ~x);
}

// convertT converts x from type `from` to type `to` where either is a type
// parameter.
export function convertT(to: Type, from: Type, x: any): any {
  const tk = to.kind, fk = from.kind;
  if (isBig(fk) && !isBig(tk)) {
    // To a number first: the width and sign of narrower kinds come from
    // wrapT below.
    x = isInteger(tk) && !is64(tk) ? Number(BigInt.asUintN(32, x)) : tk === Kind.Uint || tk === Kind.Uintptr ? Number(BigInt.asUintN(64, x)) : Number(x);
  }
  if (isBig(tk)) {
    if (isBig(fk)) return wrapT(to, x);
    if (isInteger(fk)) return wrapT(to, BigInt(x));
    return floatToBig(x, tk === Kind.Int64);
  }
  if (tk === Kind.String) {
    if (isInteger(fk)) return encodeRune(x);
    if (fk === Kind.Slice) return from.elem!.kind === Kind.Int32 ? runesToString(x) : bytesToString(x);
    return x;
  }
  if (tk === Kind.Slice && fk === Kind.String) {
    return to.elem!.kind === Kind.Int32 ? stringToRunes(x) : stringToBytes(x);
  }
  if (isInteger(tk)) return wrapT(to, isInteger(fk) ? x : Math.trunc(x));
  if (tk === Kind.Float32 || tk === Kind.Float64) return wrapT(to, x);
  if (tk === Kind.Complex64 && fk !== Kind.Complex64) return c64(x);
  return x;
}

// constT is a numeric constant of a type parameter's type, in the
// representation of the type argument. go/types has checked that the value
// is representable in every type of the type set.
export function constT(t: Type, v: any): any {
  if (v instanceof Complex && t.kind !== Kind.Complex64 && t.kind !== Kind.Complex128) v = v.re;
  switch (t.kind) {
    case Kind.Int64: case Kind.Uint64:
      return typeof v === "bigint" ? v : BigInt(v);
    case Kind.Float32:
      return Math.fround(Number(v));
    case Kind.Complex64:
      return c64(typeof v === "number" ? complex(v, 0) : v);
    case Kind.Complex128:
      return typeof v === "number" ? complex(v, 0) : v;
  }
  return typeof v === "bigint" ? Number(v) : v;
}
