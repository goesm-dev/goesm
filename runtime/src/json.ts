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
//
// The json.Unmarshal calls goesm lowers go to jsonDecode instead, which
// also decodes what the one pass gives up on, errors included, for most of
// those types: a program whose every call decodes such values leaves out
// encoding/json (see "Unmarshal with Go's errors" below).

// Like natives.ts, which imports it, this uses the runtime only through its
// public module, so that split builds share one runtime.
import {
  BoxMode, Cell, GoMap, Iface, Kind, Slice, Type, addMethods, append, bytesToString, errorType, fromJSString, funcOf, hasMethods, interfaceOf, makeMap, mapOf,
  named, noteASCII, ptrIface, ptrTo, reach, setBox, setUnderlying, sliceLit, sliceOf, stringToBytes, structOf, tBool, tFloat64, tInt64, tString, topString, typeArgsName,
} from "./index.ts";
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
    p = !hasMethods(t) && (t.kind === Kind.Interface || t.kind === Kind.Pointer || !hasMethods(ptrTo(t)));
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
const utf8Dec = /* @__PURE__ */ new TextDecoder();

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

const utf8Enc = /* @__PURE__ */ new TextEncoder();
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
  const out = jsonText(x);
  if (out === null) return null;
  // UTF-8: a short text is usually ASCII, which needs no encoding; the
  // engine's encoder pays off on longer ones.
  if (textIsGo || out.length < 256) return stringToBytes(textIsGo ? out : fromJSString(out));
  const b = utf8Enc.encode(out);
  return new Slice(b as any, 0, b.length, b.length);
}

// jsonMarshalString is string(json.Marshal(x)) for a result that goesm
// keeps as a string (see the lowering's json.go), or null for Go's code to
// do it.
export function jsonMarshalString(x: Iface | null): string | null {
  const out = jsonText(x);
  return out === null || textIsGo ? out : fromJSString(out);
}

// textIsGo reports whether jsonText's last text is a Go (byte) string,
// rather than a JS string to encode as UTF-8.
let textIsGo = false;

// jsonText is the JSON encoding of x, or null.
function jsonText(x: Iface | null): string | null {
  textIsGo = true;
  if (x === null) return "null";
  if (!deepPlain(x.t)) return null;
  try {
    let js: any;
    try {
      js = toJS(x.t, x.v, 0);
    } catch (e) {
      if (e !== NoJS) throw e;
      return enc(x.t, x.v, 0);
    }
    let out = JSON.stringify(js);
    if (htmlChars.test(out)) out = out.replace(htmlCharsAll, htmlEscape);
    textIsGo = false;
    return out;
  } catch (e) {
    if (e === Abort) return null;
    throw e;
  }
}

// ---- Generated encoders ----
//
// The lowering generates an encoder for json.Marshal of a value whose
// static type it knows (see the lowering's jsonenc.go): a function from the
// value to the JS value whose JSON.stringify is its encoding. These are its
// helpers. Strings go into the JS value as they are, and one scan of the
// text finds whether any had bytes past ASCII or HTML characters; the
// encoder then runs again with jsonStr converting such strings.

let encCareful = false; // jsonStr converts the strings
let encSlow = false; // jsonStr converted a string
const encSpecial = /[\x80-\xff<>&]/;

export function jsonStr(s: string): string {
  return encCareful ? carefulStr(s) : s;
}

function carefulStr(s: string): string {
  if (!encSpecial.test(s)) return s;
  encSlow = true;
  return jsString(s);
}

// jsonStrs is the JS value of the strings arr[off:off+n] of a []string: the
// array itself when it holds just them and they go into the text as they
// are, as JSON.stringify writes an array's elements without changing it.
export function jsonStrs(arr: string[], off: number, n: number): string[] {
  if (!encCareful && off === 0 && n === arr.length) return arr;
  const a = new Array<string>(n);
  for (let i = 0; i < n; i++) a[i] = jsonStr(arr[off + i]);
  return a;
}

export function jsonKey(k: string): string {
  if (indexLike.test(k)) throw NoJS;
  return jsonStr(k);
}

export function jsonInt(n: number): number {
  if (!Number.isSafeInteger(n)) throw NoJS;
  return n;
}

export function jsonInt64(b: bigint): number {
  const n = Number(b);
  if (!Number.isSafeInteger(n)) throw NoJS;
  return n;
}

export function jsonFloat(f: number): number {
  if (!Number.isFinite(f)) abort(); // an UnsupportedValueError
  if (f === 0 && 1 / f < 0) throw NoJS;
  return f;
}

export function jsonAbort(): never {
  abort();
}

// encodedText is the JSON text of v by the generated encoder f, or null for
// the runtime's encoder (or Go's code) to do it.
function encodedText(f: (v: any, d: number) => any, v: any): string | null {
  const out = plainText(f, v);
  if (out === null) return null;
  if (!encSpecial.test(out)) {
    textIsGo = true; // ASCII, the same as its UTF-8
    noteASCII(out); // for toJSString when an exported function returns it
    return out;
  }
  return carefulText(f, v);
}

// plainText is the text of f with the strings as they are, or null.
function plainText(f: (v: any, d: number) => any, v: any): string | null {
  try {
    encCareful = false;
    return JSON.stringify(f(v, 0));
  } catch (e) {
    if (e === NoJS || e === Abort) return null;
    throw e;
  }
}

