// Natives: the implementations of standard library functions that have no Go
// body (assembly, linkname, or a goesm replacement in internal/natives that
// leaves the body to the runtime), and of the few whose Go body goesm does
// not lower (internal/natives overrides).
//
// Generated code imports this module as $natives. Each native is an export
// named after the function's types.Func.FullName,
// "native$" + the name with every character other than letters, digits and
// _ replaced by "$": internal/bytealg.IndexByte is
// native$internal$bytealg$IndexByte. goesm reads the export names from this
// file, so a missing native is reported at compile time, and esbuild drops
// the natives a program does not call.
//
// The set is fixed and owned by goesm.

// natives.ts is a separate module (@goesm/runtime/natives) that uses the
// runtime only through its public module, so split builds share one runtime.
import {
  GoMap, GoPanic, Goexit, Iface, Kind, ProgramExit, Slice, Type, assign, chanLen, copy, exitProcess,
  fromJSString, hostNodeFS, implementsIface, isAggregate, load, toJSString, writeConsole, writeSyncAll,
  makeSlice, mapLen, numGoroutine, runtimePanic, sizeOf, store,
} from "./index.ts";
import type { S } from "./index.ts";

// ---- runtime ----

export function native$runtime$Goexit(): never {
  throw new Goexit();
}

export const native$runtime$NumGoroutine = numGoroutine;

// ---- math and internal/strconv: float bits ----
//
// 64-bit integers are JS numbers in the PoC, so the uint64 bits of a float64
// are exact only below 2^53 (see ARCHITECTURE.md, integers).

const scratch = new DataView(new ArrayBuffer(8));

export function native$math$Float64bits(f: number): number {
  scratch.setFloat64(0, f);
  return Number(scratch.getBigUint64(0));
}

export function native$math$Float64frombits(b: number): number {
  scratch.setBigUint64(0, BigInt.asUintN(64, BigInt(Math.trunc(b))));
  return scratch.getFloat64(0);
}

export function native$math$Float32bits(f: number): number {
  scratch.setFloat32(0, f);
  return scratch.getUint32(0);
}

export function native$math$Float32frombits(b: number): number {
  scratch.setUint32(0, b);
  return scratch.getFloat32(0);
}

export const native$internal$strconv$float64bits = native$math$Float64bits;
export const native$internal$strconv$float64frombits = native$math$Float64frombits;
export const native$internal$strconv$float32bits = native$math$Float32bits;
export const native$internal$strconv$float32frombits = native$math$Float32frombits;

// ---- math/bits (64-bit) ----
//
// The Go code's de Bruijn multiplications and 64-bit masks need exact uint64
// arithmetic; BigInt gives it for inputs below 2^53.

const M64 = (1n << 64n) - 1n;
const big = (x: number) => BigInt.asUintN(64, BigInt(Math.trunc(x)));
const num = (x: bigint) => Number(BigInt.asUintN(64, x));

export function native$math$bits$Len64(x: number): number {
  const b = big(x);
  return b === 0n ? 0 : b.toString(2).length;
}

export function native$math$bits$LeadingZeros64(x: number): number {
  return 64 - native$math$bits$Len64(x);
}

export function native$math$bits$TrailingZeros64(x: number): number {
  const b = big(x);
  return b === 0n ? 64 : (b & -b).toString(2).length - 1;
}

export function native$math$bits$OnesCount64(x: number): number {
  let n = 0;
  for (const c of big(x).toString(2)) if (c === "1") n++;
  return n;
}

export function native$math$bits$RotateLeft64(x: number, k: number): number {
  const s = BigInt(((k % 64) + 64) % 64), b = big(x);
  return num((b << s) | (b >> (64n - s)));
}

export function native$math$bits$Reverse64(x: number): number {
  return num(BigInt("0b" + big(x).toString(2).padStart(64, "0").split("").reverse().join("")));
}

export function native$math$bits$ReverseBytes64(x: number): number {
  let b = big(x), r = 0n;
  for (let i = 0; i < 8; i++) {
    r = (r << 8n) | (b & 0xffn);
    b >>= 8n;
  }
  return num(r);
}

export function native$math$bits$Add64(x: number, y: number, carry: number): [number, number] {
  const s = big(x) + big(y) + big(carry);
  return [num(s), Number(s >> 64n)];
}

