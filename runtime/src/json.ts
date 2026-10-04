// encoding/json's Marshal and Unmarshal for the common cases (see the
// encoding/json patch): values of booleans, numbers, strings, structs
// with plain json tags, slices, arrays, maps with string keys, pointers and
// interfaces, none of whose types has methods (so no Marshaler,
// TextMarshaler and the like is involved), are encoded and decoded here:
// encoded by JSON.stringify of their JS form (or by enc where that would
// differ from Go), decoded in one pass over the JSON text. Anything else (methods,
// embedded fields, other tag options, float32, invalid UTF-8, unusual
// escapes, errors of any kind, decoding into non-nil maps, slices,
// pointers or interfaces, which Go merges into) makes these return without
// result and without having changed anything, and Go's own code (json v2
// with v1 options) does the work and produces its exact output or error.

// Like natives.ts, which imports it, this uses the runtime only through its
// public module, so that split builds share one runtime.
import { Cell, GoMap, Iface, Kind, Slice, Type, bytesToString, fromJSString, makeMap, mapOf, ptrTo, sliceLit, sliceOf, stringToBytes, types } from "./index.ts";
import type { S } from "./index.ts";

// Abort is thrown to give up on the fast path.
const Abort = { abort: true };

function abort(): never {
  throw Abort;
}

interface FieldInfo {
  name: string; // the JSON name
  indexLike: boolean; // a name JS objects order first
  jsName: string; // the name as a JS string
  key: string; // the encoded name and colon
  bit: number; // 1 << index, 0 past 30 fields
  prop: string;
  type: Type;
  omitEmpty: boolean;
}

const fieldCache = new Map<Type, FieldInfo[] | null>();
const plainCache = new Map<Type, boolean>();

// plain reports whether neither t nor *t has methods.
function plain(t: Type): boolean {
  let p = plainCache.get(t);
  if (p === undefined) {
    p = t.methods.size === 0 && (t.kind === Kind.Interface || t.kind === Kind.Pointer || ptrTo(t).methods.size === 0);
    plainCache.set(t, p);
  }
  return p;
}

const deepCache = new Map<Type, boolean>();

// deepPlain reports whether values of type t can take the fast path: t
// and the types it contains are plain, of supported kinds and, for
// structs, with supported fields. Interfaces' dynamic types are checked
// as they are met.
function deepPlain(t: Type): boolean {
  let p = deepCache.get(t);
  if (p !== undefined) return p;
  deepCache.set(t, true); // while recursive types are checked
  p = !t.named || plain(t);
  if (p) {
    switch (t.kind) {
      case Kind.Bool: case Kind.String: case Kind.Float64: case Kind.Interface:
      case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
      case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
        break;
      case Kind.Struct: {
        const fs = structFields(t);
        p = fs !== null && fs.every((f) => deepPlain(f.type));
        break;
      }
      case Kind.Map:
        p = t.key!.kind === Kind.String && deepPlain(t.key!) && deepPlain(t.elem!);
        break;
      case Kind.Slice: case Kind.Array: case Kind.Pointer:
        p = deepPlain(t.elem!);
        break;
      default:
        p = false;
    }
  }
  deepCache.set(t, p);
  return p;
}

const nonASCII = /[\x80-\xff]/;
const tagRE = /(?:^|\s)json:"([^"\\]*)"/;
const nameRE = /^[A-Za-z0-9_-]+$/;

