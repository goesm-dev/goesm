// Go strings are immutable byte sequences. They are represented as JS strings
// whose code units are the bytes (0..255), so len(s), s[i], slicing and
// comparison are exact and invalid UTF-8 survives round trips. Conversion to
// and from ordinary (UTF-16) JS strings happens only at the JS boundary via
// toJSString / fromJSString.

import { indexError, runtimePanic, sliceError } from "./panic.ts";
import { newBytes, Slice } from "./slice.ts";
import type { S } from "./slice.ts";

export function strIndex(s: string, i: number): number {
  if (i < 0 || i >= s.length) indexError(i, s.length);
  return s.charCodeAt(i);
}

export function substr(s: string, lo?: number, hi?: number): string {
  const l = lo ?? 0;
  const h = hi ?? s.length;
  if (l < 0 || h < l || h > s.length) sliceError(l, h, undefined, s.length, "length");
  return s.substring(l, h);
}

export function encodeRune(r: number): string {
  if (r < 0 || r > 0x10ffff || (r >= 0xd800 && r <= 0xdfff)) r = 0xfffd;
  if (r < 0x80) return String.fromCharCode(r);
  if (r < 0x800) return String.fromCharCode(0xc0 | (r >> 6), 0x80 | (r & 0x3f));
  if (r < 0x10000) return String.fromCharCode(0xe0 | (r >> 12), 0x80 | ((r >> 6) & 0x3f), 0x80 | (r & 0x3f));
  return String.fromCharCode(0xf0 | (r >> 18), 0x80 | ((r >> 12) & 0x3f), 0x80 | ((r >> 6) & 0x3f), 0x80 | (r & 0x3f));
}

// decodeRune decodes the UTF-8 sequence at s[i]; returns [rune, width]. Invalid
// encodings yield [0xFFFD, 1] like utf8.DecodeRuneInString.
export function decodeRune(s: string, i: number): [number, number] {
  const n = s.length;
  const c0 = s.charCodeAt(i);
  if (c0 < 0x80) return [c0, 1];
  const cont = (j: number) => (j < n ? s.charCodeAt(j) : -1);
  const isCont = (c: number) => c >= 0x80 && c <= 0xbf;
  if (c0 >= 0xc2 && c0 <= 0xdf) {
    const c1 = cont(i + 1);
    if (isCont(c1)) return [((c0 & 0x1f) << 6) | (c1 & 0x3f), 2];
  } else if (c0 >= 0xe0 && c0 <= 0xef) {
    const c1 = cont(i + 1), c2 = cont(i + 2);
    const lo = c0 === 0xe0 ? 0xa0 : 0x80, hi = c0 === 0xed ? 0x9f : 0xbf;
    if (c1 >= lo && c1 <= hi && isCont(c2)) return [((c0 & 0x0f) << 12) | ((c1 & 0x3f) << 6) | (c2 & 0x3f), 3];
  } else if (c0 >= 0xf0 && c0 <= 0xf4) {
    const c1 = cont(i + 1), c2 = cont(i + 2), c3 = cont(i + 3);
    const lo = c0 === 0xf0 ? 0x90 : 0x80, hi = c0 === 0xf4 ? 0x8f : 0xbf;
    if (c1 >= lo && c1 <= hi && isCont(c2) && isCont(c3))
      return [((c0 & 0x07) << 18) | ((c1 & 0x3f) << 12) | ((c2 & 0x3f) << 6) | (c3 & 0x3f), 4];
  }
  return [0xfffd, 1];
}

export function bytesToString(b: S<number>): string {
  if (b === null) return "";
  const n = b.$length, a = b.$array, o = b.$offset;
  if (n <= 4) {
    let s = "";
    for (let i = 0; i < n; i++) s += String.fromCharCode(a[o + i]);
    return s;
  }
  if (a instanceof Uint8Array && n >= 64) {
    const v = a.subarray(o, o + n);
    // The engine's UTF-8 decoder is fastest, and right for ASCII: other
    // bytes either form multi-byte sequences, which shorten the result,
    // or become U+FFFD.
    const r = utf8Dec.decode(v);
    const valid = r.indexOf("\ufffd") < 0;
    if (valid && r.length === n) return r;
    // Valid UTF-8 is shorter as UTF-16 by about one unit per byte above
    // 0x7f.
    const l = valid && (n - r.length) * 16 < n ? spliced(r) : latin1Bytes(v);
    remember(l, v.slice(), valid ? r : null);
    return l;
  }
  // String.fromCharCode over chunks: one flat string instead of a rope of
  // one-character concatenations. The chunks stay below engines' argument
  // count limits. Engines spread an Array's elements as arguments faster
  // than a Uint8Array's, even counting the copy.
  let s = "";
  for (let i = 0; i < n; i += 8192) {
    const m = Math.min(8192, n - i);
    let c: number[];
    if (a instanceof Uint8Array) {
      c = new Array(m);
      for (let k = 0; k < m; k++) c[k] = a[o + i + k];
    } else {
      c = a.slice(o + i, o + i + m);
    }
    s += String.fromCharCode.apply(null, c);
  }
  return s;
}