export function native$math$bits$Sub64(x: number, y: number, borrow: number): [number, number] {
  const d = big(x) - big(y) - big(borrow);
  return [num(d), d < 0n ? 1 : 0];
}

export function native$math$bits$Mul64(x: number, y: number): [number, number] {
  const p = big(x) * big(y);
  return [num(p >> 64n), num(p & M64)];
}

export function native$math$bits$Div64(hi: number, lo: number, y: number): [number, number] {
  const d = big(y), h = big(hi);
  if (d === 0n) runtimePanic("integer divide by zero");
  if (d <= h) runtimePanic("integer overflow");
  const n = (h << 64n) | big(lo);
  return [num(n / d), num(n % d)];
}

export function native$math$bits$Rem64(hi: number, lo: number, y: number): number {
  const d = big(y);
  if (d === 0n) runtimePanic("integer divide by zero");
  return num(((big(hi) << 64n) | big(lo)) % d);
}

// ---- maps, slices ----

// maps.clone(m any) any: a shallow copy (keys and values are assigned, so
// aggregates are copied).
export function native$maps$clone(m: Iface | null): Iface | null {
  if (m === null || m.v === null) return m;
  const src = m.v as GoMap<any, any>, t = m.t;
  const dst = new GoMap<any, any>(src.keyType);
  for (const [h, [k, v]] of src.entries) dst.entries.set(h, [copy(t.key!, k), copy(t.elem!, v)]);
  return new Iface(t, dst);
}

// slices.overlaps compares element addresses in Go: here, backing arrays and
// index ranges.
export function native$slices$overlaps(_e: Type, a: S<any>, b: S<any>): boolean {
  return a !== null && b !== null && a.$length > 0 && b.$length > 0 && a.$array === b.$array &&
    a.$offset < b.$offset + b.$length && b.$offset < a.$offset + a.$length;
}

// ---- internal/abi ----

export function native$internal$abi$NoEscape(p: any): any {
  return p;
}

export function native$internal$abi$Escape(_t: Type, x: any): any {
  return x;
}

// ---- internal/bytealg ----

export function native$internal$bytealg$IndexByte(b: S<number>, c: number): number {
  if (b === null) return -1;
  const a = b.$array, o = b.$offset;
  for (let i = 0; i < b.$length; i++) if (a[o + i] === c) return i;
  return -1;
}

export function native$internal$bytealg$IndexByteString(s: string, c: number): number {
  return s.indexOf(String.fromCharCode(c));
}

export function native$internal$bytealg$Compare(x: S<number>, y: S<number>): number {
  const lx = x === null ? 0 : x.$length, ly = y === null ? 0 : y.$length;
  for (let i = 0; i < Math.min(lx, ly); i++) {
    const a = x!.$array[x!.$offset + i], b = y!.$array[y!.$offset + i];
    if (a !== b) return a < b ? -1 : 1;
  }
  return lx === ly ? 0 : lx < ly ? -1 : 1;
}