// structFields returns the JSON fields of struct type t, or null if they
// need Go's code.
function structFields(t: Type): FieldInfo[] | null {
  let fs = fieldCache.get(t);
  if (fs !== undefined) return fs;
  fs = [];
  const seen = new Set<string>();
  for (const f of t.fields) {
    if (f.embedded || f.tag.indexOf("\\") >= 0) { fs = null; break; }
    if (f.pkgPath !== "") continue; // unexported
    let name = f.name, omitEmpty = false;
    const m = tagRE.exec(f.tag);
    if (m !== null) {
      const parts = m[1].split(",");
      if (parts[0] === "-" && parts.length === 1) continue;
      if (parts[0] !== "") {
        if (!nameRE.test(parts[0])) { fs = null; break; }
        name = parts[0];
      }
      for (let i = 1; i < parts.length; i++) {
        if (parts[i] === "omitempty") omitEmpty = true;
        else { fs = null; break; }
      }
      if (fs === null) break;
    }
    if (seen.has(name.toLowerCase())) { fs = null; break; } // conflicts and case-insensitive matches
    seen.add(name.toLowerCase());
    fs.push({ name, indexLike: indexLike.test(name), jsName: jsString(name), key: '"' + name + '":', bit: fs.length < 30 ? 1 << fs.length : 0, prop: f.prop, type: f.type, omitEmpty });
  }
  fieldCache.set(t, fs);
  return fs;
}

// ---- Marshal ----

const hex = "0123456789abcdef";
const plainString = /^[\x20-\x21\x23-\x25\x27-\x3b\x3d\x3f-\x5b\x5d-\x7e]*$/; // no " & < > \ or controls

function quote(s: string): string {
  if (plainString.test(s)) return '"' + s + '"';
  let r = '"', start = 0;
  for (let i = 0; i < s.length; ) {
    const c = s.charCodeAt(i);
    if (c < 0x80) {
      let esc = "";
      switch (c) {
        case 0x22: esc = '\\"'; break;
        case 0x5c: esc = "\\\\"; break;
        case 0x0a: esc = "\\n"; break;
        case 0x0d: esc = "\\r"; break;
        case 0x09: esc = "\\t"; break;
        case 0x3c: case 0x3e: case 0x26: esc = "\\u00" + hex[c >> 4] + hex[c & 15]; break;
        default: if (c < 0x20) abort();
      }
      if (esc !== "") {
        r += s.slice(start, i) + esc;
        start = i + 1;
      }
      i++;
      continue;
    }
    // A multi-byte UTF-8 sequence, copied as it is when valid.
    const n = utf8Len(s, i);
    if (n === 0) abort();
    if (n === 3 && c === 0xe2 && s.charCodeAt(i + 1) === 0x80 && (s.charCodeAt(i + 2) & 0xfe) === 0xa8) {
      r += s.slice(start, i) + (s.charCodeAt(i + 2) === 0xa8 ? "\\u2028" : "\\u2029");
      start = i + 3;
    }
    i += n;
  }
  return r + s.slice(start) + '"';
}

// utf8Len returns the length of the valid UTF-8 sequence at s[i] (a byte
// >= 0x80), or 0.
function utf8Len(s: string, i: number): number {
  const c = s.charCodeAt(i);
  const b1 = s.charCodeAt(i + 1); // NaN past the end
  if (c >= 0xc2 && c <= 0xdf) return (b1 & 0xc0) === 0x80 ? 2 : 0;
  const b2 = s.charCodeAt(i + 2);
  if (c >= 0xe0 && c <= 0xef) {
    const lo = c === 0xe0 ? 0xa0 : 0x80, hi = c === 0xed ? 0x9f : 0xbf;
    return b1 >= lo && b1 <= hi && (b2 & 0xc0) === 0x80 ? 3 : 0;
  }
  const b3 = s.charCodeAt(i + 3);
  if (c >= 0xf0 && c <= 0xf4) {
    const lo = c === 0xf0 ? 0x90 : 0x80, hi = c === 0xf4 ? 0x8f : 0xbf;
    return b1 >= lo && b1 <= hi && (b2 & 0xc0) === 0x80 && (b3 & 0xc0) === 0x80 ? 4 : 0;
  }
  return 0;
}

function isEmpty(t: Type, v: any): boolean {
  switch (t.kind) {
    case Kind.Bool: return v === false;
    case Kind.String: return v === "";
    case Kind.Int64: case Kind.Uint64: return v === 0n;
    case Kind.Slice: return v === null || v.$length === 0;
    case Kind.Map: return v === null || v.entries.size === 0;
    case Kind.Array: return t.len === 0;
    case Kind.Pointer: case Kind.Interface: return v === null;
    case Kind.Struct: return false;
  }
  return v === 0;
}