const utf8 = /* @__PURE__ */ new TextEncoder();
// ignoreBOM keeps a leading U+FEFF, which Go keeps.
const utf8Dec = /* @__PURE__ */ new TextDecoder("utf-8", { ignoreBOM: true });
const utf16 = /* @__PURE__ */ new TextDecoder("utf-16le");

// latin1Bytes returns the string whose code units are the bytes of v
// through the engine's UTF-16 decoder, which is several times faster than
// String.fromCharCode, whatever the bytes hold.
function latin1Bytes(v: Uint8Array): string {
  const w = new Uint16Array(v.length);
  for (let i = 0; i < v.length; i++) w[i] = v[i];
  return utf16.decode(w);
}

// spliced returns the Go string of the UTF-8 encoding of s, a JS string
// with few non-ASCII characters: s's ASCII runs as they are, which engines
// share rather than copy, between the encodings of the others.
const nonASCIIRun = /[\u0080-\uffff]+/g;
function spliced(s: string): string {
  nonASCIIRun.lastIndex = 0;
  let out = "", last = 0, m: RegExpExecArray | null;
  while ((m = nonASCIIRun.exec(s)) !== null) {
    out += s.slice(last, m.index);
    const b = utf8.encode(m[0]);
    for (let i = 0; i < b.length; i++) out += String.fromCharCode(b[i]);
    last = m.index + m[0].length;
  }
  return out + s.slice(last);
}

const fitsASCII = (r: TextEncoderEncodeIntoResult, n: number) => r.read === n && r.written === n;

// The latest long non-ASCII string made from bytes, by fromJSString or
// bytesToString, a private copy of them, and its JS string when
// bytesToString decoded it. Programs often turn such a string back into
// bytes or into a JS string: []byte(s) of a string from JavaScript, or a
// result built in a bytes.Buffer. Strings over 1 MiB are not remembered, so
// as not to keep them alive.
let memoStr = "";
let memoBytes: Uint8Array | null = null;
let memoJS: string | null = null;

function remember(s: string, b: Uint8Array, js: string | null = null): void {
  if (b.length <= 1 << 20) {
    memoStr = s;
    memoBytes = b;
    memoJS = js;
  }
}

// remembered returns the bytes of s if s is the remembered string. The
// caller must not change them.
function remembered(s: string): Uint8Array | null {
  return memoBytes !== null && s.length === memoStr.length && s === memoStr ? memoBytes : null;
}

