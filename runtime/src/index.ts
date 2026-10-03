// @goesm/runtime: the semantic support library for TypeScript lowered from Go.
//
// Generated code imports this module as `$rt`. The runtime only implements Go
// semantics that JS does not have natively; it never decides Go typing
// questions, which goesm settles at compile time with go/types.

export * from "./types";
export * from "./iface";
export * from "./panic";
export * from "./slice";
export * from "./map";
export * from "./ptr";
export * from "./string";
export * from "./int";
export * from "./chan";
export * from "./interop";

import { toJSString } from "./string";

// print/println builtins. Like Go they write to standard error, byte for
// byte, in the format of the Go runtime's printers. Floats arrive already
// formatted (fmtFloat), since a JS number does not say whether it was a Go
// float. Hosts without a stderr stream get whole lines on console.error.
export function print(...args: any[]): void {
  gwrite(args.map(printArg).join(""));
}

export function println(...args: any[]): void {
  gwrite(args.map(printArg).join(" ") + "\n");
}

function printArg(a: any): string {
  switch (typeof a) {
    case "string":
      return a;
    case "boolean":
    case "number":
    case "bigint":
      return String(a);
  }
  return a === null || a === undefined ? "0x0" : addr; // pointer, map, chan, func
}

let pending = "";

function gwrite(s: string): void {
  const stderr = (globalThis as any).process?.stderr;
  if (stderr && typeof stderr.write === "function") {
    const b = new Uint8Array(s.length);
    for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i);
    stderr.write(b);
    return;
  }
  pending += s;
  const nl = pending.lastIndexOf("\n");
  if (nl >= 0) {
    for (const line of pending.slice(0, nl).split("\n")) console.error(toJSString(line));
    pending = pending.slice(nl + 1);
  }
}

// Addresses are not observable in goesm; non-nil pointers print as a fixed
// heap-looking address so output keeps Go's shape.
const addr = "0xc000010000";

// fmtSlice formats a slice for print/println: [len/cap]array-address.
export function fmtSlice(s: any): string {
  return s === null ? "[0/0]0x0" : `[${s.$length}/${s.$capacity}]${addr}`;
}

// fmtIface formats an interface value for print/println: (type,data).
export function fmtIface(v: any): string {
  return v === null ? "(0x0,0x0)" : `(${addr},${addr})`;
}

// fmtFloat formats a float for print/println like the Go runtime does:
// strconv.FormatFloat(v, 'g', -1, bits).
export function fmtFloat(v: number, bits: number): string {
  if (v !== v) return "NaN";
  if (v === Infinity) return "+Inf";
  if (v === -Infinity) return "-Inf";
  const neg = v < 0 || Object.is(v, -0);
  if (neg) v = -v;
  // Shortest digits that round-trip at this precision.
  let e = v.toExponential();
  if (bits === 32) {
    for (let p = 0; p < 9; p++) {
      const t = v.toExponential(p);
      if (Math.fround(+t) === v) {
        e = t;
        break;
      }
    }
  }
  const [mant, ex] = e.split("e");
  let digits = mant.replace(".", "");
  if (/^0+$/.test(digits)) digits = "";
  const dp = digits === "" ? 0 : +ex + 1;
  const nd = digits.length;
  const exp = dp - 1;
  let out: string;
  if (nd > 0 && (exp < -4 || exp >= 6)) {
    const a = Math.abs(exp);
    out = digits[0] + (nd > 1 ? "." + digits.slice(1) : "") + "e" + (exp < 0 ? "-" : "+") + (a < 10 ? "0" + a : String(a));
  } else if (nd === 0) {
    out = "0";
  } else if (dp <= 0) {
    out = "0." + "0".repeat(-dp) + digits;
  } else if (dp >= nd) {
    out = digits + "0".repeat(dp - nd);
  } else {
    out = digits.slice(0, dp) + "." + digits.slice(dp);
  }
  return (neg ? "-" : "") + out;
}
