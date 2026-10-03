// Go strings are immutable byte sequences. They are represented as JS strings
// whose code units are the bytes (0..255), so len(s), s[i], slicing and
// comparison are exact and invalid UTF-8 survives round trips. Conversion to
// and from ordinary (UTF-16) JS strings happens only at the JS boundary via
// toJSString / fromJSString.

import { runtimePanic } from "./panic";
import { Slice, S } from "./slice";

export function strIndex(s: string, i: number): number {
  if (i < 0 || i >= s.length) runtimePanic(`index out of range [${i}] with length ${s.length}`);
  return s.charCodeAt(i);
}

export function substr(s: string, lo?: number, hi?: number): string {
  const l = lo ?? 0;
  const h = hi ?? s.length;
  if (l < 0 || h < l || h > s.length) runtimePanic(`slice bounds out of range [${l}:${h}] with length ${s.length}`);
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
  let s = "";
  for (let i = 0; i < b.$length; i++) s += String.fromCharCode(b.$array[b.$offset + i]);
  return s;
}

export function stringToBytes(s: string): Slice<number> {
  const a = new Array<number>(s.length);
  for (let i = 0; i < s.length; i++) a[i] = s.charCodeAt(i);
  return new Slice(a, 0, a.length, a.length);
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

// toJSString decodes a Go (byte) string as UTF-8 into a JS string.
export function toJSString(s: string): string {
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
  let out = "";
  for (const ch of s) out += encodeRune(ch.codePointAt(0)!);
  return out;
}
