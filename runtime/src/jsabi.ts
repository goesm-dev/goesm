// The JS calling ABI: how Go values cross into JavaScript and back when Go
// calls a function or reads a value imported with //goesm:import
// (internal/lower/jsimport.go). The lowering converts strings and numbers
// inline and calls goToJS and jsToGo with the type descriptor for the rest.
//
//	| Go                               | JavaScript                         |
//	| -------------------------------- | ---------------------------------- |
//	| bool, integers, floats           | boolean, number                    |
//	| int64, uint64                    | bigint                             |
//	| string                           | string                             |
//	| js.Value, js.Func                | the value itself                   |
//	| []byte                           | Uint8Array (a copy)                |
//	| other slices and arrays          | Array (a copy)                     |
//	| map[string]T                     | plain object (a copy)              |
//	| struct, *struct                  | plain object of the exported       |
//	|                                  | fields named as encoding/json does |
//	| func (Go to JS only)             | function                           |
//	| any                              | the dynamic value's conversion; to |
//	|                                  | Go, JSON-shaped values             |
//
// Everything is copied: a slice or struct changed on the other side does not
// change the original.
//
// The exported functions and methods of the package a build starts from are
// called from JavaScript through wrappers (exportWrapper in
// internal/lower/jsexport.go) that use the same table the other way round,
// with argToGo and resultToJS. There, Go values without a plain JavaScript
// counterpart pass as they are, as handles JavaScript gives back to Go: a
// pointer to a struct type with methods, a non-empty interface, a channel.
// A Go value JavaScript passes back where its type is expected is taken as
// it is, a struct value copied. An error result is thrown as a GoError.

// jsabi.ts is a separate module (@goesm/runtime/jsabi), imported only by the
// modules that use //goesm:import and by entry modules whose export wrappers
// convert more than numbers; like natives.ts, it uses the runtime only
// through its public module, so split builds share one runtime.
import {
  GoMap, GoPanic, Goexit, Iface, Kind, ProgramExit, Slice, Type, addMethods, box, cell, classTypes, errorType, fromJSString, hasMethods,
  fromRef, funcOf, goThrown, implementsIface, interfaceOf, makeMap, mapOf, mapRange, mapSet, named, newBytes, plainPanic,
  ptrTo, setUnderlying, sliceOf, toJSString, toRef,
  tBool, tFloat64, tInt64, tString, tUnsafePointer,
} from "./index.ts";

function jsValueKind(t: Type): number {
  if (t.pkgPath !== "syscall/js") return 0;
  if (t.name === "Value") return 1;
  if (t.name === "Func") return 2;
  return 0;
}

// jsString converts a JS value Go declared a string to a Go string.
export function jsString(x: any): string {
  return typeof x === "string" ? fromJSString(x) : x === null || x === undefined ? "" : fromJSString(String(x));
}

export function jsInt(x: any): number {
  return Math.trunc(Number(x)) || 0;
}

export function jsInt64(x: any): bigint {
  return BigInt.asIntN(64, typeof x === "bigint" ? x : BigInt(jsInt(x)));
}

export function jsUint64(x: any): bigint {
  return BigInt.asUintN(64, typeof x === "bigint" ? x : BigInt(jsInt(x)));
}