// Strings hold one byte per UTF-16 code unit, so JS order is byte order.
export function native$internal$bytealg$abigen_runtime_cmpstring(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

export function native$internal$bytealg$MakeNoZero(n: number): Slice<number> {
  return makeSlice(n, n, () => 0);
}

// ---- sync/atomic ----
//
// Goroutines share one thread and never preempt each other, so the atomic
// operations are plain loads and stores through the pointer (a Cell or a
// field pointer: p.v).

const ld = (p: any) => p.v;
const st = (p: any, v: any) => { p.v = v; };
const swap = (p: any, v: any) => { const old = p.v; p.v = v; return old; };
const cas = (p: any, old: any, v: any) => {
  if (p.v !== old) return false;
  p.v = v;
  return true;
};
const add = (w: (x: number) => number) => (p: any, d: number) => (p.v = w(p.v + d));
const and = (w: (x: number) => number) => (p: any, m: number) => { const old = p.v; p.v = w(Number(BigInt(old) & BigInt(m))); return old; };
const or = (w: (x: number) => number) => (p: any, m: number) => { const old = p.v; p.v = w(Number(BigInt(old) | BigInt(m))); return old; };
const i32 = (x: number) => x | 0, u32 = (x: number) => x >>> 0, n64 = (x: number) => x;

export const native$sync$atomic$LoadInt32 = ld, native$sync$atomic$LoadInt64 = ld, native$sync$atomic$LoadUint32 = ld,
  native$sync$atomic$LoadUint64 = ld, native$sync$atomic$LoadUintptr = ld, native$sync$atomic$LoadPointer = ld;
export const native$sync$atomic$StoreInt32 = st, native$sync$atomic$StoreInt64 = st, native$sync$atomic$StoreUint32 = st,
  native$sync$atomic$StoreUint64 = st, native$sync$atomic$StoreUintptr = st, native$sync$atomic$StorePointer = st;
export const native$sync$atomic$SwapInt32 = swap, native$sync$atomic$SwapInt64 = swap, native$sync$atomic$SwapUint32 = swap,
  native$sync$atomic$SwapUint64 = swap, native$sync$atomic$SwapUintptr = swap, native$sync$atomic$SwapPointer = swap;
export const native$sync$atomic$CompareAndSwapInt32 = cas, native$sync$atomic$CompareAndSwapInt64 = cas,
  native$sync$atomic$CompareAndSwapUint32 = cas, native$sync$atomic$CompareAndSwapUint64 = cas,
  native$sync$atomic$CompareAndSwapUintptr = cas, native$sync$atomic$CompareAndSwapPointer = cas;
export const native$sync$atomic$AddInt32 = add(i32), native$sync$atomic$AddUint32 = add(u32),
  native$sync$atomic$AddInt64 = add(n64), native$sync$atomic$AddUint64 = add(n64), native$sync$atomic$AddUintptr = add(n64);
export const native$sync$atomic$AndInt32 = and(i32), native$sync$atomic$AndUint32 = and(u32),
  native$sync$atomic$AndInt64 = and(n64), native$sync$atomic$AndUint64 = and(n64), native$sync$atomic$AndUintptr = and(n64);
export const native$sync$atomic$OrInt32 = or(i32), native$sync$atomic$OrUint32 = or(u32),
  native$sync$atomic$OrInt64 = or(n64), native$sync$atomic$OrUint64 = or(n64), native$sync$atomic$OrUintptr = or(n64);

// ---- internal/reflectlite ----
//
// A reflectlite *rtype is a runtime type descriptor; a Value's ptr is the JS
// value itself, or (flagAddr) a pointer to it.

export function native$internal$reflectlite$ifaceType(i: Iface | null): Type | null {
  return i === null ? null : i.t;
}

export function native$internal$reflectlite$ifaceValue(i: Iface | null): any {
  return i === null ? null : i.v;
}

export function native$internal$reflectlite$asIface(x: any): Iface | null {
  return x;
}

export function native$internal$reflectlite$typeName(t: Type): string {
  if (t.named) return t.name + (t.typeArgs.length ? `[${t.typeArgs.map((a) => a.str).join(",")}]` : "");
  if (t.kind === Kind.UnsafePointer) return "Pointer";
  // Predeclared types are named; composite literal types are not.
  return t.kind <= Kind.Complex128 || t.kind === Kind.String ? t.str : "";
}

export function native$internal$reflectlite$typePkgPath(t: Type): string {
  return t.named ? t.pkgPath : "";
}

export const native$internal$reflectlite$typeSize = sizeOf;

export function native$internal$reflectlite$typeKind(t: Type): number {
  return t.kind;
}

export function native$internal$reflectlite$typeString(t: Type): string {
  return t.str;
}

export function native$internal$reflectlite$typeComparable(t: Type): boolean {
  switch (t.kind) {
    case Kind.Slice: case Kind.Map: case Kind.Func:
      return false;
    case Kind.Array:
      return native$internal$reflectlite$typeComparable(t.elem!);
    case Kind.Struct:
      return t.fields.every((f) => native$internal$reflectlite$typeComparable(f.type));
  }
  return true;
}

export function native$internal$reflectlite$typeElem(t: Type): Type | null {
  return t.elem;
}

export function native$internal$reflectlite$implements(iface: Type, t: Type): boolean {
  if (iface.kind !== Kind.Interface) return false;
  if (t.kind === Kind.Interface) {
    return iface.imethods.every((m) => t.imethods.some((n) => n.name === m.name && n.pkgPath === m.pkgPath && n.type === m.type));
  }
  return implementsIface(t, iface);
}

export function native$internal$reflectlite$directlyAssignable(dst: Type, src: Type): boolean {
  if (dst === src) return true;
  if ((dst.named && src.named) || dst.kind !== src.kind) return false;
  if (dst.kind === Kind.Chan && src.dir === 3 && dst.elem === src.elem) return true;
  return dst.underlying === src.underlying;
}

export function native$internal$reflectlite$load(t: Type, p: any): any {
  return load(t, p);
}

export function native$internal$reflectlite$store(t: Type, p: any, x: any): void {
  store(t, p, x);
}

// convert returns x (of type src) as a value of type dst for assignment:
// boxed into an interface, and a copy for aggregates.
export function native$internal$reflectlite$convert(dst: Type, src: Type, x: any): any {
  return dst.kind === Kind.Interface && src.kind !== Kind.Interface ? new Iface(src, copy(src, x)) : copy(src, x);
}

export function native$internal$reflectlite$length(t: Type, x: any): number {
  switch (t.kind) {
    case Kind.Array: return t.len;
    case Kind.Slice: return x === null ? 0 : x.$length;
    case Kind.String: return x.length;
    case Kind.Map: return mapLen(x);
    case Kind.Chan: return chanLen(x);
  }
  return 0;
}

// swapper swaps slice elements by value: aggregates are copied in place, so
// pointers to elements keep pointing at their slots.
export function native$internal$reflectlite$swapper(t: Type, s: Slice<any> | null): (i: number, j: number) => void {
  const e = t.elem!;
  return (i: number, j: number) => {
    const n = s === null ? 0 : s.$length;
    if (i < 0 || i >= n) runtimePanic(`index out of range [${i}] with length ${n}`);
    if (j < 0 || j >= n) runtimePanic(`index out of range [${j}] with length ${n}`);
    const a = s!.$array, o = s!.$offset;
    if (isAggregate(e)) {
      const tmp = copy(e, a[o + i]);
      assign(e, a[o + i], a[o + j]);
      assign(e, a[o + j], tmp);
    } else {
      const tmp = a[o + i];
      a[o + i] = a[o + j];
      a[o + j] = tmp;
    }
  };
}

// ---- syscall/js ----
//
// goesm's syscall/js (internal/natives/goroot/syscall/js) holds JavaScript
// values as they are. Go's nil (the zero Value) is undefined, so JavaScript
// null needs a sentinel.

const jsNull = { toString: () => "null" };

function toRef(x: unknown): any {
  return x === undefined ? null : x === null ? jsNull : x;
}

function fromRef(r: any): any {
  return r === null ? undefined : r === jsNull ? null : r;
}

function refArgs(args: S<any>): any[] {
  const out: any[] = [];
  if (args !== null) for (let i = 0; i < args.$length; i++) out.push(fromRef(args.$array[args.$offset + i]));
  return out;
}

// A JavaScript exception becomes a js.Error; a Go panic or Goexit raised by
// Go code called back from JavaScript keeps unwinding.
function jsThrown(e: unknown): [any, boolean] {
  if (e instanceof GoPanic || e instanceof Goexit || e instanceof ProgramExit) throw e;
  return [toRef(e), false];
}

export function native$syscall$js$nullRef(): any {
  return jsNull;
}

export function native$syscall$js$globalRef(): any {
  return globalThis;
}

export function native$syscall$js$boolVal(b: boolean): any {
  return b;
}

export function native$syscall$js$floatVal(f: number): any {
  return f;
}

export function native$syscall$js$stringVal(s: string): any {
  return toJSString(s);
}

export function native$syscall$js$valueEqual(v: any, w: any): boolean {
  return fromRef(v) === fromRef(w);
}

export function native$syscall$js$valueIsNaN(v: any): boolean {
  return typeof v === "number" && v !== v;
}

export function native$syscall$js$valueType(v: any): number {
  const x = fromRef(v);
  if (x === undefined) return 0;
  if (x === null) return 1;
  switch (typeof x) {
    case "boolean": return 2;
    case "number": return 3;
    case "string": return 4;
    case "symbol": return 5;
    case "function": return 7;
  }
  return 6; // objects and bigints, as in wasm_exec.js
}

export function native$syscall$js$valueGet(v: any, p: string): any {
  const o = fromRef(v);
  const name = toJSString(p);
  if (o === globalThis && (name === "fs" || name === "process") && (globalThis as any)[name] === undefined) {
    return name === "fs" ? hostFS() : hostProcess();
  }
  return toRef(Reflect.get(o, name));
}

export function native$syscall$js$valueSet(v: any, p: string, x: any): void {
  Reflect.set(fromRef(v), toJSString(p), fromRef(x));
}

export function native$syscall$js$valueDelete(v: any, p: string): void {
  Reflect.deleteProperty(fromRef(v), toJSString(p));
}

export function native$syscall$js$valueIndex(v: any, i: number): any {
  return toRef(Reflect.get(fromRef(v), i));
}

export function native$syscall$js$valueSetIndex(v: any, i: number, x: any): void {
  Reflect.set(fromRef(v), i, fromRef(x));
}

export function native$syscall$js$valueLength(v: any): number {
  return Number(fromRef(v).length);
}

export function native$syscall$js$valueCall(v: any, m: string, args: S<any>): [any, boolean] {
  try {
    const o = fromRef(v);
    const f = native$syscall$js$valueGet(v, m);
    return [toRef(Reflect.apply(fromRef(f), o, refArgs(args))), true];
  } catch (e) {
    return jsThrown(e);
  }
}

export function native$syscall$js$valueInvoke(v: any, args: S<any>): [any, boolean] {
  try {
    return [toRef(Reflect.apply(fromRef(v), undefined, refArgs(args))), true];
  } catch (e) {
    return jsThrown(e);
  }
}

export function native$syscall$js$valueNew(v: any, args: S<any>): [any, boolean] {
  try {
    return [toRef(Reflect.construct(fromRef(v), refArgs(args))), true];
  } catch (e) {
    return jsThrown(e);
  }
}

export function native$syscall$js$valueFloat(v: any): number {
  return v;
}

export function native$syscall$js$valueTruthy(v: any): boolean {
  return !!fromRef(v);
}

export function native$syscall$js$valueString(v: any): string {
  return fromJSString(String(fromRef(v)));
}

export function native$syscall$js$valueInstanceOf(v: any, t: any): boolean {
  return fromRef(v) instanceof fromRef(t);
}

function isByteArray(x: unknown): x is Uint8Array | Uint8ClampedArray {
  return x instanceof Uint8Array || x instanceof Uint8ClampedArray;
}

export function native$syscall$js$copyBytesToGo(dst: S<number>, src: any): [number, boolean] {
  const a = fromRef(src);
  if (!isByteArray(a)) return [0, false];
  const n = Math.min(dst === null ? 0 : dst.$length, a.length);
  for (let i = 0; i < n; i++) dst!.$array[dst!.$offset + i] = a[i];
  return [n, true];
}

export function native$syscall$js$copyBytesToJS(dst: any, src: S<number>): [number, boolean] {
  const a = fromRef(dst);
  if (!isByteArray(a)) return [0, false];
  const n = Math.min(a.length, src === null ? 0 : src.$length);
  for (let i = 0; i < n; i++) a[i] = src!.$array[src!.$offset + i];
  return [n, true];
}

export function native$syscall$js$makeFunc(fn: (self: any, args: S<any>) => any): any {
  return function (this: any, ...args: any[]): any {
    const a = args.map(toRef);
    const r = fn(toRef(this), new Slice(a, 0, a.length, a.length));
    // A Go function that blocks was lowered to an async function.
    return r instanceof Promise ? r.then(fromRef) : fromRef(r);
  };
}

// ---- the host's fs and process, seen through js.Global() ----
//
// Package syscall reaches files through js.Global().Get("fs") with Node's
// callback API, like Go's wasm_exec.js expects. Where the host has no global
// fs (Node and Bun do not expose one; browsers have no file system), goesm
// supplies one without touching globalThis. It calls back before returning,
// which lets goesm lower syscall.fsCall as synchronous (internal/natives:
// syncFuncs), so writing to os.Stdout does not make callers async.

function enosys(): Error {
  const err = new Error("not implemented");
  (err as any).code = "ENOSYS";
  return err;
}

const fsCalls = [
  "open", "close", "read", "write", "fstat", "stat", "lstat", "readdir", "mkdir", "unlink", "rmdir",
  "chmod", "fchmod", "chown", "fchown", "lchown", "utimes", "rename", "truncate", "ftruncate",
  "readlink", "link", "symlink", "fsync",
];

let theFS: any = null;

function hostFS(): any {
  if (theFS === null) {
    const fs = hostNodeFS();
    theFS = fs ? nodeFS(fs) : consoleFS();
  }
  return theFS;
}

// nodeFS adapts the synchronous API of node:fs (Node, Bun, Deno) to the
// callback API.
function nodeFS(fs: any): any {
  const shim: any = { constants: fs.constants };
  for (const name of fsCalls) {
    shim[name] = (...args: any[]) => {
      const cb = args.pop();
      let r: any;
      try {
        r = name === "write" ? writeSyncAll(fs, args[0], args[1], args[2], args[3], args[4]) : fs[name + "Sync"](...args);
      } catch (e) {
        cb(e);
        return;
      }
      cb(null, r);
    };
  }
  return shim;
}

// consoleFS is a browser's file system: as in wasm_exec.js, standard output
// and standard error go to the console line by line, and everything else
// fails with ENOSYS.
function consoleFS(): any {
  const shim: any = {
    constants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1, O_DIRECTORY: -1 },
    write(fd: number, buf: Uint8Array, off: number, len: number, pos: number | null, cb: (err: any, n?: number) => void) {
      if ((fd !== 1 && fd !== 2) || pos !== null) {
        cb(enosys());
        return;
      }
      writeConsole(fd, buf.subarray(off, off + len));
      cb(null, len);
    },
  };
  for (const name of fsCalls) {
    if (!(name in shim)) shim[name] = (...args: any[]) => args[args.length - 1](enosys());
  }
  return shim;
}