// carefulText is the text of f with jsonStr converting the strings that
// need it, or null.
function carefulText(f: (v: any, d: number) => any, v: any): string | null {
  try {
    encCareful = true;
    encSlow = false;
    let out = JSON.stringify(f(v, 0));
    textIsGo = !encSlow;
    if (encSlow && htmlChars.test(out)) out = out.replace(htmlCharsAll, htmlEscape);
    return out;
  } catch (e) {
    if (e === NoJS || e === Abort) return null;
    throw e;
  } finally {
    encCareful = false;
  }
}

// jsonMarshalWith is json.Marshal(v) by the generated encoder f, or null.
export function jsonMarshalWith(f: (v: any, d: number) => any, v: any): S<number> | null {
  let out = plainText(f, v);
  if (out === null) return null;
  if (out.length >= 256) {
    // The plain text is the encoding when it is ASCII without HTML
    // characters. Its UTF-8 finds a byte past ASCII faster than a scan
    // would, as each encodes to two, and is the result when there is none.
    const b = utf8Enc.encode(out);
    if (b.length === out.length && out.indexOf("<") < 0 && out.indexOf(">") < 0 && out.indexOf("&") < 0) {
      return new Slice(b as any, 0, b.length, b.length);
    }
  } else if (!encSpecial.test(out)) {
    return stringToBytes(out);
  }
  out = carefulText(f, v);
  if (out === null) return null;
  // An ASCII text is its own UTF-8, which the engine's encoder copies.
  if (!textIsGo && out.length < 256) return stringToBytes(fromJSString(out));
  const b = utf8Enc.encode(out);
  return new Slice(b as any, 0, b.length, b.length);
}