// NoJS is thrown when JSON.stringify of the JS form of a value would not
// encode it as Go does (-0, integers past 2^53, object keys that JS orders
// as indices); enc then encodes it.
const NoJS = { nojs: true };
const indexLike = /^(?:\d+|__proto__)$/;
const utf8Dec = new TextDecoder();

// toJS returns the JS value whose JSON.stringify is the encoding of v,
// before HTML escaping.
function toJS(t: Type, v: any, depth: number): any {
  switch (t.kind) {
    case Kind.Bool:
    case Kind.Int8: case Kind.Int16: case Kind.Int32:
    case Kind.Uint8: case Kind.Uint16: case Kind.Uint32:
      return v;
    case Kind.Int: case Kind.Uint: case Kind.Uintptr:
      if (!Number.isSafeInteger(v)) throw NoJS;
      return v;
    case Kind.Int64: case Kind.Uint64: {
      const n = Number(v);
      if (!Number.isSafeInteger(n)) throw NoJS;
      return n;
    }
    case Kind.Float64:
      if (!Number.isFinite(v)) abort(); // an UnsupportedValueError
      if (v === 0 && 1 / v < 0) throw NoJS;
      return v;
    case Kind.String:
      return jsString(v);
    case Kind.Pointer:
      if (v === null) return null;
      if (depth > 100) abort(); // possibly a cycle
      return toJS(t.elem!, t.elem!.kind === Kind.Struct || t.elem!.kind === Kind.Array ? v : v.v, depth + 1);
    case Kind.Interface:
      if (v === null) return null;
      if (depth > 100 || !deepPlain(v.t)) abort();
      return toJS(v.t, v.v, depth + 1);
    case Kind.Struct: {
      const fs = structFields(t)!, o: any = { ...template(t, fs) };
      for (let i = 0; i < fs.length; i++) {
        const f = fs[i], x = v[f.prop];
        if (f.omitEmpty && isEmpty(f.type, x)) continue;
        if (f.indexLike) throw NoJS;
        o[f.jsName] = toJS(f.type, x, depth);
      }
      return o;
    }
    case Kind.Slice: {
      if (v === null) return null;
      const e = t.elem!;
      if (e.kind === Kind.Uint8) return btoa(bytesToString(v));
      if (depth > 100) abort();
      const a = [], arr = v.$array, o = v.$offset;
      for (let i = 0; i < v.$length; i++) a.push(toJS(e, arr[o + i], depth + 1));
      return a;
    }
    case Kind.Array: {
      const e = t.elem!;
      if (e.kind === Kind.Uint8) abort();
      const a = [];
      for (let i = 0; i < v.length; i++) a.push(toJS(e, v[i], depth));
      return a;
    }
    case Kind.Map: {
      if (v === null) return null;
      if (depth > 100) abort();
      const m = v as GoMap<string, any>, o: any = {};
      const ks: string[] = Array.from(m.entries.keys()); // direct: string keys
      if (ks.length > 1) ks.sort(); // in Go's order, which JS keeps for keys that are not indices
      for (let i = 0; i < ks.length; i++) {
        const k = ks[i];
        if (indexLike.test(k)) throw NoJS;
        o[jsString(k)] = toJS(t.elem!, m.entries.get(k), depth + 1);
      }
      return o;
    }
  }
  return abort();
}

const templates = new Map<Type, object>();

// template returns an object with the JSON fields of struct type t in
// order, if none is omitempty: copies of it share a hidden class and are
// filled in without adding properties.
function template(t: Type, fs: FieldInfo[]): object {
  let o = templates.get(t);
  if (o === undefined) {
    const p: any = {};
    if (!fs.some((f) => f.omitEmpty)) for (const f of fs) p[f.jsName] = null;
    templates.set(t, (o = p));
  }
  return o!;
}

// jsString converts a Go string to the JS string of its characters.
function jsString(s: string): string {
  if (!nonASCII.test(s)) return s;
  for (let i = 0; i < s.length; ) {
    if (s.charCodeAt(i) < 0x80) i++;
    else {
      const n = utf8Len(s, i);
      if (n === 0) abort(); // Go writes U+FFFD
      i += n;
    }
  }
  return utf8Dec.decode(stringToBytes(s).$array as unknown as Uint8Array);
}

