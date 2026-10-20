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

// ---- strconv.FormatFloat and ParseFloat of float64 (strconvcall.go) ----

// formatFloat64 is strconv.FormatFloat(v, fmt, prec, 64) for fmt 'e', 'E',
// 'f', 'g' and 'G': Go's fmtEFG of the digits floatDigits gives, as
// internal/strconv's ftoa64 formats them.
export function formatFloat64(v: number, fmt: number, prec: number): string {
  if (v !== v) return "NaN";
  if (v === Infinity) return "+Inf";
  if (v === -Infinity) return "-Inf";
  const neg = v < 0 || Object.is(v, -0);
  if (v === 0) return fmtEFG(neg, "", 0, prec, fmt, prec < 0);
  if (prec < 0 && fmt === 103) return fmtShortest(v); // 'g'
  if (fmt === 102 && prec >= 0) { // 'f'
    const t = fmtFixed(v, prec);
    if (t !== "") return t;
  }
  const [d, dp] = floatDigits(Math.abs(v), fmt, prec);
  const shortest = prec < 0;
  if (shortest) {
    prec = fmt === 101 || fmt === 69 ? Math.max(d.length - 1, 0) : fmt === 102 ? Math.max(d.length - dp, 0) : d.length;
  } else if ((fmt === 103 || fmt === 71) && prec === 0) {
    prec = 1;
  }
  return fmtEFG(neg, d, dp, prec, fmt, shortest);
}

// floatDigits returns the decimal digits of x > 0, without leading or
// trailing zeros, and the position of the decimal point: the shortest that
// read back as x if prec < 0, else x rounded to even to prec digits for fmt
// ('e': prec+1 significant digits, 'g': prec, 'f': prec after the point).
function floatDigits(x: number, fmt: number, prec: number): [string, number] {
  let t: string, dp: number;
  const n = fmt === 101 || fmt === 69 ? prec + 1 : Math.max(prec, 1); // significant digits for 'e' and 'g'
  if (prec < 0 || (fmt !== 102 && n <= 101)) {
    t = prec < 0 ? x.toExponential() : x.toExponential(n - 1);
    const e = t.indexOf("e");
    const exp = +t.slice(e + 1);
    t = t.slice(0, e);
    if (prec >= 0 && tieScale(x) === n - 1 - exp) t = roundToEven(t);
    dp = exp + 1;
  } else {
    // Beyond the engine's range (toFixed for 'f' up to 1e21 and 100
    // decimals, toExponential up to 101 digits): x's exact decimal
    // expansion, x = m·2^e, rounded to even.
    scratch.setFloat64(0, x);
    const hi = scratch.getUint32(0), lo = scratch.getUint32(4);
    const be = (hi >>> 20) & 0x7ff;
    const m = BigInt((hi & 0xfffff) * 2 ** 32 + lo + (be === 0 ? 0 : 2 ** 52));
    const e = (be === 0 ? 1 : be) - 1075;
    t = e >= 0 ? (m << BigInt(e)).toString() : (m * 5n ** BigInt(-e)).toString();
    dp = e >= 0 ? t.length : t.length + e;
    let keep = fmt === 102 ? dp + prec : n;
    if (keep < t.length) {
      if (keep < 0) return ["", 0];
      const c = t.charCodeAt(keep);
      let up = c > 53 || (c === 53 && (/[1-9]/.test(t.slice(keep + 1)) || (keep > 0 && (t.charCodeAt(keep - 1) & 1) === 1)));
      t = t.slice(0, keep);
      if (up) {
        let i = keep - 1;
        while (i >= 0 && t.charCodeAt(i) === 57) i--;
        if (i < 0) {
          t = "1";
          dp++;
        } else {
          t = t.slice(0, i) + String.fromCharCode(t.charCodeAt(i) + 1);
        }
      }
    }
  }
  t = t.replace(".", "");
  let i = 0;
  while (i < t.length && t.charCodeAt(i) === 48) i++;
  dp -= i;
  t = t.slice(i).replace(/0+$/, "");
  return t === "" ? ["", 0] : [t, dp];
}