// goToJS converts the Go value v of type t to a JavaScript value. ex is set
// for the results of exported functions, where Go values without a
// JavaScript counterpart are handles (see the top of this file).
export function goToJS(t: Type, v: any, ex = false): any {
  switch (t.kind) {
    case Kind.String:
      return toJSString(v);
    case Kind.Slice: {
      if (v === null) return null;
      const s = v as Slice<any>, a = s.$array, o = s.$offset, n = s.$length;
      if (t.elem!.kind === Kind.Uint8) {
        if (a instanceof Uint8Array) return a.slice(o, o + n);
        const b = new Uint8Array(n);
        for (let i = 0; i < n; i++) b[i] = a[o + i];
        return b;
      }
      const out = new Array(n);
      if (plain(t.elem!)) for (let i = 0; i < n; i++) out[i] = a[o + i];
      else for (let i = 0; i < n; i++) out[i] = goToJS(t.elem!, a[o + i], ex);
      return out;
    }
    case Kind.Array:
      return plain(t.elem!) ? (v as any[]).slice() : (v as any[]).map((x) => goToJS(t.elem!, x, ex));
    case Kind.Map: {
      if (v === null) return null;
      if (t.key!.kind !== Kind.String) {
        if (!ex) plainPanic(`goesm: a ${t.str} cannot be passed to JavaScript`);
        const m = new Map();
        for (const [k, x] of mapRange(v as GoMap<any, any>)) m.set(goToJS(t.key!, k, ex), goToJS(t.elem!, x, ex));
        return m;
      }
      const o: Record<string, any> = {};
      for (const [k, x] of mapRange(v as GoMap<any, any>)) setProp(o, toJSString(k), goToJS(t.elem!, x, ex));
      return o;
    }
    case Kind.Struct: {
      switch (jsValueKind(t)) {
        case 1: return fromRef(v.ref);
        case 2: return fromRef(v.Value.ref);
      }
      const o: Record<string, any> = {};
      const fs = jsFields(t);
      for (const f of fs) {
        if (!f.flatten) setProp(o, f.name, goToJS(f.type, v[f.prop], ex));
        else {
          const p = goToJS(f.type, v[f.prop], ex && f.type.kind !== Kind.Pointer); // null for a nil *T: no fields, as encoding/json
          // As in encoding/json, a field of the outer struct hides one of
          // the same name further down.
          if (p !== null) for (const k of Object.keys(p)) if (!fs.direct.has(k) && !Object.hasOwn(o, k)) setProp(o, k, p[k]);
        }
      }
      return o;
    }
    case Kind.Pointer:
      if (v === null) return null;
      if (ex && isHandle(t)) return v;
      return goToJS(t.elem!, t.elem!.kind === Kind.Struct || t.elem!.kind === Kind.Array ? v : v.v, ex);
    case Kind.Interface:
      if (v === null) return null;
      if (ex) {
        if (t === errorType) return goError(v);
        if (t.imethods.length > 0) return v;
      }
      return goToJS(v.t, v.v, ex);
    case Kind.Func:
      return v === null ? null : goFuncToJS(t, v, ex);
    case Kind.Complex64: case Kind.Complex128: case Kind.Chan: case Kind.UnsafePointer:
      if (ex) return v;
      plainPanic(`goesm: a ${t.str} cannot be passed to JavaScript`);
  }
  return v;
}

// setProp sets the property k of o, as an own property also when k is
// "__proto__", which an assignment would take for o's prototype.
function setProp(o: Record<string, any>, k: string, x: any): void {
  if (k === "__proto__") Object.defineProperty(o, k, { value: x, enumerable: true, writable: true, configurable: true });
  else o[k] = x;
}

// isHandle reports whether the pointer type t crosses to JavaScript as the Go
// object itself: a pointer to a struct type with methods.
function isHandle(t: Type): boolean {
  return t.elem!.kind === Kind.Struct && hasMethods(t);
}

// plainSliceToJS, stringsToJS and stringsToGo are goToJS and jsToGo for
// slices of booleans and numbers other than bytes, and of strings. Export
// wrappers call them directly, so that a bundle that converts no more
// leaves the rest of this module out.
export function plainSliceToJS(s: Slice<any> | null): any[] | null {
  if (s === null) return null;
  const a = s.$array, o = s.$offset, n = s.$length, out = new Array(n);
  for (let i = 0; i < n; i++) out[i] = a[o + i];
  return out;
}

export function stringsToJS(s: Slice<string> | null): string[] | null {
  if (s === null) return null;
  const a = s.$array, o = s.$offset, n = s.$length, out = new Array<string>(n);
  for (let i = 0; i < n; i++) out[i] = toJSString(a[o + i]);
  return out;
}

export function stringsToGo(x: any): Slice<string> | null {
  if (x === null || x === undefined) return null;
  if (x instanceof Slice) return x;
  const n = x.length >>> 0, a = new Array<string>(n);
  for (let i = 0; i < n; i++) a[i] = jsString(x[i]);
  return new Slice(a, 0, n, n);
}

// resultToJS converts the result v of type t of an exported Go function for
// its JavaScript caller. methods is the entry module's $jsm, passed so that
// the handles' JS methods are kept where handles can cross.
export function resultToJS(t: Type, v: any, methods?: unknown): any {
  return goToJS(t, v, true);
}

// argToGo converts the argument x JavaScript passes to an exported Go
// function for a parameter of type t.
export function argToGo(t: Type, x: any, methods?: unknown): any {
  return jsToGo(t, x, true);
}

// GoError is what JavaScript receives for a Go error: an exported function's
// error result is thrown as one. Passed back to Go where an error is
// expected, it is the Go error again.
export class GoError extends Error {
  declare value: Iface;
  constructor(value: Iface) {
    const m = value.t.methods.get("Error");
    super(m ? toJSString(m.fn(value.v)) : value.t.str);
    this.value = value;
    this.name = "GoError";
  }
}

// goError is the GoError of the non-nil error err.
export function goError(err: Iface): GoError {
  return new GoError(err);
}