const utf8Enc = new TextEncoder();
const htmlChars = /[<>&\u2028\u2029]/;
const htmlCharsAll = /[<>&\u2028\u2029]/g;
const htmlEscape = (c: string) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0");

function enc(t: Type, v: any, depth: number): string {
  switch (t.kind) {
    case Kind.Bool:
      return v ? "true" : "false";
    case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
    case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
      return typeof v === "number" && !Number.isSafeInteger(v) ? BigInt(v).toString() : String(v);
    case Kind.Float64:
      if (!Number.isFinite(v)) abort();
      if (v === 0) return Object.is(v, -0) ? "-0" : "0";
      return String(v); // ES6 number formatting, which jsonwire.AppendFloat reproduces
    case Kind.String:
      return quote(v);
    case Kind.Pointer:
      if (v === null) return "null";
      if (depth > 100) abort();
      return enc(t.elem!, t.elem!.kind === Kind.Struct || t.elem!.kind === Kind.Array ? v : v.v, depth + 1);
    case Kind.Interface:
      if (v === null) return "null";
      if (depth > 100 || !deepPlain(v.t)) abort();
      return enc(v.t, v.v, depth + 1);
    case Kind.Struct: {
      const fs = structFields(t)!;
      let r = "{", sep = "";
      for (let i = 0; i < fs.length; i++) {
        const f = fs[i], x = v[f.prop];
        if (f.omitEmpty && isEmpty(f.type, x)) continue;
        r += sep + f.key + enc(f.type, x, depth);
        sep = ",";
      }
      return r + "}";
    }
    case Kind.Slice: {
      if (v === null) return "null";
      const e = t.elem!;
      if (e.kind === Kind.Uint8) return '"' + btoa(bytesToString(v)) + '"';
      if (depth > 100) abort();
      let r = "[";
      for (let i = 0; i < v.$length; i++) r += (i > 0 ? "," : "") + enc(e, v.$array[v.$offset + i], depth + 1);
      return r + "]";
    }
    case Kind.Array: {
      const e = t.elem!;
      if (e.kind === Kind.Uint8) abort();
      let r = "[";
      for (let i = 0; i < v.length; i++) r += (i > 0 ? "," : "") + enc(e, v[i], depth);
      return r + "]";
    }
    case Kind.Map: {
      if (v === null) return "null";
      if (depth > 100) abort();
      const m = v as GoMap<string, any>;
      const ks: string[] = Array.from(m.entries.keys()); // direct: string keys
      if (ks.length > 1) ks.sort();
      let r = "{";
      for (let i = 0; i < ks.length; i++) r += (i > 0 ? "," : "") + quote(ks[i]) + ":" + enc(t.elem!, m.entries.get(ks[i]), depth + 1);
      return r + "}";
    }
  }
  return abort();
}

// jsonMarshal is json.Marshal(x), or null for Go's code to do it.
export function jsonMarshal(x: Iface | null): S<number> {
  if (x === null) return stringToBytes("null");
  if (!deepPlain(x.t)) return null;
  try {
    let js: any;
    try {
      js = toJS(x.t, x.v, 0);
    } catch (e) {
      if (e !== NoJS) throw e;
      return stringToBytes(enc(x.t, x.v, 0));
    }
    let out = JSON.stringify(js);
    if (htmlChars.test(out)) out = out.replace(htmlCharsAll, htmlEscape);
    // UTF-8: a short text is usually ASCII, which needs no encoding; the
    // engine's encoder pays off on longer ones.
    if (out.length < 256) return stringToBytes(fromJSString(out));
    const b = utf8Enc.encode(out);
    return new Slice(b as any, 0, b.length, b.length);

  } catch (e) {
    if (e === Abort) return null;
    throw e;
  }
}

// ---- Unmarshal ----

class Decoder {
  i = 0;
  declare s: string;
  constructor(s: string) {
    this.s = s;
  }

