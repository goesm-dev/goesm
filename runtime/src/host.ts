// The host: standard output and error, and process exit. Node, Bun and Deno
// expose node:fs synchronously through process.getBuiltinModule; browsers
// get the console, line by line, like Go's wasm_exec.js.

const g = globalThis as any;

// hostBuiltin returns the host's built-in module name (node:fs, ...), or
// null where it has none. Next.js's edge runtime throws as soon as
// process.getBuiltinModule is read.
export function hostBuiltin(name: string): any {
  try {
    return g.process?.getBuiltinModule?.(name) ?? null;
  } catch {
    return null;
  }
}

let nodeFS: any = undefined;

// hostNodeFS returns node:fs, or null where the host has none.
export function hostNodeFS(): any {
  if (nodeFS === undefined) nodeFS = hostBuiltin("fs");
  return nodeFS;
}

// writeSyncAll is fs.writeSync, retrying while a non-blocking pipe is full.
export function writeSyncAll(fs: any, fd: number, buf: Uint8Array, off: number, len: number, pos: number | null): number {
  for (;;) {
    try {
      return fs.writeSync(fd, buf, off, len, pos);
    } catch (e: any) {
      if (e?.code !== "EAGAIN") throw e;
    }
  }
}

const decoders = [new TextDecoder("utf-8"), new TextDecoder("utf-8")];
const pending = ["", ""];

// writeConsole writes complete lines of fd 1 or 2 to the console.
export function writeConsole(fd: number, buf: Uint8Array): void {
  const i = fd === 2 ? 1 : 0;
  pending[i] += decoders[i].decode(buf, { stream: true });
  const nl = pending[i].lastIndexOf("\n");
  if (nl === -1) return;
  const out = pending[i].slice(0, nl);
  pending[i] = pending[i].slice(nl + 1);
  if (i === 0) console.log(out);
  else console.error(out);
}

// writeStd writes the bytes of a Go string to standard output (fd 1) or
// standard error (fd 2).
export function writeStd(fd: number, s: string): void {
  const buf = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) buf[i] = s.charCodeAt(i);
  const fs = hostNodeFS();
  if (fs) {
    for (let off = 0; off < buf.length; ) off += writeSyncAll(fs, fd, buf, off, buf.length - off, null);
  } else {
    writeConsole(fd, buf);
  }
}

// ProgramExit unwinds the program after os.Exit where the host cannot stop
// it (in browsers).
export class ProgramExit {
  declare code: number;
  constructor(code: number) {
    this.code = code;
  }
}

// exiting is set once the program has asked the host to exit.
export let exiting = false;

export function exitProcess(code: number): never {
  exiting = true;
  if (typeof g.process?.exit === "function") g.process.exit(code);
  throw new ProgramExit(code);
}