// fmtEFG is internal/strconv's: the digits d (a decimal point at dp) in
// format fmt with precision prec.
function fmtEFG(neg: boolean, d: string, dp: number, prec: number, fmt: number, shortest: boolean): string {
  const nd = d.length;
  if (fmt === 103 || fmt === 71) { // 'g', 'G'
    let eprec = prec;
    if (eprec > nd && nd >= dp) eprec = nd;
    if (shortest) eprec = 6;
    const exp = dp - 1;
    if (exp < -4 || exp >= eprec) {
      if (prec > nd) prec = nd;
      prec--;
      fmt = fmt + 101 - 103;
    } else {
      if (prec > dp) prec = nd;
      prec = Math.max(prec - dp, 0);
      fmt = 102;
    }
  }
  let s = neg ? "-" : "";
  if (fmt === 101 || fmt === 69) { // 'e', 'E'
    s += nd !== 0 ? d[0] : "0";
    if (prec > 0) s += "." + d.slice(1, Math.min(nd, prec + 1)) + "0".repeat(Math.max(0, prec + 1 - Math.max(1, Math.min(nd, prec + 1))));
    let exp = nd === 0 ? 0 : dp - 1;
    s += String.fromCharCode(fmt) + (exp < 0 ? "-" : "+");
    exp = Math.abs(exp);
    return s + (exp < 10 ? "0" + exp : String(exp));
  }
  if (fmt === 102) { // 'f'
    s += dp > 0 ? d.slice(0, Math.min(nd, dp)) + "0".repeat(dp - Math.min(nd, dp)) : "0";
    if (prec > 0) {
      const lz = Math.min(prec, Math.max(0, -dp));
      const off = dp + lz;
      const m = Math.min(prec - lz, Math.max(0, nd - off));
      s += "." + "0".repeat(lz) + d.slice(off, off + m) + "0".repeat(Math.max(0, prec - lz - m));
    }
    return s;
  }
  return s.slice(neg ? 1 : 0) + "%" + String.fromCharCode(fmt);
}

// parseFloat64 is strconv.ParseFloat(s, 64) on the engine's number parsing
// once s has Go's syntax for a floating-point literal (readFloat and
// underscoreOK in internal/strconv): the value and 0, or 1 for ErrSyntax,
// 2 for ErrRange. s is a Go string; a valid one is ASCII.
export function parseFloat64(s: string): [number, number] {
  const sp = /^([+-]?)(?:(inf|infinity)|(nan))$/i.exec(s);
  if (sp !== null) {
    if (sp[3] !== undefined) return sp[1] === "" ? [NaN, 0] : [0, 1];
    return [sp[1] === "-" ? -Infinity : Infinity, 0];
  }
  const m = /^([+-]?)(?:0[xX]((?:[0-9a-fA-F_]*\.)?[0-9a-fA-F_]*)[pP]([+-]?[0-9][0-9_]*)|((?:[0-9_]*\.)?[0-9_]*)(?:[eE]([+-]?[0-9][0-9_]*))?)$/.exec(s);
  if (m === null || !/[0-9a-fA-F]/.test(m[2] ?? m[4]) || (m[4] !== undefined && !/[0-9]/.test(m[4]))) return [0, 1];
  if (s.includes("_") && !underscoreOK(s)) return [0, 1];
  let v: number;
  if (m[4] !== undefined) {
    v = Number(m[1] + (m[4] + (m[5] !== undefined ? "e" + m[5] : "")).replace(/_/g, ""));
  } else {
    v = hexFloat(m[2].replace(/_/g, ""), Math.max(-1e5, Math.min(1e5, Number(m[3].replace(/_/g, "")))));
    if (m[1] === "-") v = -v;
  }
  return v === Infinity || v === -Infinity ? [v, 2] : [v, 0];
}

// underscoreOK is internal/strconv's: underscores in s only separate digits
// (or a base prefix and a digit), as in Go literals.
function underscoreOK(s: string): boolean {
  let saw = "^", i = 0;
  if (s[0] === "+" || s[0] === "-") s = s.slice(1);
  let hex = false;
  if (s.length >= 2 && s[0] === "0" && "bBoOxX".includes(s[1])) {
    i = 2;
    saw = "0";
    hex = s[1] === "x" || s[1] === "X";
  }
  for (; i < s.length; i++) {
    const c = s[i];
    if ((c >= "0" && c <= "9") || (hex && /[a-fA-F]/.test(c))) {
      saw = "0";
      continue;
    }
    if (c === "_") {
      if (saw !== "0") return false;
      saw = "_";
      continue;
    }
    if (saw === "_") return false;
    saw = "!";
  }
  return saw !== "_";
}

// hexFloat is the hexadecimal mantissa h (with an optional point) times
// 2^p, rounded to even as a float64.
function hexFloat(h: string, p: number): number {
  const dot = h.indexOf(".");
  if (dot >= 0) {
    p -= 4 * (h.length - dot - 1);
    h = h.slice(0, dot) + h.slice(dot + 1);
  }
  let m = BigInt("0x" + (h || "0"));
  if (m === 0n) return 0;
  const bits = m.toString(2).length;
  const top = bits - 1 + p; // the exponent of m's leading bit
  if (top > 1023) return Infinity;
  const width = Math.min(53, top + 1075); // the bits a float64 keeps there
  let drop = bits - Math.max(width, 0);
  if (width <= 0) {
    // Below half the smallest subnormal, or at most it: it rounds to it
    // only if above the halfway point.
    return top === -1075 && m !== 1n << BigInt(bits - 1) ? 2 ** -1074 : 0;
  }
  if (drop > 0) {
    const d = BigInt(drop);
    const rest = m & ((1n << d) - 1n), half = 1n << (d - 1n);
    m >>= d;
    if (rest > half || (rest === half && (m & 1n) === 1n)) m++;
  } else {
    drop = 0;
  }
  // m now fits in 54 bits at most, and m·2^(p+drop) is exact unless it overflows.
  return Number(m) * 2 ** (p + drop);
}