  // ws skips white space and returns the next character.
  ws(): number {
    const c = this.s.charCodeAt(this.i);
    return c > 0x20 ? c : this.skipWS();
  }

  skipWS(): number {
    const s = this.s;
    let i = this.i, c = s.charCodeAt(i);
    while (c === 0x20 || c === 0x0a || c === 0x0d || c === 0x09) c = s.charCodeAt(++i);
    this.i = i;
    return c;
  }

  expect(c: number): void {
    if (this.ws() !== c) abort();
    this.i++;
  }

  // literal consumes null, true or false if the input has it here.
  literal(word: string): boolean {
    if (this.s.startsWith(word, this.i)) {
      const c = this.s.charCodeAt(this.i + word.length);
      if (!(c >= 0x61 && c <= 0x7a)) { // not followed by more letters
        this.i += word.length;
        return true;
      }
    }
    return false;
  }

  str(): string {
    if (this.ws() !== 0x22) abort();
    const s = this.s;
    let i = this.i + 1, start = i, r = "";
    for (;;) {
      const c = s.charCodeAt(i);
      if (c === 0x22) break;
      if (c < 0x20 || c !== c) abort();
      if (c === 0x5c) {
        r += s.slice(start, i);
        const e = s.charCodeAt(i + 1);
        i += 2;
        switch (e) {
          case 0x22: r += '"'; break;
          case 0x5c: r += "\\"; break;
          case 0x2f: r += "/"; break;
          case 0x62: r += "\b"; break;
          case 0x66: r += "\f"; break;
          case 0x6e: r += "\n"; break;
          case 0x72: r += "\r"; break;
          case 0x74: r += "\t"; break;
          case 0x75: {
            let u = this.hex4(i);
            i += 4;
            if (u >= 0xd800 && u <= 0xdfff) {
              // A surrogate pair, else Go's U+FFFD: leave it to Go.
              if (u > 0xdbff || s.charCodeAt(i) !== 0x5c || s.charCodeAt(i + 1) !== 0x75) abort();
              const lo = this.hex4(i + 2);
              if (lo < 0xdc00 || lo > 0xdfff) abort();
              u = 0x10000 + ((u - 0xd800) << 10) + (lo - 0xdc00);
              i += 6;
            }
            r += utf8(u);
            break;
          }
          default: abort();
        }
        start = i;
        continue;
      }
      if (c >= 0x80) {
        const n = utf8Len(s, i);
        if (n === 0) abort();
        i += n;
        continue;
      }
      i++;
    }
    this.i = i + 1;
    return r + s.slice(start, i);
  }

  hex4(i: number): number {
    let u = 0;
    for (let k = 0; k < 4; k++) {
      const c = this.s.charCodeAt(i + k);
      const d = c >= 0x30 && c <= 0x39 ? c - 0x30 : c >= 0x61 && c <= 0x66 ? c - 0x57 : c >= 0x41 && c <= 0x46 ? c - 0x37 : -1;
      if (d < 0) abort();
      u = u * 16 + d;
    }
    return u;
  }

  // num consumes a JSON number and returns its text.
  num(): string {
    const s = this.s, start = this.ws();
    let i = this.i;
    if (start === 0x2d) i++;
    let c = s.charCodeAt(i);
    if (c === 0x30) c = s.charCodeAt(++i);
    else if (c >= 0x31 && c <= 0x39) { do c = s.charCodeAt(++i); while (c >= 0x30 && c <= 0x39); }
    else abort();
    if (c === 0x2e) {
      c = s.charCodeAt(++i);
      if (!(c >= 0x30 && c <= 0x39)) abort();
      do c = s.charCodeAt(++i); while (c >= 0x30 && c <= 0x39);
    }
    if (c === 0x65 || c === 0x45) {
      c = s.charCodeAt(++i);
      if (c === 0x2b || c === 0x2d) c = s.charCodeAt(++i);
      if (!(c >= 0x30 && c <= 0x39)) abort();
      do c = s.charCodeAt(++i); while (c >= 0x30 && c <= 0x39);
    }
    const t = s.slice(this.i, i);
    this.i = i;
    return t;
  }