interface JSField {
  prop: string; // the Go value's property
  name: string; // the JS object's
  type: Type;
  flatten: boolean; // an embedded struct or *struct without a json name: its fields are the object's
}

// jsFields are the fields of struct type t that cross the boundary: the
// exported ones, named as encoding/json names them (a json tag name, or
// the Go name), without those tagged json:"-". An embedded struct or
// pointer to struct, exported or not, promotes its fields.
type JSFields = JSField[] & { direct: Set<string> }; // direct: the names of the fields not flattened

const fieldCache = /* @__PURE__ */ new WeakMap<Type, JSFields>();

function jsFields(t: Type): JSFields {
  let fs = fieldCache.get(t);
  if (fs) return fs;
  fs = Object.assign([] as JSField[], { direct: new Set<string>() });
  for (const f of t.fields) {
    const e = f.type.kind === Kind.Pointer ? f.type.elem! : f.type;
    const promotes = f.embedded && e.kind === Kind.Struct;
    if (f.pkgPath !== "" && !promotes) continue;
    const m = /json:"([^"]*)"/.exec(f.tag);
    const tagName = m ? m[1].split(",")[0] : "";
    if (tagName === "-") continue;
    fs.push({ prop: f.prop, name: tagName || f.name, type: f.type, flatten: promotes && !tagName });
    if (!promotes || tagName) fs.direct.add(tagName || f.name);
  }
  fieldCache.set(t, fs);
  return fs;
}

// plain reports whether values of t are the same in Go and JavaScript.
function plain(t: Type): boolean {
  switch (t.kind) {
    case Kind.Bool: case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
    case Kind.Uint: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
    case Kind.Float32: case Kind.Float64:
      return true;
  }
  return false;
}

// goArgsToJS converts the variadic arguments s of element type elem to the
// arguments of a JavaScript call.
export function goArgsToJS(elem: Type, s: Slice<any> | null): any[] {
  if (s === null) return [];
  const out = new Array(s.$length);
  for (let i = 0; i < s.$length; i++) out[i] = goToJS(elem, s.$array[s.$offset + i]);
  return out;
}

// goFuncToJS wraps a Go function of type t for JavaScript to call. ex is
// set for a function an exported function returns, which converts like an
// export wrapper: its final error result, if any, is thrown when it is not
// nil and left out of the results.
function goFuncToJS(t: Type, f: (...a: any[]) => any, ex = false): (...a: any[]) => any {
  const ps = t.params, rs = t.results, n = ps.length;
  const hasErr = ex && rs.length > 0 && rs[rs.length - 1] === errorType;
  const nres = hasErr ? rs.length - 1 : rs.length;
  const result = (r: any) => {
    if (rs.length === 0) return undefined;
    const v = rs.length === 1 ? [r] : r;
    if (hasErr && v[nres] !== null) throw goError(v[nres]);
    return nres === 0 ? undefined : nres === 1 ? goToJS(rs[0], v[0], ex) : rs.slice(0, nres).map((rt, i) => goToJS(rt, v[i], ex));
  };
  const js = function (...args: any[]): any {
    const a = new Array(n);
    for (let i = 0; i < n; i++) {
      a[i] = t.variadic && i === n - 1
        ? new Slice(args.slice(i).map((x) => jsToGo(ps[i].elem!, x, ex)), 0, Math.max(args.length - i, 0), Math.max(args.length - i, 0))
        : jsToGo(ps[i], args[i], ex);
    }
    let r;
    try {
      r = f(...a);
    } catch (e) {
      throw goThrown(e);
    }
    // A Go function that blocks was lowered to an async function.
    return r instanceof Promise ? r.then(result, (e) => { throw goThrown(e); }) : result(r);
  };
  goFuncs.set(js, f);
  return js;
}

// goFuncs maps the functions goFuncToJS made to the Go functions they call,
// so a Go function that comes back to Go is itself again.
const goFuncs = /* @__PURE__ */ new WeakMap<object, (...a: any[]) => any>();

