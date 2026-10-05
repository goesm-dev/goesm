// fmt.Sprintf for the common cases (see the fmt patch): the verbs
// %v %d %s %t %x %X %f %F of strings, booleans, integers and float64s of
// the predeclared types, with the flags - and 0, a width and a precision
// for %f. A format is parsed once and cached. Anything else returns null
// for the patch's Go code (fastSprintf, then fmt's own) to format.

// Like natives.ts, which imports it, this uses the runtime only through its
// public module, so that split builds share one runtime.
import { decodeRune, goThrown, Goexit, ProgramExit, tBool, tFloat64, tInt, tInt16, tInt32, tInt64, tInt8, tString, tUint, tUint16, tUint32, tUint64, tUint8, tUintptr } from "./index.ts";
import type { Iface, Type } from "./index.ts";
import type { S } from "./index.ts";

interface Spec {
  lit: string; // the text before the verb
  minus: boolean;
  zero: boolean;
  wid: number; // -1 if none
  prec: number; // -1 if none
  verb: number;
}

interface Format {
  specs: Spec[];
  tail: string; // the text after the last verb
}

const formats = new Map<string, Format | null>();

// parse parses a format into its verbs, or returns null for one with a
// verb that is not a single ASCII byte, or an unterminated one.
function parse(format: string): Format | null {
  const specs: Spec[] = [], end = format.length;
  let lit = "", start = 0;
  for (let i = 0; i < end; ) {
    i = format.indexOf("%", i);
    if (i < 0) break;
    lit += format.slice(start, i);
    i++;
    if (format.charCodeAt(i) === 0x25) { // %%
      lit += "%";
      start = ++i;
      continue;
    }
    let minus = false, zero = false;
    for (; i < end; i++) {
      const c = format.charCodeAt(i);
      if (c === 0x2d) { minus = true; zero = false; }
      else if (c === 0x30) zero = !minus;
      else break;
    }
    let wid = -1, prec = -1, c = format.charCodeAt(i);
    if (c >= 0x30 && c <= 0x39) {
      wid = 0;
      for (; c >= 0x30 && c <= 0x39; c = format.charCodeAt(++i)) {
        if (wid > 1e5) return null;
        wid = wid * 10 + c - 0x30;
      }
    }
    if (c === 0x2e) {
      prec = 0;
      for (c = format.charCodeAt(++i); c >= 0x30 && c <= 0x39; c = format.charCodeAt(++i)) {
        if (prec > 1e5) return null;
        prec = prec * 10 + c - 0x30;
      }
    }
    if (i >= end || c >= 0x80) return null;
    specs.push({ lit, minus, zero, wid, prec, verb: c });
    lit = "";
    start = ++i;
  }
  return { specs, tail: lit + format.slice(start) };
}

// sprintf is fmt.Sprintf(format, a...), or null.
export function sprintf(format: string, a: S<Iface | null>): string | null {
  let f = formats.get(format);
  if (f === undefined) {
    f = parse(format);
    if (formats.size < 1000) formats.set(format, f);
  }
  const n = a === null ? 0 : a.$length;
  if (f === null || f.specs.length !== n) return null;
  let s = "";
  for (let k = 0; k < n; k++) {
    const sp = f.specs[k], x = a!.$array[a!.$offset + k];
    if (x === null) return null;
    const t = x.t, v = x.v, verb = sp.verb;
    let num: string;
    if (t === tString) {
      if ((verb !== 0x73 && verb !== 0x76) || sp.prec >= 0 || sp.zero) return null; // s v
      s += sp.lit + padded(v, sp.wid, sp.minus);
      continue;
    } else if (t === tBool) {
      if ((verb !== 0x74 && verb !== 0x76) || sp.prec >= 0 || sp.zero) return null; // t v
      s += sp.lit + padded(v ? "true" : "false", sp.wid, sp.minus);
      continue;
    } else if (t === tFloat64) {
      num = fmtFloat(v, verb, sp.prec);
    } else if (isSmallInt(t)) {
      num = fmtInt(v, verb, sp.prec);
    } else if (t === tInt64 || t === tUint64) {
      num = fmtInt(v, verb, sp.prec);
    } else {
      return null;
    }
    if (num === "") return null;
    if (sp.zero && sp.wid > num.length && num !== "NaN" && num !== "+Inf" && num !== "-Inf") {
      num = num.charCodeAt(0) === 0x2d ? "-" + num.slice(1).padStart(sp.wid - 1, "0") : num.padStart(sp.wid, "0");
    }
    s += sp.lit + padded(num, sp.wid, sp.minus);
  }
  return s + f.tail;
}

// isSmallInt reports whether t is an integer type held in a JS number
// (int64 and uint64 are BigInts).
function isSmallInt(t: Type): boolean {
  return t === tInt || t === tUint8 || t === tInt32 || t === tUint32 || t === tUint ||
    t === tInt8 || t === tInt16 || t === tUint16 || t === tUintptr;
}