  // skip consumes any JSON value.
  skip(depth: number): void {
    if (depth > 100) abort();
    const c = this.ws();
    if (c === 0x22) { this.str(); return; }
    if (c === 0x7b) {
      this.i++;
      if (this.ws() === 0x7d) { this.i++; return; }
      for (;;) {
        this.str();
        this.expect(0x3a);
        this.skip(depth + 1);
        const d = this.ws();
        this.i++;
        if (d === 0x7d) return;
        if (d !== 0x2c) abort();
      }
    }
    if (c === 0x5b) {
      this.i++;
      if (this.ws() === 0x5d) { this.i++; return; }
      for (;;) {
        this.skip(depth + 1);
        const d = this.ws();
        this.i++;
        if (d === 0x5d) return;
        if (d !== 0x2c) abort();
      }
    }
    if (this.literal("null") || this.literal("true") || this.literal("false")) return;
    this.num();
  }

  int(t: Type): any {
    const s = this.s, c0 = this.ws();
    if (t.kind !== Kind.Int64 && t.kind !== Kind.Uint64) {
      // Up to 15 digits, which a float64 holds exactly.
      let i = this.i, neg = false;
      if (c0 === 0x2d) { neg = true; i++; }
      const start = i;
      let n = 0, c = s.charCodeAt(i);
      if (c === 0x30) c = s.charCodeAt(++i);
      else while (c >= 0x30 && c <= 0x39 && i - start < 15) { n = n * 10 + (c - 0x30); c = s.charCodeAt(++i); }
      if (i > start && !(c >= 0x30 && c <= 0x39) && c !== 0x2e && c !== 0x65 && c !== 0x45) {
        this.i = i;
        return this.inRange(t, neg ? -n : n);
      }
    }
    if (c0 === 0x22) abort();
    const lit = this.num();
    if (!/^-?\d+$/.test(lit)) abort(); // a fraction or exponent: Go's error
    if (t.kind === Kind.Int64 || t.kind === Kind.Uint64) {
      const b = BigInt(lit);
      if (t.kind === Kind.Int64 ? BigInt.asIntN(64, b) !== b : BigInt.asUintN(64, b) !== b) abort();
      return b;
    }
    const n = Number(lit);
    if (!Number.isSafeInteger(n)) abort();
    return this.inRange(t, n);
  }

  inRange(t: Type, n: number): number {
    let lo = -(2 ** 53), hi = 2 ** 53;
    switch (t.kind) {
      case Kind.Int8: lo = -128; hi = 127; break;
      case Kind.Int16: lo = -32768; hi = 32767; break;
      case Kind.Int32: lo = -(2 ** 31); hi = 2 ** 31 - 1; break;
      case Kind.Uint8: lo = 0; hi = 255; break;
      case Kind.Uint16: lo = 0; hi = 65535; break;
      case Kind.Uint32: lo = 0; hi = 2 ** 32 - 1; break;
      case Kind.Uint: case Kind.Uintptr: lo = 0; break;
    }
    if (n < lo || n > hi) abort();
    return n + 0; // not -0
  }

  // any decodes a value into an empty interface of type t as Go does:
  // map[string]any, []any, float64, string, bool or nil.
  any(t: Type, depth: number): Iface | null {
    if (depth > 100) abort();
    const c = this.ws();
    switch (c) {
      case 0x22:
        return new Iface(types.string, this.str());
      case 0x7b: {
        this.i++;
        const mt = mapOf(types.string, t), m = makeMap(types.string);
        if (this.ws() === 0x7d) { this.i++; return new Iface(mt, m); }
        for (;;) {
          const k = this.str();
          this.expect(0x3a);
          if (m.entries.has(k)) abort();
          m.entries.set(k, this.any(t, depth + 1));
          const d = this.ws();
          this.i++;
          if (d === 0x7d) return new Iface(mt, m);
          if (d !== 0x2c) abort();
        }
      }
      case 0x5b: {
        this.i++;
        const a: any[] = [];
        const st = sliceOf(t);
        if (this.ws() === 0x5d) { this.i++; return new Iface(st, sliceLit(a)); }
        for (;;) {
          a.push(this.any(t, depth + 1));
          const d = this.ws();
          this.i++;
          if (d === 0x5d) return new Iface(st, sliceLit(a));
          if (d !== 0x2c) abort();
        }
      }
    }
    if (this.literal("null")) return null;
    if (this.literal("true")) return new Iface(types.bool, true);
    if (this.literal("false")) return new Iface(types.bool, false);
    const f = Number(this.num());
    if (!Number.isFinite(f)) abort();
    return new Iface(types.float64, f);
  }
}