let theProcess: any = null;

function hostProcess(): any {
  theProcess ??= {
    getuid: () => -1, getgid: () => -1, geteuid: () => -1, getegid: () => -1,
    getgroups: () => { throw enosys(); },
    pid: -1, ppid: -1,
    umask: () => { throw enosys(); },
    cwd: () => { throw enosys(); },
    chdir: () => { throw enosys(); },
  };
  return theProcess;
}

// ---- syscall and os: the process ----

const gproc = (globalThis as any).process;

function goStrings(xs: string[]): Slice<string> {
  const a = xs.map(fromJSString);
  return new Slice(a, 0, a.length, a.length);
}

// The environment is the host's at start-up; package syscall keeps its own
// copy, so Setenv does not change the host's.
export function native$syscall$runtime_envs(): Slice<string> {
  const env = gproc?.env;
  return goStrings(env ? Object.keys(env).map((k) => k + "=" + env[k]) : []);
}

export function native$syscall$runtimeSetenv(_k: string, _v: string): void {}
export function native$syscall$runtimeUnsetenv(_k: string): void {}
export function native$syscall$runtimeClearenv(): void {}

export function native$syscall$Getpagesize(): number {
  return 65536;
}

export function native$syscall$Exit(code: number): never {
  return exitProcess(code);
}