// jsFuncToGo wraps the JavaScript function f for Go to call as a function of
// type t: the arguments are converted to JavaScript, the results to Go. An
// exception is returned as the final error result if there is one, and
// panics otherwise.
function jsFuncToGo(t: Type, f: any): any {
  if (f === null || f === undefined) return null;
  const g = goFuncs.get(f);
  if (g !== undefined) return g;
  if (typeof f !== "function") plainPanic(`goesm: a JavaScript ${typeof f} cannot be converted to ${t.str}`);
  const ps = t.params, rs = t.results;
  const hasErr = rs.length > 0 && rs[rs.length - 1] === errorType;
  const nres = hasErr ? rs.length - 1 : rs.length;
  return (...a: any[]): any => {
    const args = new Array(a.length);
    for (let i = 0; i < a.length; i++) {
      args[i] = t.variadic && i === ps.length - 1 ? goArgsToJS(ps[i].elem!, a[i]) : goToJS(ps[i], a[i], true);
    }
    if (t.variadic) args.push(...args.pop());
    let r;
    try {
      r = f(...args);
    } catch (e) {
      if (!hasErr) jsPanic(e, null);
      const zeros = rs.slice(0, nres).map((rt) => rt.zero());
      return nres === 0 ? jsError(e, null) : [...zeros, jsError(e, null)];
    }
    if (nres === 0) return hasErr ? null : undefined;
    if (nres === 1 && !hasErr) return jsToGo(rs[0], r, true);
    const out = rs.slice(0, nres).map((rt, i) => jsToGo(rt, nres === 1 ? r : r?.[i], true));
    if (hasErr) out.push(null);
    return out;
  };
}

// jsToGo converts the JavaScript value x to a Go value of type t. A Go value
// of that type is taken as it is (a struct copied). ex is set for the
// arguments of exported functions (see the top of this file). embedding
// holds the struct types whose embedded fields are being filled from x: a
// struct type embedded in itself, directly or not, is left zero there, as
// encoding/json leaves it.
export function jsToGo(t: Type, x: any, ex = false, embedding: Type[] | null = null): any {
  switch (t.kind) {
    case Kind.Bool:
      return !!x;
    case Kind.Int: case Kind.Uint: case Kind.Uintptr:
      return jsInt(x);
    case Kind.Int8:
      return (Number(x) << 24) >> 24;
    case Kind.Int16:
      return (Number(x) << 16) >> 16;
    case Kind.Int32:
      return Number(x) | 0;
    case Kind.Uint8:
      return Number(x) & 255;
    case Kind.Uint16:
      return Number(x) & 65535;
    case Kind.Uint32:
      return Number(x) >>> 0;
    case Kind.Int64:
      return jsInt64(x);
    case Kind.Uint64:
      return jsUint64(x);
    case Kind.Float32:
      return Math.fround(Number(x));
    case Kind.Float64:
      return Number(x);
    case Kind.String:
      return jsString(x);
    case Kind.Slice: {
      if (x === null || x === undefined) return null;
      if (x instanceof Slice) return x;
      const n = x.length >>> 0;
      if (t.elem!.kind === Kind.Uint8) {
        const b = newBytes(n);
        if (x instanceof Uint8Array) b.set(x);
        else for (let i = 0; i < n; i++) b[i] = x[i] & 255;
        return new Slice(b as any, 0, n, n);
      }
      const a = new Array(n);
      for (let i = 0; i < n; i++) a[i] = jsToGo(t.elem!, x[i], ex);
      return new Slice(a, 0, n, n);
    }
    case Kind.Array: {
      const a = new Array(t.len);
      for (let i = 0; i < t.len; i++) a[i] = x === null || x === undefined ? t.elem!.zero() : jsToGo(t.elem!, x[i], ex);
      return a;
    }
    case Kind.Map: {
      if (x === null || x === undefined) return null;
      if (x instanceof GoMap) return x;
      const m = makeMap(t.key!);
      if (x instanceof Map) for (const [k, y] of x) mapSet(m, jsToGo(t.key!, k, ex), jsToGo(t.elem!, y, ex));
      else for (const k of Object.keys(x)) mapSet(m, jsToGo(t.key!, k, ex), jsToGo(t.elem!, x[k], ex));
      return m;
    }
    case Kind.Struct: {
      const v = t.zero();
      switch (jsValueKind(t)) {
        case 1: v.ref = toRef(x); return v;
        case 2: v.Value.ref = toRef(x); return v;
      }
      if (x === null || x === undefined) return v;
      if (t.ctor !== null && x instanceof t.ctor) return x.$clone(t);
      for (const f of jsFields(t)) {
        if (f.flatten) {
          const e = f.type.kind === Kind.Pointer ? f.type.elem! : f.type;
          if (e === t || embedding?.includes(e)) continue;
          v[f.prop] = jsToGo(f.type, x, ex && f.type.kind !== Kind.Pointer, embedding ? [...embedding, t] : [t]);
          continue;
        }
        const y = f.name === "__proto__" && !Object.hasOwn(x, f.name) ? undefined : x[f.name];
        if (y !== undefined) v[f.prop] = jsToGo(f.type, y, ex);
      }
      return v;
    }
    case Kind.Pointer:
      if (x === null || x === undefined) return null;
      if (t.elem!.ctor !== null && x instanceof t.elem!.ctor) return x;
      if (t.elem!.kind !== Kind.Struct && t.elem!.kind !== Kind.Array) {
        // A *int and the like cross to JavaScript as the value they point
        // to (goToJS), and come back as a pointer to a copy of it.
        if (ex) return cell(jsToGo(t.elem!, x, ex));
        break;
      }
      return jsToGo(t.elem!, x, ex, embedding); // a pointer to a struct is the struct object
    case Kind.Interface:
      if (x instanceof Iface) return x;
      if (ex) {
        if (t === errorType) return x === null || x === undefined ? null : x instanceof GoError ? x.value : jsError(x, null);
        if (x === null || x === undefined) return null;
        // A Go struct object: a handle, or a value of a struct class.
        const ct = typeof x === "object" ? classTypes.get(x.constructor) : undefined;
        if (t.imethods.length > 0) {
          if (ct && implementsIface(ct, t)) return box(ct, x.$clone(ct));
          if (ct && implementsIface(ptrTo(ct), t)) return box(ptrTo(ct), x);
          break;
        }
        if (ct) return box(ptrTo(ct), x);
      }
      return jsonToAny(x);
    case Kind.Func:
      if (ex) return jsFuncToGo(t, x);
      break;
    case Kind.Chan: case Kind.UnsafePointer: case Kind.Complex64: case Kind.Complex128:
      if (ex) return x;
      break;
  }
  plainPanic(`goesm: a JavaScript value cannot be converted to ${t.str}`);
}