function utf8(r: number): string {
  if (r < 0x80) return String.fromCharCode(r);
  if (r < 0x800) return String.fromCharCode(0xc0 | (r >> 6), 0x80 | (r & 0x3f));
  if (r < 0x10000) return String.fromCharCode(0xe0 | (r >> 12), 0x80 | ((r >> 6) & 0x3f), 0x80 | (r & 0x3f));
  return String.fromCharCode(0xf0 | (r >> 18), 0x80 | ((r >> 12) & 0x3f), 0x80 | ((r >> 6) & 0x3f), 0x80 | (r & 0x3f));
}

// A Dec decodes a JSON value of one type merged into the current value
// cur and returns the result (cur itself, changed in place, for a struct).
// depth counts the enclosing values, as a guard against deep nesting.
type Dec = (d: Decoder, cur: any, depth: number) => any;

const decCache = new Map<Type, Dec>();

// decoderOf returns the decoder for values of a deepPlain type t.
function decoderOf(t: Type): Dec {
  let f = decCache.get(t);
  if (f === undefined) {
    let real: Dec | null = null;
    decCache.set(t, (d, cur, depth) => real!(d, cur, depth)); // while recursive types are built
    real = makeDecoder(t);
    decCache.set(t, real);
    f = real;
  }
  return f;
}

// isNull consumes null if the input has it here.
const isNull = (d: Decoder, c: number) => c === 0x6e && d.literal("null");

const decBool: Dec = (d, cur) => {
  const c = d.ws();
  if (c === 0x74 && d.literal("true")) return true;
  if (c === 0x66 && d.literal("false")) return false;
  if (isNull(d, c)) return cur; // null leaves the value as it is
  return abort();
};

const decString: Dec = (d, cur) => {
  const c = d.ws();
  if (c === 0x22) return d.str();
  if (isNull(d, c)) return cur;
  return abort();
};

const decFloat: Dec = (d, cur) => {
  const c = d.ws();
  if (isNull(d, c)) return cur;
  if (c === 0x22) abort();
  const f = Number(d.num());
  if (!Number.isFinite(f)) abort();
  return f;
};