export function native$syscall$now(): [number, number] {
  const ms = Date.now();
  return [Math.floor(ms / 1000), (ms % 1000) * 1e6];
}

// os.Args: the program (the script) and its arguments, as with go run.
export function native$os$runtime_args(): Slice<string> {
  const argv: string[] | undefined = gproc?.argv;
  return goStrings(Array.isArray(argv) && argv.length > 1 ? argv.slice(1) : ["js"]);
}

export function native$os$runtime_beforeExit(_code: number): void {}
export function native$os$sigpipe(): void {}

export function native$os$runtime_rand(): number {
  // 53 random bits: 64-bit integers are JS numbers (exact below 2^53).
  return Math.floor(Math.random() * 2 ** 53);
}

// ---- internal/poll ----
//
// goesm's file I/O is synchronous, so the fd mutex is never contended.

export function native$internal$poll$runtime_Semacquire(sema: any): void {
  if (sema.v === 0) runtimePanic("goesm: internal/poll semaphore would block");
  sema.v--;
}

export function native$internal$poll$runtime_Semrelease(sema: any): void {
  sema.v++;
}

// ---- time: clocks ----
//
// The wall clock is Date.now (millisecond resolution); the monotonic clock
// is performance.now, in nanoseconds since the program started, so it stays
// exact as a JS number.

const perf = (globalThis as any).performance;
const monoStart = perf ? perf.now() : Date.now();

function monoNanos(): number {
  return Math.round(((perf ? perf.now() : Date.now()) - monoStart) * 1e6) + 1;
}

function wallNow(): [number, number, number] {
  const ms = Date.now();
  return [Math.floor(ms / 1000), (ms % 1000) * 1e6, monoNanos()];
}

export const native$time$now = wallNow;
export const native$time$runtimeNow = wallNow;
export const native$time$runtimeNano = monoNanos;

export function native$time$runtimeIsBubbled(): boolean {
  return false;
}