export function stringToBytes(s: string): Slice<number> {
  const a = newBytes(s.length);
  const m = remembered(s);
  if (m !== null) {
    a.set(m);
  } else if (s.length < 64 || !fitsASCII(utf8.encodeInto(s, a), s.length)) {
    // The engine's UTF-8 encoder writes ASCII as is; other code units take
    // two bytes, so a string with them does not fit.
    for (let i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
  }
  return new Slice(a as any, 0, a.length, a.length);
}

export function runesToString(r: S<number>): string {
  if (r === null) return "";
  let s = "";
  for (let i = 0; i < r.$length; i++) s += encodeRune(r.$array[r.$offset + i]);
  return s;
}

export function stringToRunes(s: string): Slice<number> {
  const a: number[] = [];
  for (let i = 0; i < s.length; ) {
    const [r, w] = decodeRune(s, i);
    a.push(r);
    i += w;
  }
  return new Slice(a, 0, a.length, a.length);
}

// nonASCII matches a code unit outside ASCII, where Go and JS strings
// differ.
const nonASCII = /[\u0080-\uffff]/;

// isASCII reports whether s holds only code units below 0x80, where a Go
// (byte) string and a JS string are the same. A string that JS passes to Go
// and gets back is checked at each step (fromJSString, strings.ToUpper,
// toJSString), each a scan of about 1 ns per unit; the two latest ASCII
// strings are remembered, so that checking one of them again is an identity
// (or, for an equal copy, a content) comparison. Strings over 4 KiB are not
// remembered, so as not to keep them alive.
let ascii0 = "";
let ascii1 = "";
export function isASCII(s: string): boolean {
  if (s === ascii0 || s === ascii1) return true;
  if (nonASCII.test(s)) return false;
  noteASCII(s);
  return true;
}

// noteASCII remembers s, known to be ASCII, for isASCII.
export function noteASCII(s: string): void {
  if (s.length <= 4096) {
    ascii1 = ascii0;
    ascii0 = s;
  }
}

// toJSString decodes a Go (byte) string as UTF-8 into a JS string. Both
// conversions collect code units and make the string with
// String.fromCharCode in chunks, which engines do several times faster
// than appending one character at a time.
export function toJSString(s: string): string {
  if (memoJS !== null && s.length === memoStr.length && s === memoStr) return memoJS;
  if (isASCII(s)) return s;
  const n = s.length;
  if (n >= 64) {
    // The engine's UTF-8 decoder agrees with Go's decoding of valid UTF-8.
    // It replaces some invalid sequences differently, so a result with
    // U+FFFD is made again below.
    let b = remembered(s);
    if (b === null) {
      b = new Uint8Array(n);
      for (let i = 0; i < n; i++) b[i] = s.charCodeAt(i);
    }
    const r = utf8Dec.decode(b);
    if (r.indexOf("\ufffd") < 0) return r;
  }
  const units: number[] = [];
  let out = "";
  for (let i = 0; i < n; ) {
    if (units.length >= chunk) {
      out += String.fromCharCode.apply(null, units);
      units.length = 0;
    }
    const c = s.charCodeAt(i);
    if (c < 0x80) {
      units.push(c);
      i++;
      continue;
    }
    // The common valid sequences inline, the rest through decodeRune.
    const c1 = s.charCodeAt(i + 1);
    if (c >= 0xc2 && c <= 0xdf && (c1 & 0xc0) === 0x80) {
      units.push(((c & 0x1f) << 6) | (c1 & 0x3f));
      i += 2;
    } else if (c >= 0xe1 && c <= 0xec && (c1 & 0xc0) === 0x80 && (s.charCodeAt(i + 2) & 0xc0) === 0x80) {
      units.push(((c & 0x0f) << 12) | ((c1 & 0x3f) << 6) | (s.charCodeAt(i + 2) & 0x3f));
      i += 3;
    } else {
      const [r, w] = decodeRune(s, i);
      if (r > 0xffff) units.push(0xd7c0 + (r >> 10), 0xdc00 | (r & 0x3ff));
      else units.push(r);
      i += w;
    }
  }
  return out + String.fromCharCode.apply(null, units);
}

// chunk bounds the arguments of one String.fromCharCode call.
const chunk = 8192;

// fromJSString encodes a JS string as UTF-8 into a Go (byte) string.
export function fromJSString(s: string): string {
  if (isASCII(s)) return s;
  const n = s.length;
  // The engine's UTF-8 encoder, like the loop below, encodes a lone
  // surrogate as U+FFFD.
  if (n >= 64) {
    const b = utf8.encode(s);
    const r = (b.length - n) * 16 < b.length ? spliced(s) : latin1Bytes(b);
    remember(r, b);
    return r;
  }
  const units: number[] = [];
  let out = "";
  for (let i = 0; i < n; i++) {
    if (units.length >= chunk) {
      out += String.fromCharCode.apply(null, units);
      units.length = 0;
    }
    let r = s.charCodeAt(i);
    if (r < 0x80) {
      units.push(r);
    } else if (r < 0x800) {
      units.push(0xc0 | (r >> 6), 0x80 | (r & 0x3f));
    } else {
      if (r >= 0xd800 && r <= 0xdbff && i + 1 < n) {
        const lo = s.charCodeAt(i + 1);
        if (lo >= 0xdc00 && lo <= 0xdfff) {
          r = 0x10000 + ((r - 0xd800) << 10) + (lo - 0xdc00);
          i++;
          units.push(0xf0 | (r >> 18), 0x80 | ((r >> 12) & 0x3f), 0x80 | ((r >> 6) & 0x3f), 0x80 | (r & 0x3f));
          continue;
        }
      }
      if (r >= 0xd800 && r <= 0xdfff) r = 0xfffd; // a lone surrogate
      units.push(0xe0 | (r >> 12), 0x80 | ((r >> 6) & 0x3f), 0x80 | (r & 0x3f));
    }
  }
  return out + String.fromCharCode.apply(null, units);
}
