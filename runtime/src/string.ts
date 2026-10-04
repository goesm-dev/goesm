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
    let r = utf8Dec.decode(v);
    if (r.length === n && r.indexOf("\ufffd") < 0) return r;
    // Its windows-1252 decoder is latin1 except for 0x80-0x9f, which it
    // maps above U+00FF.
    r = latin1.decode(v);
    if (!aboveLatin1.test(r)) return r;
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

const latin1 = new TextDecoder("latin1");
const utf8Dec = new TextDecoder();
const aboveLatin1 = /[\u0100-\uffff]/;
const utf8 = new TextEncoder();
const fitsASCII = (r: TextEncoderEncodeIntoResult, n: number) => r.read === n && r.written === n;

export function stringToBytes(s: string): Slice<number> {
  const a = newBytes(s.length);
  // The engine's UTF-8 encoder writes ASCII as is; other code units take two
  // bytes, so a string with them does not fit.
  if (s.length < 64 || !fitsASCII(utf8.encodeInto(s, a), s.length)) {
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

// toJSString decodes a Go (byte) string as UTF-8 into a JS string.
export function toJSString(s: string): string {
  if (isASCII(s)) return s;
  let out = "";
  for (let i = 0; i < s.length; ) {
    const [r, w] = decodeRune(s, i);
    out += String.fromCodePoint(r);
    i += w;
  }
  return out;
}

// fromJSString encodes a JS string as UTF-8 into a Go (byte) string.
export function fromJSString(s: string): string {
  if (isASCII(s)) return s;
  let out = "";
  for (const ch of s) out += encodeRune(ch.codePointAt(0)!);
  return out;
}
