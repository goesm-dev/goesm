// panic / recover / defer.
//
// A Go panic is a JS exception of class GoPanic carrying the panic value as a
// Go interface value. Functions that contain `defer` are lowered to
//
//   const $d = new $rt.Defers();
//   $body: try { ... } catch (e) { $d.fail(e); } finally { $d.run(); }
//   return <results>;
//
// so deferred calls run in LIFO order on both normal return and panic, a
// deferred call can recover() and the function then returns its (possibly
// modified) named results, exactly like Go.

import { Iface } from "./iface.ts";
import { Kind, Type, addMethods, funcOf, named, setUnderlying, types } from "./types.ts";
import { toJSString } from "./string.ts";
import { ProgramExit } from "./host.ts";

// runtime.Error values. Their dynamic type implements error and runtime.Error
// so user code can recover() them and call Error().
export const runtimeErrorType: Type = named("runtime", "Error");
setUnderlying(runtimeErrorType, types.string);
addMethods(runtimeErrorType, {
  Error: [(v: string) => "runtime error: " + v, funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

// runtime.plainError: runtime errors whose message has no prefix in Go
// (nil map assignment, channel misuse).
export const plainErrorType: Type = named("runtime", "plainError");
setUnderlying(plainErrorType, types.string);
addMethods(plainErrorType, {
  Error: [(v: string) => v, funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

export function plainPanic(msg: string): never {
  throw new GoPanic(new Iface(plainErrorType, msg));
}

// *runtime.TypeAssertionError: its message has no "runtime error: " prefix.
export const typeAssertionErrorType: Type = named("runtime", "TypeAssertionError");
setUnderlying(typeAssertionErrorType, types.string);
addMethods(typeAssertionErrorType, {
  Error: [(v: string) => v, funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

export const panicNilErrorType: Type = named("runtime", "PanicNilError");
setUnderlying(panicNilErrorType, types.string);
addMethods(panicNilErrorType, {
  Error: [() => "panic called with nil argument (use runtime.PanicNilError)", funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

export class GoPanic extends Error {
  value: Iface;
  constructor(value: Iface) {
    super("panic: " + formatPanicValue(value));
    this.value = value;
    this.name = "GoPanic";
  }
}

export function formatPanicValue(v: Iface | null): string {
  if (v === null) return "nil";
  const err = v.t.methods.get("Error");
  if (err) {
    return toJSString(err.fn(v.v));
  }
  const str = v.t.methods.get("String");
  if (str) return toJSString(str.fn(v.v));
  switch (v.t.kind) {
    case Kind.String:
      return v.t.named ? `${v.t.str}(${JSON.stringify(toJSString(v.v))})` : toJSString(v.v);
    case Kind.Bool: case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
    case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
    case Kind.Float32: case Kind.Float64:
      return v.t.named ? `${v.t.str}(${v.v})` : String(v.v);
  }
  return `(${v.t.str}) ${String(v.v)}`;
}

export function panic(v: Iface | null): never {
  if (v === null) v = new Iface(panicNilErrorType, "");
  throw new GoPanic(v);
}

export function runtimePanic(msg: string): never {
  throw new GoPanic(new Iface(runtimeErrorType, msg));
}

// indexError and sliceError panic with gc's bounds error messages
// (runtime/error.go): a negative index is reported without the length.
export function indexError(i: number, n: number): never {
  runtimePanic(i < 0 ? `index out of range [${i}]` : `index out of range [${i}] with length ${n}`);
}

// sliceError reports the first failing check of x[lo:hi] (max undefined) or
// x[lo:hi:max]; c is the capacity (what = "capacity") or, for strings and
// arrays sliced with two indices, the length.
export function sliceError(lo: number, hi: number, max: number | undefined, c: number, what = "capacity"): never {
  const p = "slice bounds out of range ";
  if (max === undefined) {
    if (hi < 0 || hi > c) runtimePanic(hi < 0 ? `${p}[:${hi}]` : `${p}[:${hi}] with ${what} ${c}`);
    runtimePanic(lo < 0 ? `${p}[${lo}:]` : `${p}[${lo}:${hi}]`);
  }
  if (max < 0 || max > c) runtimePanic(max < 0 ? `${p}[::${max}]` : `${p}[::${max}] with ${what} ${c}`);
  if (hi < 0 || hi > max) runtimePanic(hi < 0 ? `${p}[:${hi}:]` : `${p}[:${hi}:${max}]`);
  runtimePanic(lo < 0 ? `${p}[${lo}::]` : `${p}[${lo}:${hi}:]`);
}

// toPanic converts anything thrown into a GoPanic. JS TypeErrors arise from
// touching null (nil pointers, nil maps on read paths that skipped a check);
// they are reported as Go's nil dereference runtime error.
export function toPanic(e: unknown): GoPanic {
  if (e instanceof GoPanic) return e;
  let p: GoPanic;
  if (e instanceof TypeError) {
    p = new GoPanic(new Iface(runtimeErrorType, "invalid memory address or nil pointer dereference"));
  } else if (e instanceof RangeError && /call stack/i.test(e.message)) {
    p = new GoPanic(new Iface(runtimeErrorType, "stack overflow (JS call stack exhausted)"));
  } else {
    p = new GoPanic(new Iface(runtimeErrorType, "js: " + String(e)));
  }
  if (e instanceof Error && e.stack) p.stack = p.message + "\n" + e.stack.split("\n").slice(1).join("\n");
  (p as any).cause = e;
  return p;
}

// The defer frame whose deferred call is currently executing (synchronously).
// recover() consults it. See ARCHITECTURE.md for the known gap: Go only lets
// recover() work when called directly by the deferred function; the PoC
// accepts any call made synchronously during that deferred call.
let current: Defers | null = null;

// Goexit is thrown by runtime.Goexit: deferred calls run, recover() does
// not stop it, and the goroutine ends without crashing the program.
export class Goexit {}

export class Defers {
  private list: Array<() => any> = [];
  panicking: GoPanic | null = null;
  exiting: Goexit | null = null;
  // halt is os.Exit's unwinding where the host cannot stop the program: no
  // deferred call runs and nothing recovers it.
  halt: ProgramExit | null = null;

  defer(fn: () => any): void {
    this.list.push(fn);
  }

  fail(e: unknown): void {
    if (e instanceof ProgramExit) {
      this.halt = e;
      this.list.length = 0;
    } else if (this.halt) return;
    else if (e instanceof Goexit) this.exiting = e;
    else this.panicking = toPanic(e);
  }

  run(): void {
    while (this.list.length > 0) {
      const fn = this.list.pop()!;
      const prev = current;
      current = this;
      try {
        fn();
      } catch (e) {
        this.fail(e);
      } finally {
        current = prev;
      }
    }
    if (this.halt) throw this.halt;
    if (this.panicking) throw this.panicking;
    if (this.exiting) throw this.exiting;
  }

  async runAsync(): Promise<void> {
    while (this.list.length > 0) {
      const fn = this.list.pop()!;
      const prev = current;
      current = this;
      let r: any;
      try {
        r = fn();
      } catch (e) {
        this.fail(e);
      } finally {
        current = prev;
      }
      if (r instanceof Promise) {
        try {
          await r;
        } catch (e) {
          this.fail(e);
        }
      }
    }
    if (this.halt) throw this.halt;
    if (this.panicking) throw this.panicking;
    if (this.exiting) throw this.exiting;
  }
}

export function recover(): Iface | null {
  const f = current;
  if (f === null || f.panicking === null) return null;
  const v = f.panicking.value;
  f.panicking = null;
  return v;
}

// rangeError panics for an iterator that misuses yield (Go's runtime
// checks on range-over-func). state is the loop state when yield was
// called (1 body returned false, 2 loop exited, 3 body panicked) or 4 when
// the iterator returned after recovering a panic of the body.
export function rangeError(state: number): never {
  runtimePanic([
    "",
    "range function continued iteration after function for loop body returned false",
    "range function continued iteration after whole loop exit",
    "range function continued iteration after loop body panic",
    "range function recovered a loop body panic and did not resume panicking",
  ][state]);
}