// fmtInt formats an integer (a number or a BigInt) for %d, %v, %x or %X
// without a precision, as fmtInteger does, or returns "".
function fmtInt(v: number | bigint, verb: number, prec: number): string {
  if (prec >= 0) return "";
  if (typeof v === "number" && !Number.isSafeInteger(v)) return ""; // an int past 2^53: Go's code
  switch (verb) {
    case 0x64: case 0x76: return v.toString(); // d v
    case 0x78: return v.toString(16); // x
    case 0x58: return v.toString(16).toUpperCase(); // X
  }
  return "";
}

// fmtFloat formats a float64 for %v and for %f and %F, or returns "".
function fmtFloat(v: number, verb: number, prec: number): string {
  switch (verb) {
    case 0x76: // v
      return prec < 0 ? fmtShortest(v) : "";
    case 0x66: case 0x46: // f F
      return fmtF(v, prec < 0 ? 6 : prec);
  }
  return "";
}

// padded pads s with spaces to wid runes, on the right if minus.
function padded(s: string, wid: number, minus: boolean): string {
  if (wid <= 0) return s;
  const n = wid - runeCount(s);
  if (n <= 0) return s;
  return minus ? s + " ".repeat(n) : " ".repeat(n) + s;
}

// runeCount is utf8.RuneCountInString(s).
function runeCount(s: string): number {
  let n = 0;
  for (let i = 0; i < s.length; n++) i += s.charCodeAt(i) < 0x80 ? 1 : decodeRune(s, i)[1];
  return n;
}

// ---- float formatting, shared with strconv's natives ----

const scratch = /* @__PURE__ */ new DataView(new ArrayBuffer(8));

// fmtFixed is strconv.FormatFloat(v, 'f', prec, 64) for the values
// ftoaDigits (natives.ts) formats with toFixed, and "" for the others.
export function fmtFixed(v: number, prec: number): string {
  const x = Math.abs(v);
  if (v === 0 || !(x < 1e21) || prec > 100) return "";
  let t = x.toFixed(prec);
  if (mayTie(x, prec) && tieScale(x) === prec) t = roundToEven(t);
  return v < 0 ? "-" + t : t;
}

const pow10 = [1, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22];

// mayTie is false if x is certainly not halfway between two numbers with
// prec decimals: then x*10^prec, exact for a tie below 2^52 (10^prec is
// exact, and so is the correctly rounded product of a representable
// result), does not end in .5.
export function mayTie(x: number, prec: number): boolean {
  if (prec > 22) return true;
  const y = x * pow10[prec];
  return y >= 2 ** 52 || y - Math.floor(y) === 0.5;
}

// fmtShortest is strconv.FormatFloat(v, 'g', -1, 64), fmt's %v of a
// float64: the shortest digits that read back as v (toExponential's), in
// %e form if the exponent is below -4 or at least max(digits, 6)... as
// strconv's fmtEFG decides.
export function fmtShortest(v: number): string {
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

// tieScale returns the s for which x·10^s lies exactly halfway between two
// integers, or NaN if there is none. With x = m·2^e for an odd m, that is
// when 2·m·2^e·10^s is an odd integer: e+s+1 = 0, and 5^-s divides m if
// s < 0.
export function tieScale(x: number): number {
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
export function roundToEven(t: string): string {
  const c = t.charCodeAt(t.length - 1);
  return (c & 1) === 1 ? t.slice(0, -1) + String.fromCharCode(c - 1) : t;
}

// plainErr reports whether fmt prints the error e for %v, %s and %w with
// its Error method alone: e is not nil and has no Format method.
export function plainErr(e: Iface | null): boolean {
  return e !== null && !e.t.methods.has("Format");
}

// errText is fmt's %v, %s or %w (verb) of the error e, as goesm lowers a
// Sprintf or Errorf with a constant format, or null for an error that
// plainErr rejects. A panic of its Error method gives the text fmt prints
// for it, from the fmt patch's panicText.
export function errText(e: Iface | null, verb: number, panicText: (arg: Iface | null, verb: number, method: string, err: Iface | null) => string): string | null {
  if (!plainErr(e)) return null;
  try {
    return e!.t.mt.Error(e!.v);
  } catch (x) {
    const p = goThrown(x);
    if (p instanceof Goexit || p instanceof ProgramExit) throw p;
    return panicText(e, verb, "Error", (p as any).value);
  }
}

// quoteText is strconv.Quote(s), fmt's %q of a string, for a string of
// printable ASCII without " or \, or null.
export function quoteText(s: string): string | null {
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c < 0x20 || c > 0x7e || c === 0x22 || c === 0x5c) return null;
  }
  return '"' + s + '"';
}

// fmtF is fmt's %f of a float64 with precision prec, as goesm lowers a
// Sprintf with a constant format, or "" for the values that fmtFixed leaves
// to strconv (NaN, ±Inf, beyond 1e21).
export function fmtF(v: number, prec: number): string {
  if (v === 0 && prec <= 100) return (Object.is(v, -0) ? "-" : "") + (0).toFixed(prec);
  return fmtFixed(v, prec);
}
