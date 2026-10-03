// The print and println builtins. They write to standard error, as in Go,
// and format their operands like the Go runtime does (runtime/print.go);
// goesm's lowering wraps operands whose format differs from JS's (floats,
// complex numbers, references) in the helpers below.

import type { Complex } from "./complex.ts";
import { writeStd } from "./host.ts";

// Strings are Go (byte) strings and written as they are.
function str(a: any): string {
  return typeof a === "string" ? a : String(a);
}

export function print(...args: any[]): void {
  writeStd(2, args.map(str).join(""));
}

export function println(...args: any[]): void {
  writeStd(2, args.map(str).join(" ") + "\n");
}

// printFloat formats v as the Go runtime does: strconv's shortest %g for
// the float's size (bits 32 or 64).
export function printFloat(v: number, bits = 64): string {
  if (v !== v) {
    return "NaN";
  }
  if (v === Infinity) {
    return "+Inf";
  }
  if (v === -Infinity) {
    return "-Inf";
  }
  const neg = v < 0 || (v === 0 && 1 / v < 0);
  const a = Math.abs(v);
  let digits = "0";
  let exp = 0; // decimal exponent of the first digit
  if (a !== 0) {
    let s = a.toExponential();
    if (bits === 32) {
      for (let p = 0; p < 9; p++) {
        const t = a.toExponential(p);
        if (Math.fround(Number(t)) === a) {
          s = t;
          break;
        }
      }
    }
    const e = s.indexOf("e");
    digits = s.slice(0, e).replace(".", "").replace(/0+$/, "") || "0";
    exp = Number(s.slice(e + 1));
  }
  let out: string;
  if (exp < -4 || exp >= 6) {
    const ea = Math.abs(exp);
    out = digits[0] + (digits.length > 1 ? "." + digits.slice(1) : "") + "e" + (exp < 0 ? "-" : "+") + (ea < 10 ? "0" : "") + ea;
  } else if (exp < 0) {
    out = "0." + "0".repeat(-exp - 1) + digits;
  } else if (digits.length <= exp + 1) {
    out = digits + "0".repeat(exp + 1 - digits.length);
  } else {
    out = digits.slice(0, exp + 1) + "." + digits.slice(exp + 1);
  }
  return (neg ? "-" : "") + out;
}

export function printComplex(c: Complex, bits = 64): string {
  const im = printFloat(c.im, bits);
  return "(" + printFloat(c.re, bits) + (im[0] === "-" || im[0] === "+" ? "" : "+") + im + "i)";
}

// JS has no addresses; non-nil references print as a fixed fake address
// (Go's addresses are not reproducible either).
export function printPointer(p: any): string {
  return p === null || p === undefined ? "0x0" : "0xc000010000";
}

export function printIface(i: any): string {
  return i === null ? "(0x0,0x0)" : "(0x4b0000,0xc000010000)";
}

export function printSlice(s: any): string {
  return s === null ? "[0/0]0x0" : "[" + s.$length + "/" + s.$capacity + "]0xc000010000";
}