function makeDecoder(t: Type): Dec {
  switch (t.kind) {
    case Kind.Bool:
      return decBool;
    case Kind.String:
      return decString;
    case Kind.Float64:
      return decFloat;
    case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
    case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
      return (d, cur) => (isNull(d, d.ws()) ? cur : d.int(t));
    case Kind.Struct: {
      const fs = structFields(t)!, decs = fs.map((f) => decoderOf(f.type));
      return (d, cur, depth) => {
        if (depth > 100) abort();
        const c = d.ws();
        if (c !== 0x7b) return isNull(d, c) ? cur : abort();
        d.i++;
        if (d.ws() === 0x7d) { d.i++; return cur; }
        let done = 0, next = 0;
        for (;;) {
          // Fields usually come in their declared order, so the next one's
          // encoded name is tried first; field names need no escapes.
          let j = -1;
          if (next < fs.length && d.s.startsWith(fs[next].key, d.i)) {
            j = next;
            d.i += fs[j].key.length;
          } else {
            const k = d.str();
            d.expect(0x3a);
            j = fs.findIndex((g) => g.name === k);
            if (j < 0) {
              const lk = k.toLowerCase();
              if (nonASCII.test(k)) abort(); // Unicode case folding
              j = fs.findIndex((g) => g.name.toLowerCase() === lk);
            }
          }
          if (j < 0) d.skip(depth + 1);
          else {
            const f = fs[j];
            // A repeated name merges into the first value.
            if (f.bit === 0 || (done & f.bit) !== 0) abort();
            done |= f.bit;
            next = j + 1;
            const x = decs[j](d, cur[f.prop], depth + 1);
            if (f.type.kind !== Kind.Struct) cur[f.prop] = x;
          }
          const e = d.ws();
          d.i++;
          if (e === 0x7d) return cur;
          if (e !== 0x2c) abort();
        }
      };
    }
    case Kind.Slice: {
      const e = t.elem!;
      if (e.kind === Kind.Uint8 || e.kind === Kind.Array) return abort;
      const ed = decoderOf(e), ez = e.zero;
      return (d, cur, depth) => {
        if (depth > 100) abort();
        const c = d.ws();
        if (isNull(d, c)) return null;
        if (cur !== null || c !== 0x5b) abort(); // Go reuses a slice's elements
        d.i++;
        const a: any[] = [];
        if (d.ws() === 0x5d) { d.i++; return sliceLit(a); }
        for (;;) {
          a.push(ed(d, ez(), depth + 1));
          const c = d.ws();
          d.i++;
          if (c === 0x5d) return sliceLit(a);
          if (c !== 0x2c) abort();
        }
      };
    }
    case Kind.Map: {
      const e = t.elem!;
      if (e.kind === Kind.Array) return abort;
      const ed = decoderOf(e), ez = e.zero, kt = t.key!;
      return (d, cur, depth) => {
        if (depth > 100) abort();
        const c = d.ws();
        if (isNull(d, c)) return null;
        if (cur !== null || c !== 0x7b) abort(); // Go merges into a map
        d.i++;
        const m = makeMap(kt);
        if (d.ws() === 0x7d) { d.i++; return m; }
        for (;;) {
          const k = d.str();
          d.expect(0x3a);
          if (m.entries.has(k)) abort(); // Go merges into the first value
          m.entries.set(k, ed(d, ez(), depth + 1)); // direct: string keys
          const c = d.ws();
          d.i++;
          if (c === 0x7d) return m;
          if (c !== 0x2c) abort();
        }
      };
    }
    case Kind.Pointer: {
      const e = t.elem!;
      if (e.kind === Kind.Array) return abort;
      const ed = decoderOf(e), ez = e.zero, agg = e.kind === Kind.Struct;
      return (d, cur, depth) => {
        if (depth > 100) abort();
        if (isNull(d, d.ws())) return null;
        if (cur !== null) abort(); // Go decodes into the pointee
        const x = ed(d, ez(), depth + 1);
        return agg ? x : new Cell(x);
      };
    }
    case Kind.Interface:
      if (t.imethods.length > 0) return abort;
      return (d, cur, depth) => {
        if (isNull(d, d.ws())) return null;
        if (cur !== null) abort(); // Go decodes into the dynamic value
        return d.any(t, depth);
      };
  }
  return abort;
}

// jsonUnmarshal is json.Unmarshal(data, x) when it succeeds: it reports
// false, having changed nothing, for Go's code to do it.
export function jsonUnmarshal(data: S<number>, x: Iface | null): boolean {
  if (x === null || x.t.kind !== Kind.Pointer || x.v === null) return false;
  const e = x.t.elem!, p = x.v;
  const agg = e.kind === Kind.Struct || e.kind === Kind.Array;
  if (e.kind === Kind.Array || !deepPlain(e)) return false;
  try {
    const d = new Decoder(bytesToString(data));
    // A struct is decoded into a copy, which replaces it once all is well.
    const cur = agg ? p.$clone(e) : p.v;
    const v = decoderOf(e)(d, cur, 0);
    d.ws();
    if (d.i < d.s.length) return false; // trailing data: Go's syntax error
    if (agg) p.$set(v, e);
    else p.v = v;
    return true;
  } catch (err) {
    if (err === Abort) return false;
    throw err;
  }
}

