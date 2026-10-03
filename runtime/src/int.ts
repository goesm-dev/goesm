// Integer semantics.
//
// 8/16/32-bit integers are exact: the lowering wraps every arithmetic result
// (x|0, x>>>0, x<<24>>24, ...). int, int64, uint, uint64 and uintptr are
// represented as JS numbers in the PoC: exact up to 2^53, without 64-bit
// wrap-around. This is a documented gap (see ARCHITECTURE.md); the helpers
// below are where a BigInt or hi/lo representation would plug in.

import { runtimePanic } from "./panic";

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
