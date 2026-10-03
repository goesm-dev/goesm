// Integer semantics.
//
// 8/16/32-bit integers are exact: the lowering wraps every arithmetic result
// (x|0, x>>>0, x<<24>>24, ...). int, int64, uint, uint64 and uintptr are
// represented as JS numbers in the PoC: exact up to 2^53, without 64-bit
// wrap-around. This is a documented gap (see ARCHITECTURE.md); the helpers
// below are where a BigInt or hi/lo representation would plug in.

import { runtimePanic } from "./panic.ts";
import { bytesToString, encodeRune, runesToString, stringToBytes, stringToRunes } from "./string.ts";
import { c64, cadd, cdiv, cmul, cneg, csub } from "./complex.ts";
import { Kind, Type } from "./types.ts";

export function div(a: number, b: number): number {
  if (b === 0) runtimePanic("integer divide by zero");
  return Math.trunc(a / b);
}

export function mod(a: number, b: number): number {
  if (b === 0) runtimePanic("integer divide by zero");
  const r = a % b;
  return r === 0 ? 0 : r; // avoid -0
}

export const imul = Math.imul;
export const trunc = Math.trunc;
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

// 64-bit operations go through BigInt so in-range results are exact.
export function shl64(x: number, n: number, signed: boolean): number {
  checkShift(n);
  if (n >= 64) return 0;
  const r = BigInt(x) << BigInt(n);
  return Number(signed ? BigInt.asIntN(64, r) : BigInt.asUintN(64, r));
}

export function shr64(x: number, n: number, signed: boolean): number {
  checkShift(n);
  if (n >= 64) return signed && x < 0 ? -1 : 0;
  return Number(BigInt(x) >> BigInt(n));
}

export function and64(a: number, b: number): number { return Number(BigInt(a) & BigInt(b)); }
export function or64(a: number, b: number): number { return Number(BigInt(a) | BigInt(b)); }
export function xor64(a: number, b: number): number { return Number(BigInt(a) ^ BigInt(b)); }
export function andNot64(a: number, b: number): number { return Number(BigInt(a) & ~BigInt(b)); }
export function not64(a: number, signed: boolean): number {
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
  return k === Kind.Int || k === Kind.Int64 || k === Kind.Uint || k === Kind.Uint64 || k === Kind.Uintptr;
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
  if (is64(t.kind)) return left ? shl64(a, n, isSigned(t.kind)) : shr64(a, n, isSigned(t.kind));
  return wrapT(t, left ? shl32(a, n) : shr32(a, n, isSigned(t.kind)));
}

export function negT(t: Type, x: any): any {
  if (t.kind === Kind.Complex64 || t.kind === Kind.Complex128) return cneg(x);
  return wrapT(t, -x);
}

export function notT(t: Type, x: any): any {
  return is64(t.kind) ? not64(x, isSigned(t.kind)) : wrapT(t, ~x);
}

// convertT converts x from type `from` to type `to` where either is a type
// parameter.
export function convertT(to: Type, from: Type, x: any): any {
  const tk = to.kind, fk = from.kind;
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