// jsonToAny converts x as encoding/json decodes into an any.
function jsonToAny(x: any): Iface | null {
  switch (typeof x) {
    case "undefined": return null;
    case "boolean": return box(tBool, x);
    case "number": return box(tFloat64, x);
    case "bigint": return box(tInt64, BigInt.asIntN(64, x));
    case "string": return box(tString, fromJSString(x));
    case "object":
      if (x === null) return null;
      if (Array.isArray(x)) {
        const t = sliceOf(interfaceOf([]));
        return box(t, jsToGo(t, x));
      }
      const t = mapOf(tString, interfaceOf([]));
      return box(t, jsToGo(t, x));
  }
  plainPanic(`goesm: a JavaScript ${typeof x} cannot be converted to a Go value`);
}

// The error a JavaScript exception becomes: js.Error where the calling
// package imports syscall/js (the lowering passes its descriptor), otherwise
// this type, whose Error method returns the same text.
// It is made on first use, so that bundles without //goesm:import do not
// carry it.
let jsErrorType: Type | null = null;

function jsErrorT(): Type {
  if (jsErrorType === null) {
    jsErrorType = named("goesm", "jsError");
    setUnderlying(jsErrorType, tUnsafePointer);
    addMethods(jsErrorType, {
      Error: [(v: any) => "JavaScript error: " + jsString(errorMessage(v)), funcOf([], [tString], false)],
    });
  }
  return jsErrorType;
}

function errorMessage(e: any): string {
  if (e !== null && (typeof e === "object" || typeof e === "function")) {
    const m = e.message;
    return m === undefined ? "<undefined>" : String(m);
  }
  return "<undefined>";
}

// jsError converts the exception e thrown by JavaScript into a Go error of
// type errType (js.Error) or of the runtime's own type. A Go panic, a
// Goexit or an exit raised by Go code called back from JavaScript keeps
// unwinding. A thrown value that is not an object, such as a string or
// null, is wrapped in an Error with it as the message (and the cause), since
// js.Error's Error method reads the message property.
export function jsError(e: unknown, errType: Type | null): Iface {
  if (e instanceof GoPanic || e instanceof Goexit || e instanceof ProgramExit) throw e;
  if (e === null || (typeof e !== "object" && typeof e !== "function")) e = new Error(String(e), { cause: e });
  if (errType === null) return new Iface(jsErrorT(), e);
  const v = errType.zero();
  v.Value.ref = toRef(e);
  return box(errType, v)!;
}

// notAwaited panics for the Promise p, returned to a function imported
// without await. The Promise's rejection, if any, is handled here so that it
// is not also reported as unhandled.
export function notAwaited(p: Promise<unknown>, msg: string): never {
  p.catch(() => {});
  plainPanic(msg);
}

// jsPanic is the panic of a JavaScript exception in a call whose Go
// declaration has no error result, as syscall/js's Value.Call panics.
export function jsPanic(e: unknown, errType: Type | null): never {
  throw new GoPanic(jsError(e, errType));
}