// jsonMarshalStringWith is jsonMarshalString(v) by the generated encoder f,
// or null.
export function jsonMarshalStringWith(f: (v: any, d: number) => any, v: any): string | null {
  const out = encodedText(f, v);
  return out === null || textIsGo ? out : fromJSString(out);
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

  // at reports whether the input has word here. A loop of charCodeAt,
  // which the engines inline, is several times faster than startsWith with
  // a position on these short words.
  at(word: string): boolean {
    const s = this.s, i = this.i, n = word.length;
    for (let k = 0; k < n; k++) if (s.charCodeAt(i + k) !== word.charCodeAt(k)) return false;
    return true;
  }

  // literal consumes null, true or false if the input has it here.
  literal(word: string): boolean {
    if (this.at(word)) {
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
    if (c0 === 0x2d && t.kind >= Kind.Uint) abort(); // -0 too is Go's error for unsigned integers
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
        return new Iface(tString, this.str());
      case 0x7b: {
        this.i++;
        const mt = mapOf(tString, t), m = makeMap(tString);
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
    if (this.literal("true")) return new Iface(tBool, true);
    if (this.literal("false")) return new Iface(tBool, false);
    const f = Number(this.num());
    if (!Number.isFinite(f)) abort();
    return new Iface(tFloat64, f);
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
          if (next < fs.length && d.at(fs[next].key)) {
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
      const refs = e.kind > Kind.Float64; // numbers and booleans keep nothing alive
      // The elements are collected in a scratch array and copied out at the
      // array's exact length, as JSON.parse makes its arrays: an array grown
      // by push keeps room for at least 16 elements, several times what a
      // small array needs. A nested array of the same type, which finds the
      // scratch array taken, and one after an abort use a new one.
      let scratch: any[] | null = [];
      return (d, cur, depth) => {
        if (depth > 100) abort();
        const c = d.ws();
        if (isNull(d, c)) return null;
        if (cur !== null || c !== 0x5b) abort(); // Go reuses a slice's elements
        d.i++;
        if (d.ws() === 0x5d) { d.i++; return sliceLit([]); }
        const a = scratch ?? [];
        scratch = null;
        for (let n = 0; ;) {
          a[n++] = ed(d, ez(), depth + 1);
          const c = d.ws();
          d.i++;
          if (c === 0x5d) {
            const r = a.slice(0, n);
            if (refs) a.fill(null, 0, n); // not to keep the elements alive
            if (n <= 4096) scratch = a;
            return sliceLit(r);
          }
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
  return jsonUnmarshalString(bytesToString(data), x);
}

// jsonUnmarshalString is jsonUnmarshal of []byte(s), which goesm calls for
// json.Unmarshal([]byte(s), v) without converting s.
export function jsonUnmarshalString(s: string, x: Iface | null): boolean {
  return x !== null && x.t.kind === Kind.Pointer && jsonUnmarshalTo(s, x.t, x.v);
}

// The decoder of jsonUnmarshalTo, which no Go code runs during. It is made
// on first use, as a top-level value would keep the decoder in every bundle.
let dec: Decoder | null = null;

// jsonUnmarshalTo is jsonUnmarshalString of the pointer p of type t, which
// goesm calls without boxing p when it has the pointer's type.
export function jsonUnmarshalTo(s: string, t: Type, p: any): boolean {
  const e = t.elem!;
  if (p === null || e.kind === Kind.Array || !deepPlain(e)) return false;
  const agg = e.kind === Kind.Struct;
  const d = (dec ??= new Decoder(""));
  d.s = s;
  d.i = 0;
  try {
    // A struct is decoded into a copy, which replaces it once all is well.
    const cur = agg ? p.$clone(e) : p.v;
    const v = decoderOf(e)(d, cur, 0);
    d.ws();
    if (d.i < s.length) return false; // trailing data: Go's syntax error
    if (agg) p.$set(v, e);
    else p.v = v;
    return true;
  } catch (err) {
    if (err === Abort) return false;
    throw err;
  } finally {
    d.s = "";
  }
}


// ---- Unmarshal with Go's errors ----
//
// jsonDecode is json.Unmarshal of the JSON text s into the pointer p of
// type t, errors included: when the one-pass decoder above gives up, the
// input is checked as Go's json v2 with v1 options checks it (jsonSyntax),
// and then decoded into the value in place with v1's legacy semantics, in
// which a type error is recorded (the first one is reported) and decoding
// goes on. Values of the types fullPlain accepts never need Go's code; for
// the others (an interface holding a pointer, for one, which Go decodes
// into) jsonDecode returns undefined, having changed nothing, unless
// strict is set, when goesm has checked at compile time that they cannot
// occur.

// fullPlain reports whether jsonDecode decodes every value of type t
// itself: t is deepPlain but has no arrays, the names of its structs' fields
// are ASCII and its interfaces are empty.
const fullCache = new Map<Type, boolean>();

function fullPlain(t: Type): boolean {
  let p = fullCache.get(t);
  if (p !== undefined) return p;
  fullCache.set(t, true); // while recursive types are checked
  p = deepPlain(t);
  if (p) {
    switch (t.kind) {
      case Kind.Array:
        p = false;
        break;
      case Kind.Interface:
        p = t.imethods.length === 0;
        break;
      case Kind.Struct:
        p = structFields(t)!.every((f) => !nonASCII.test(f.name) && fullPlain(f.type));
        break;
      case Kind.Map:
        p = fullPlain(t.elem!);
        break;
      case Kind.Slice: case Kind.Pointer:
        p = t.elem!.kind !== Kind.Array && (t.kind !== Kind.Slice || t.elem!.kind !== Kind.Uint8) && fullPlain(t.elem!);
        break;
    }
  }
  fullCache.set(t, p);
  return p;
}

// mergesIfaces reports whether decoding into a value of type t may decode
// into what an interface in it holds: whether t has interfaces on the paths
// that v1 merges into (fields, pointees and the elements of slices, whose
// arrays are reused; not map elements, which are always decoded afresh).
const mergeCache = new Map<Type, boolean>();

function mergesIfaces(t: Type): boolean {
  let p = mergeCache.get(t);
  if (p !== undefined) return p;
  mergeCache.set(t, false); // while recursive types are checked
  switch (t.kind) {
    case Kind.Interface: p = true; break;
    case Kind.Struct: p = structFields(t)!.some((f) => mergesIfaces(f.type)); break;
    case Kind.Slice: case Kind.Pointer: p = mergesIfaces(t.elem!); break;
    default: p = false;
  }
  mergeCache.set(t, p);
  return p;
}

// holdsPointer reports whether an interface in v, of type t, on the paths of
// mergesIfaces, holds a non-nil pointer, which Go decodes into. seen holds
// the pointees already looked at.
function holdsPointer(t: Type, v: any, seen: Set<any> | null): boolean {
  if (v === null || !mergesIfaces(t)) return false;
  switch (t.kind) {
    case Kind.Interface:
      return v.t.kind === Kind.Pointer && v.v !== null;
    case Kind.Struct:
      return structFields(t)!.some((f) => holdsPointer(f.type, v[f.prop], seen));
    case Kind.Pointer:
      if ((seen ??= new Set()).has(v)) return false;
      seen.add(v);
      return holdsPointer(t.elem!, t.elem!.kind === Kind.Struct ? v : v.v, seen);
    case Kind.Slice:
      for (let i = 0; i < v.$capacity; i++) if (holdsPointer(t.elem!, v.$array[v.$offset + i], seen)) return true;
  }
  return false;
}

// The errors of json.Unmarshal: encoding/json's own *SyntaxError,
// *UnmarshalTypeError and *InvalidUnmarshalError, made by functions the
// package registers (see the encoding/json patch) if the program has it.
// A program that only decodes in the runtime leaves the package out; its
// errors are then of the types below, which nothing can tell from the
// package's own: the same names, fields and messages. Their Type field is
// a reflect.Type if the program has reflect, which is the only way to see
// the field without naming the type.
interface JSONErrors {
  syntax(msg: string, off: bigint): Iface;
  type(value: string, ptr: Iface, off: bigint, struct: string, field: string): Iface;
  invalid(ptr: Iface): Iface;
}
let jsonErrors: JSONErrors | null = null;
let reflectTypeOf: ((ptr: Iface) => Iface) | null = null;
let reflectTypeType: Type | null = null; // reflect.Type

export function registerJSONErrors(e: JSONErrors): void {
  jsonErrors = e;
}

// registerTypeOf is called by package reflect with a function returning
// reflect.TypeOf(ptr).Elem() and a *reflect.Type.
export function registerTypeOf(f: (ptr: Iface) => Iface, typePtr: Iface): void {
  reflectTypeOf = f;
  reflectTypeType = typePtr.t.elem!;
}

const tErrMsg = /* @__PURE__ */ funcOf([], [tString], false);

// shadowErrors makes encoding/json's error types for a program without the
// package.
function shadowErrors(): JSONErrors {
  const pkg = "encoding/json";
  class SyntaxError {
    declare msg: string;
    declare Offset: bigint;
    constructor(msg: string, off: bigint) { this.msg = msg; this.Offset = off; }
    $clone() { return new SyntaxError(this.msg, this.Offset); }
    $set(o: SyntaxError) { this.msg = o.msg; this.Offset = o.Offset; }
  }
  class UnmarshalTypeError {
    declare Value: string; declare Type: Iface | null; declare Offset: bigint; declare Struct: string; declare Field: string; declare Err: Iface | null;
    constructor(value: string, type: Iface | null, off: bigint, struct: string, field: string, err: Iface | null) {
      this.Value = value; this.Type = type; this.Offset = off; this.Struct = struct; this.Field = field; this.Err = err;
    }
    $clone() { return new UnmarshalTypeError(this.Value, this.Type, this.Offset, this.Struct, this.Field, this.Err); }
    $set(o: UnmarshalTypeError) { this.Value = o.Value; this.Type = o.Type; this.Offset = o.Offset; this.Struct = o.Struct; this.Field = o.Field; this.Err = o.Err; }
  }
  class InvalidUnmarshalError {
    declare Type: Iface | null;
    constructor(type: Iface | null) { this.Type = type; }
    $clone() { return new InvalidUnmarshalError(this.Type); }
    $set(o: InvalidUnmarshalError) { this.Type = o.Type; }
  }
  // The type a reflect.Type describes, kept beside the field for Error.
  const described = new WeakMap<object, Type>();
  const reflectType = (ptr: Iface, elem: boolean): Iface | null => (reflectTypeOf === null ? null : elem ? reflectTypeOf(ptr) : reflectTypeOf(new Iface(ptrTo(ptr.t), null)));
  const typeIface = reflectTypeType ?? interfaceOf([]);
  const def = (name: string, ctor: any, fields: [string, Type][], error: (e: any) => string, unwrap: boolean): Type => {
    const t = named(pkg, name);
    setUnderlying(t, structOf(fields, ctor, pkg), ctor);
    const methods: Record<string, [(recv: any) => any, Type]> = { Error: [error, tErrMsg] };
    if (unwrap) methods.Unwrap = [(e: any) => e.Err, funcOf([], [errorType], false)];
    const pt = ptrTo(t);
    addMethods(pt, methods);
    setBox(pt, ctor, BoxMode.Self);
    return pt;
  };
  const tSyntax = def("SyntaxError", SyntaxError, [["msg", tString], ["Offset", tInt64]], (e) => e.msg, false);
  const tType = def("UnmarshalTypeError", UnmarshalTypeError, [["Value", tString], ["Type", typeIface], ["Offset", tInt64], ["Struct", tString], ["Field", tString], ["Err", errorType]], (e) => {
    const ts = topString(described.get(e)!);
    if (e.Struct === "" && e.Field === "") return "json: cannot unmarshal " + e.Value + " into Go value of type " + ts;
    const last = e.Field.slice(e.Field.lastIndexOf(".") + 1);
    return "json: cannot unmarshal " + e.Value + " into " + (/^[0-9]+$/.test(last) ? "" : "Go struct field ") + e.Struct + "." + e.Field + " of type " + ts;
  }, true);
  const tInvalid = def("InvalidUnmarshalError", InvalidUnmarshalError, [["Type", typeIface]], (e) => "json: Unmarshal(nil " + topString(described.get(e)!) + ")", false);
  return {
    syntax: (msg, off) => ptrIface(tSyntax, new SyntaxError(msg, off)),
    type: (value, ptr, off, struct, field) => {
      const e = new UnmarshalTypeError(value, reflectType(ptr, true), off, struct, field, null);
      described.set(e, ptr.t.elem!);
      return ptrIface(tType, e);
    },
    invalid: (ptr) => {
      const e = new InvalidUnmarshalError(reflectType(ptr, false));
      described.set(e, ptr.t);
      return ptrIface(tInvalid, e);
    },
  };
}

function errorsOf(): JSONErrors {
  return (jsonErrors ??= shadowErrors());
}

// ---- Go's quoting, for the messages of syntax errors ----

const printRE = /[\p{L}\p{M}\p{N}\p{P}\p{S}]/u;

// isPrint is unicode.IsPrint.
function isPrint(r: number): boolean {
  return r < 0x80 ? r >= 0x20 && r < 0x7f : printRE.test(String.fromCodePoint(r));
}

// isSpace is unicode.IsSpace.
function isSpace(r: number): boolean {
  return (r >= 0x09 && r <= 0x0d) || r === 0x20 || r === 0x85 || r === 0xa0 || r === 0x1680 || (r >= 0x2000 && r <= 0x200a) ||
    r === 0x2028 || r === 0x2029 || r === 0x202f || r === 0x205f || r === 0x3000;
}

// runeAt decodes the rune at s[i] as utf8.DecodeRuneInString does: its
// value (0xFFFD if invalid) and width.
function runeAt(s: string, i: number): [number, number] {
  const c = s.charCodeAt(i);
  if (c < 0x80) return [c, 1];
  const n = utf8Len(s, i);
  if (n === 0) return [0xfffd, 1];
  let r = c & (0xff >> (n + 1));
  for (let k = 1; k < n; k++) r = (r << 6) | (s.charCodeAt(i + k) & 0x3f);
  return [r, n];
}

const escapes: Record<number, string> = { 7: "\\a", 8: "\\b", 12: "\\f", 10: "\\n", 13: "\\r", 9: "\\t", 11: "\\v" };

// escapedRune appends r, of width n at s[i], as strconv does between quotes q.
function escapedRune(s: string, i: number, r: number, n: number, q: number): string {
  if (r === q || r === 0x5c) return "\\" + String.fromCharCode(r);
  if (isPrint(r)) return s.slice(i, i + n);
  const e = escapes[r];
  if (e !== undefined) return e;
  if (r < 0x20 || r === 0x7f) return "\\x" + r.toString(16).padStart(2, "0");
  if (r < 0x10000) return "\\u" + r.toString(16).padStart(4, "0");
  return "\\U" + r.toString(16).padStart(8, "0");
}

// quoteRuneAt is jsonwire.QuoteRune of s[i:].
function quoteRuneAt(s: string, i: number): string {
  const [r, n] = runeAt(s, i);
  if (r === 0xfffd && n === 1) return "'\\x" + s.charCodeAt(i).toString(16) + "'";
  return "'" + escapedRune(s, i, r, n, 0x27) + "'";
}

// goQuote is strconv.Quote.
function goQuote(s: string): string {
  let q = '"';
  for (let i = 0; i < s.length;) {
    const [r, n] = runeAt(s, i);
    q += r === 0xfffd && n === 1 ? "\\x" + s.charCodeAt(i).toString(16).padStart(2, "0") : escapedRune(s, i, r, n, 0x22);
    i += n;
  }
  return q + '"';
}

// invalidText is the message of a jsonwire.InvalidTextError for the text
// what, as encoding/json reports it.
function invalidText(label: string, what: string, where: string): string {
  let runes = 0, escape = false;
  for (let i = 0; i < what.length;) {
    const [r, n] = runeAt(what, i);
    runes++;
    if (r === 0x60 || r === 0xfffd || isSpace(r) || !isPrint(r)) escape = true;
    i += n;
  }
  const w = runes === 1 ? quoteRuneAt(what, 0) : escape ? goQuote(what) : "`" + what + "`";
  return "invalid " + label + " " + w + " " + where;
}

// ---- Syntax ----

const unexpectedEnd = "unexpected end of JSON input";

// A SyntaxErr is the message and offset of a *json.SyntaxError.
type SyntaxErr = [string, number];

function invalidChar(s: string, i: number, where: string): SyntaxErr {
  return [invalidText("character", s.slice(i, i + runeAt(s, i)[1]), where), i + runeAt(s, i)[1]];
}

function invalidEscape(s: string, i: number, j: number): SyntaxErr {
  return [invalidText("escape sequence", s.slice(i, j), "in string"), j];
}

function isWS(c: number): boolean {
  return c === 0x20 || c === 0x0a || c === 0x0d || c === 0x09;
}

function isHex(c: number): boolean {
  return (c >= 0x30 && c <= 0x39) || (c >= 0x61 && c <= 0x66) || (c >= 0x41 && c <= 0x46);
}

// fullRune is utf8.FullRuneInString(s[i:]) for s[i] >= 0x80 not starting
// a valid sequence: whether s ends before the sequence could complete.
function truncatedRune(s: string, i: number): boolean {
  const c = s.charCodeAt(i), rest = s.length - i;
  const need = c >= 0xc2 && c <= 0xdf ? 2 : c >= 0xe0 && c <= 0xef ? 3 : c >= 0xf0 && c <= 0xf4 ? 4 : 1;
  if (rest >= need) return false;
  if (rest > 1) {
    const b1 = s.charCodeAt(i + 1);
    const lo = c === 0xe0 ? 0xa0 : c === 0xf0 ? 0x90 : 0x80, hi = c === 0xed ? 0x9f : c === 0xf4 ? 0x8f : 0xbf;
    if (b1 < lo || b1 > hi) return false;
    if (rest > 2 && (s.charCodeAt(i + 2) & 0xc0) !== 0x80) return false;
  }
  return true;
}

// escapedUTF16Prefix is jsonwire's hasEscapedUTF16Prefix(s[i:], false).
function escapedUTF16Prefix(s: string, i: number): boolean {
  for (let k = 0; i + k < s.length; k++) {
    const c = s.charCodeAt(i + k);
    if ((k === 0 && c !== 0x5c) || (k === 1 && c !== 0x75) || (k >= 2 && k < 6 && !isHex(c))) return false;
  }
  return true;
}

// stringEnd returns the end of the JSON string at s[i] (a quote), or its
// syntax error, as jsontext checks a string with invalid UTF-8 allowed.
function stringEnd(s: string, i: number): number | SyntaxErr {
  const n = s.length;
  let j = i + 1;
  for (;;) {
    if (j >= n) return [unexpectedEnd, n];
    const c = s.charCodeAt(j);
    if (c === 0x22) return j + 1;
    if (c < 0x20) return invalidChar(s, j, "in string");
    if (c === 0x5c) {
      if (j + 2 > n) return [unexpectedEnd, j];
      const e = s.charCodeAt(j + 1);
      if (e === 0x75) {
        if (j + 6 > n) return escapedUTF16Prefix(s, j) ? [unexpectedEnd, j] : invalidEscape(s, j, n);
        for (let k = 2; k < 6; k++) if (!isHex(s.charCodeAt(j + k))) return invalidEscape(s, j, j + 6);
        j += 6;
      } else if (e === 0x22 || e === 0x5c || e === 0x2f || e === 0x62 || e === 0x66 || e === 0x6e || e === 0x72 || e === 0x74) {
        j += 2;
      } else {
        return invalidEscape(s, j, j + 2);
      }
      continue;
    }
    if (c >= 0x80) {
      const k = utf8Len(s, j);
      if (k > 0) j += k;
      else if (truncatedRune(s, j)) return [unexpectedEnd, j];
      else j++;
      continue;
    }
    j++;
  }
}

// numberEnd returns the end of the JSON number at s[i] ('-' or a digit), or
// its syntax error.
function numberEnd(s: string, i: number): number | SyntaxErr {
  const n = s.length;
  const digit = (j: number) => { const c = s.charCodeAt(j); return c >= 0x30 && c <= 0x39; };
  let j = i;
  if (s.charCodeAt(j) === 0x2d) j++;
  if (j >= n) return [unexpectedEnd, i];
  if (s.charCodeAt(j) === 0x30) j++;
  else if (digit(j)) { do j++; while (digit(j)); }
  else return invalidChar(s, j, "in numeric literal");
  if (s.charCodeAt(j) === 0x2e) {
    j++;
    if (j >= n) return [unexpectedEnd, i];
    if (!digit(j)) return invalidChar(s, j, "in numeric literal");
    do j++; while (digit(j));
  }
  const c = s.charCodeAt(j);
  if (c === 0x65 || c === 0x45) {
    j++;
    const d = s.charCodeAt(j);
    if (d === 0x2b || d === 0x2d) j++;
    if (j >= n) return [unexpectedEnd, i];
    if (!digit(j)) return invalidChar(s, j, "in numeric literal");
    do j++; while (digit(j));
  }
  return j;
}

const maxDepth = 10000;

// jsonSyntax returns the *json.SyntaxError's message and offset for s, if
// it is not one valid JSON value: json.Unmarshal checks the whole input
// before it decodes anything.
export function jsonSyntax(s: string): SyntaxErr | null {
  const n = s.length;
  const ws = (j: number) => { while (j < n && isWS(s.charCodeAt(j))) j++; return j; };
  const stack: number[] = [];
  let i = ws(0);
  if (i >= n) return [unexpectedEnd, n];
  // The states: 0, a value is next; 1, an object member's name is next;
  // 2, a value has ended.
  let state = 0;
  for (;;) {
    if (state === 0) {
      const c = s.charCodeAt(i);
      let end: number | SyntaxErr;
      if (c === 0x7b || c === 0x5b) {
        if (stack.length === maxDepth) return ["exceeded max depth", i + 1];
        stack.push(c);
        i = ws(i + 1);
        if (i >= n) return [unexpectedEnd, n];
        if (s.charCodeAt(i) === c + 2) { // } or ]
          stack.pop();
          i++;
          state = 2;
        } else {
          state = c === 0x7b ? 1 : 0;
        }
        continue;
      } else if (c === 0x22) {
        end = stringEnd(s, i);
      } else if (c === 0x6e || c === 0x74 || c === 0x66) {
        const lit = c === 0x6e ? "null" : c === 0x74 ? "true" : "false";
        end = i + lit.length;
        for (let k = 0; k < lit.length; k++) {
          if (i + k >= n) { end = [unexpectedEnd, n]; break; }
          if (s.charCodeAt(i + k) !== lit.charCodeAt(k)) {
            end = invalidChar(s, i + k, "in literal " + lit + " (expecting '" + lit[k] + "')");
            break;
          }
        }
      } else if (c === 0x2d || (c >= 0x30 && c <= 0x39)) {
        end = numberEnd(s, i);
      } else {
        return invalidChar(s, i, "looking for beginning of value");
      }
      if (typeof end !== "number") return end;
      i = end;
      state = 2;
    } else if (state === 1) {
      if (s.charCodeAt(i) !== 0x22) return invalidChar(s, i, "looking for beginning of object key string");
      const end = stringEnd(s, i);
      if (typeof end !== "number") return end;
      i = ws(end);
      if (i >= n) return [unexpectedEnd, n];
      if (s.charCodeAt(i) !== 0x3a) return invalidChar(s, i, "after object key");
      i = ws(i + 1);
      if (i >= n) return [unexpectedEnd, n];
      state = 0;
    } else {
      i = ws(i);
      if (stack.length === 0) return i < n ? invalidChar(s, i, "after top-level value") : null;
      if (i >= n) return [unexpectedEnd, n];
      const top = stack[stack.length - 1], c = s.charCodeAt(i);
      if (c === 0x2c) {
        i = ws(i + 1);
        if (i >= n) return [unexpectedEnd, n];
        state = top === 0x7b ? 1 : 0;
      } else if (c === top + 2) {
        stack.pop();
        i++;
      } else {
        return invalidChar(s, i, top === 0x7b ? "after object key:value pair" : "after array element");
      }
    }
  }
}

// ---- Decoding with v1's semantics ----

// kindName is the Value of an UnmarshalTypeError for a JSON value that
// starts with c.
function kindName(c: number): string {
  switch (c) {
    case 0x22: return "string";
    case 0x74: case 0x66: return "bool";
    case 0x5b: return "array";
    case 0x7b: return "object";
    case 0x6e: return "null";
  }
  return "number";
}

// foldKey is the ASCII lower case of an object member name if it can equal
// an ASCII name under strings.EqualFold (with the Kelvin sign and the long
// s, the two non-ASCII runes that fold to ASCII letters), or null.
function foldKey(k: string): string | null {
  if (nonASCII.test(k)) {
    k = k.replace(/\xe2\x84\xaa/g, "k").replace(/\xc5\xbf/g, "s");
    if (nonASCII.test(k)) return null;
  }
  return k.toLowerCase();
}

const anyType = /* @__PURE__ */ interfaceOf([]);
const fffd = "\xef\xbf\xbd";

// Full decodes JSON text that jsonSyntax accepts into Go values as json v2
// with v1's options does, in place, recording the first type error.
class Full {
  i = 0;
  err: Iface | null = null;
  path: (string | number)[] = []; // the object names and array indices down to the value
  declare s: string;
  declare root: string; // the name of the type pointed to
  constructor(s: string, root: string) {
    this.s = s;
    this.root = root;
  }

  ws(): number {
    const s = this.s;
    let i = this.i, c = s.charCodeAt(i);
    while (c === 0x20 || c === 0x0a || c === 0x0d || c === 0x09) c = s.charCodeAt(++i);
    this.i = i;
    return c;
  }

  // fail records the type error of the value that started at start with c
  // and that was just read, unless there was one before.
  fail(t: Type, c: number, start: number, lit: string | null): void {
    if (this.err !== null) return;
    const off = c === 0x5b || c === 0x7b ? start + 1 : this.i;
    const field = this.path.map((p) => (typeof p === "number" ? String(p) : p.replace(/~/g, "~0").replace(/\//g, "~1"))).join(".");
    this.err = errorsOf().type(kindName(c) + (lit === null ? "" : " " + lit), new Iface(ptrTo(t), null), BigInt(off), this.path.length > 0 ? this.root : "", field);
  }

  // mismatch skips the value of the wrong kind for t and records the error.
  mismatch(t: Type, c: number): void {
    const start = this.i;
    this.skip();
    this.fail(t, c, start, null);
  }

  skip(): void {
    const s = this.s, c = this.ws();
    let j = this.i;
    if (c === 0x22) {
      j = this.strEnd(j);
    } else if (c === 0x7b || c === 0x5b) {
      for (let depth = 0; ;) {
        const d = s.charCodeAt(j);
        if (d === 0x22) { j = this.strEnd(j); continue; }
        j++;
        if (d === 0x7b || d === 0x5b) depth++;
        else if ((d === 0x7d || d === 0x5d) && --depth === 0) break;
      }
    } else if (c === 0x66) {
      j += 5;
    } else if (c === 0x6e || c === 0x74) {
      j += 4;
    } else {
      this.num();
      return;
    }
    this.i = j;
  }

  strEnd(j: number): number {
    const s = this.s;
    for (j++; ;) {
      const c = s.charCodeAt(j);
      if (c === 0x22) return j + 1;
      j += c === 0x5c ? 2 : 1;
    }
  }

  num(): string {
    const s = this.s, start = this.i;
    let i = start, c = s.charCodeAt(i);
    while ((c >= 0x30 && c <= 0x39) || c === 0x2d || c === 0x2b || c === 0x2e || c === 0x65 || c === 0x45) c = s.charCodeAt(++i);
    this.i = i;
    return s.slice(start, i);
  }

  // str reads a string as jsonwire.AppendUnquote does: invalid UTF-8 and
  // escaped surrogates that are not a pair become U+FFFD.
  str(): string {
    const s = this.s;
    let i = this.i + 1, start = i, r = "";
    for (;;) {
      const c = s.charCodeAt(i);
      if (c === 0x22) break;
      if (c === 0x5c) {
        r += s.slice(start, i);
        const e = s.charCodeAt(i + 1);
        i += 2;
        switch (e) {
          case 0x62: r += "\b"; break;
          case 0x66: r += "\f"; break;
          case 0x6e: r += "\n"; break;
          case 0x72: r += "\r"; break;
          case 0x74: r += "\t"; break;
          case 0x75: {
            let u = parseInt(s.slice(i, i + 4), 16);
            i += 4;
            if (u >= 0xd800 && u <= 0xdfff) {
              const lo = s.charCodeAt(i) === 0x5c && s.charCodeAt(i + 1) === 0x75 ? hexValue(s, i + 2) : -1;
              if (u <= 0xdbff && lo >= 0xdc00 && lo <= 0xdfff) {
                u = 0x10000 + ((u - 0xd800) << 10) + (lo - 0xdc00);
                i += 6;
              } else {
                u = 0xfffd;
              }
            }
            r += utf8(u);
            break;
          }
          default: r += String.fromCharCode(e); // " \ /
        }
        start = i;
        continue;
      }
      if (c >= 0x80) {
        const n = utf8Len(s, i);
        if (n === 0) {
          r += s.slice(start, i) + fffd;
          start = ++i;
        } else {
          i += n;
        }
        continue;
      }
      i++;
    }
    this.i = i + 1;
    return r + s.slice(start, i);
  }

  int(t: Type, cur: any, c: number, start: number): any {
    const lit = this.num();
    const neg = c === 0x2d, digits = neg ? lit.slice(1) : lit;
    let ok = /^(?:0|[1-9][0-9]*)$/.test(digits);
    let b = 0n;
    if (ok) {
      b = BigInt(lit);
      let bits = 64;
      switch (t.kind) {
        case Kind.Int8: case Kind.Uint8: bits = 8; break;
        case Kind.Int16: case Kind.Uint16: bits = 16; break;
        case Kind.Int32: case Kind.Uint32: bits = 32; break;
      }
      const signed = t.kind <= Kind.Int64;
      ok = signed ? BigInt.asIntN(bits, b) === b : !neg && BigInt.asUintN(bits, b) === b;
    }
    if (!ok) {
      this.fail(t, c, start, lit);
      return cur;
    }
    return t.kind === Kind.Int64 || t.kind === Kind.Uint64 ? b : Number(b);
  }

  // val decodes the next value, of type t, merged into cur, and returns the
  // result: cur itself, changed in place, for a struct.
  val(t: Type, cur: any): any {
    const c = this.ws(), start = this.i;
    if (c === 0x6e) {
      this.i += 4;
      // null clears maps, slices, pointers and interfaces, and leaves the
      // other values as they are.
      const k = t.kind;
      return k === Kind.Map || k === Kind.Slice || k === Kind.Pointer || k === Kind.Interface ? null : cur;
    }
    const num = c === 0x2d || (c >= 0x30 && c <= 0x39);
    switch (t.kind) {
      case Kind.Bool:
        if (c === 0x74 || c === 0x66) {
          this.i += c === 0x74 ? 4 : 5;
          return c === 0x74;
        }
        break;
      case Kind.String:
        if (c === 0x22) return this.str();
        break;
      case Kind.Float64:
        if (num) {
          const lit = this.num(), f = Number(lit);
          if (!Number.isFinite(f)) this.fail(t, c, start, lit); // ±Inf, as strconv.ParseFloat returns
          return f;
        }
        break;
      case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
      case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
        if (num) return this.int(t, cur, c, start);
        break;
      case Kind.Struct:
        if (c === 0x7b) return this.struct(t, cur);
        break;
      case Kind.Map:
        if (c === 0x7b) return this.map(t, cur);
        break;
      case Kind.Slice:
        if (c === 0x5b) return this.slice(t, cur);
        break;
      case Kind.Pointer: {
        // A nil pointer gets a new value even if decoding into it fails.
        const e = t.elem!;
        if (e.kind === Kind.Struct) {
          const p = cur ?? e.zero();
          this.val(e, p);
          return p;
        }
        const p = cur ?? new Cell(e.zero());
        p.v = this.val(e, p.v);
        return p;
      }
      case Kind.Interface:
        // An interface holding no pointer (see holdsPointer) gets the value
        // decoded as an any.
        switch (c) {
          case 0x74: case 0x66: return new Iface(tBool, this.val(tBool, false));
          case 0x22: return new Iface(tString, this.str());
          case 0x7b: return new Iface(mapOf(tString, anyType), this.map(mapOf(tString, anyType), null));
          case 0x5b: return new Iface(sliceOf(anyType), this.slice(sliceOf(anyType), null));
        }
        return new Iface(tFloat64, this.val(tFloat64, 0));
    }
    this.mismatch(t, c);
    return cur;
  }

  struct(t: Type, cur: any): any {
    const fs = structFields(t)!;
    this.i++;
    if (this.ws() === 0x7d) { this.i++; return cur; }
    for (;;) {
      const k = this.str();
      this.ws();
      this.i++; // :
      let f = fs.find((g) => g.name === k);
      if (f === undefined) {
        const fk = foldKey(k);
        if (fk !== null) f = fs.find((g) => g.name.toLowerCase() === fk);
      }
      if (f === undefined) this.skip();
      else {
        this.path.push(k);
        const x = this.val(f.type, cur[f.prop]);
        if (f.type.kind !== Kind.Struct) cur[f.prop] = x;
        this.path.pop();
      }
      const d = this.ws();
      this.i++;
      if (d === 0x7d) return cur;
      this.ws();
    }
  }

  map(t: Type, cur: any): any {
    const m = cur ?? makeMap(t.key!), e = t.elem!;
    this.i++;
    if (this.ws() === 0x7d) { this.i++; return m; }
    for (;;) {
      const k = this.str();
      this.ws();
      this.i++; // :
      this.path.push(k);
      // An existing element is replaced, not merged into.
      m.entries.set(k, this.val(e, e.zero()));
      this.path.pop();
      const d = this.ws();
      this.i++;
      if (d === 0x7d) return m;
      this.ws();
    }
  }

  slice(t: Type, cur: any): any {
    const e = t.elem!, agg = e.kind === Kind.Struct;
    // The elements are decoded into the slice's array, to its capacity
    // (into what it held there before), which grows as reflect's Grow
    // grows it.
    let sl: Slice<any> | null = cur === null || cur.$capacity === 0 ? null : new Slice(cur.$array, cur.$offset, cur.$capacity, cur.$capacity);
    let cap = sl === null ? 0 : sl.$capacity;
    this.i++;
    let n = 0;
    if (this.ws() !== 0x5d) {
      for (;;) {
        if (n === cap) {
          sl = append(sl, [e.zero()], e.zero, e) as Slice<any>;
          cap = sl.$capacity;
          sl = new Slice(sl.$array, sl.$offset, cap, cap);
        }
        this.path.push(n);
        const j = sl!.$offset + n;
        if (agg) reach(sl!.$array, j, j + 1); // spare elements made on first use
        const x = this.val(e, sl!.$array[j]);
        if (!agg) sl!.$array[j] = x;
        this.path.pop();
        n++;
        const d = this.ws();
        this.i++;
        if (d === 0x5d) break;
        this.ws();
      }
    } else {
      this.i++;
    }
    if (n === 0) return new Slice([], 0, 0, 0);
    return new Slice(sl!.$array, sl!.$offset, n, cap);
  }
}

function hexValue(s: string, i: number): number {
  for (let k = 0; k < 4; k++) if (!isHex(s.charCodeAt(i + k))) return -1;
  return parseInt(s.slice(i, i + 4), 16);
}

// rootName is reflect.Type's Name of t.
function rootName(t: Type): string {
  if (t.named) return typeArgsName(t);
  return t.kind <= Kind.Complex128 || t.kind === Kind.String ? t.str : "";
}

// jsonDecode is json.Unmarshal([]byte(s), p) for the pointer p of type t:
// it returns the error (null if none), or undefined, having changed
// nothing, for Go's code to decode the value (never if strict). (It is typed
// any for the lowering's temporaries, which hold the result.)
export function jsonDecode(s: string, t: Type, p: any, strict: boolean): any {
  if (p === null) return errorsOf().invalid(new Iface(t, null));
  const e = t.elem!, agg = e.kind === Kind.Struct;
  if (!fullPlain(e)) return giveUp(strict);
  if (jsonUnmarshalTo(s, t, p)) return null;
  const se = jsonSyntax(s);
  if (se !== null) return errorsOf().syntax(se[0], BigInt(se[1]));
  if (holdsPointer(e, agg ? p : p.v, null)) return giveUp(strict);
  const d = new Full(s, rootName(e));
  if (agg) d.val(e, p);
  else p.v = d.val(e, p.v);
  return d.err;
}

function giveUp(strict: boolean): undefined {
  if (strict) throw new Error("goesm: json.Unmarshal: a value goesm decodes itself needs encoding/json");
  return undefined;
}
