// HTTP servers on the host: Go's net/http Handlers serving the Fetch API's
// Requests. goesm's patch of net/http (internal/natives/patch/net/http)
// registers the Go function that serves one request; fetchHandler makes it a
// fetch handler (Cloudflare Workers, Deno.serve, Bun.serve), and
// http.ListenAndServe starts the host's own server with it (hostListen).

import type { Iface } from "./iface.ts";
import { hostBuiltin } from "./host.ts";
import { plainPanic } from "./panic.ts";

// serve(handler, request, info) is net/http's hostServeJS: it resolves the
// Response, or an Error when the handler panicked before writing its header.
let serve: ((h: Iface | null, req: any, info?: any) => any) | null = null;

export function setHTTPServer(fn: (h: Iface | null, req: any, info?: any) => any): void {
  serve = fn;
}

async function serveOne(h: Iface | null, req: any, info?: any): Promise<Response> {
  const res = await serve!(h, req, info);
  if (res instanceof Error) throw res;
  return res;
}

/**
 * fetchHandler returns a fetch handler that serves each Request with the Go
 * http.Handler h (a nil Handler is http.DefaultServeMux), for Cloudflare
 * Workers (`export default { fetch: rt.fetchHandler(Handler()) }`),
 * Deno.serve, Bun.serve and service workers. h may also be the Promise of a
 * Handler that a Go function which may block returns. Each request runs in
 * its own goroutine; its body is read in full first. The Response resolves
 * when the handler returns, or when it flushes (http.Flusher,
 * http.ResponseController), after which the body streams. The request's
 * context is canceled when its signal aborts or the handler returns.
 */
export function fetchHandler(h: Iface | null | PromiseLike<Iface | null>): (req: Request, ...rest: any[]) => Promise<Response> {
  if (serve === null) plainPanic("goesm: fetchHandler needs a program that uses net/http");
  if (h !== null && typeof (h as any).then === "function") {
    let handler: Iface | null | undefined;
    const ready = Promise.resolve(h).then((v) => (handler = v));
    return async (req: Request) => serveOne(handler === undefined ? await ready : handler, req);
  }
  return (req: Request) => serveOne(h as Iface | null, req);
}

// hostListen serves h on host:port with the host's server: Bun.serve,
// Deno.serve, or node:http. It calls listening(address, port) once it
// listens and done(error?) when it stops, and returns the function that
// stops it.
export function hostListen(
  id: number, host: string, port: number, h: Iface | null,
  listening: (addr: string, port: number) => void, done: (err?: Error) => void,
): () => void {
  const g = globalThis as any;
  const hostname = host === "" ? undefined : host;
  const info = (remoteAddr: string) => ({ remoteAddr, server: id });
  if (g.Bun?.serve) {
    let srv: any;
    try {
      srv = g.Bun.serve({
        port, hostname,
        fetch: (req: Request, server: any) => {
          const ip = server.requestIP?.(req);
          return serveOne(h, req, info(ip ? joinHostPort(ip.address, ip.port) : ""));
        },
      });
    } catch (e: any) {
      queueMicrotask(() => done(e instanceof Error ? e : new Error(String(e))));
      return () => {};
    }
    listening(srv.hostname ?? "", srv.port);
    return () => {
      srv.stop(true);
      done();
    };
  }
  if (g.Deno?.serve) {
    const ac = new AbortController();
    const srv = g.Deno.serve(
      { port, hostname, signal: ac.signal, onListen: (a: any) => listening(a.hostname, a.port), onError: () => new Response(null, { status: 500 }) },
      (req: Request, i: any) => serveOne(h, req, { ...info(i?.remoteAddr ? joinHostPort(i.remoteAddr.hostname, i.remoteAddr.port) : ""), noSignal: true }),
    );
    srv.finished.then(() => done(), (e: any) => done(e));
    return () => ac.abort();
  }
  const http = hostBuiltin("node:http");
  if (!http) plainPanic("goesm: http.ListenAndServe needs Node.js, Bun or Deno; elsewhere export a fetch handler (fetchHandler)");
  const srv = http.createServer((req: any, res: any) => nodeServe(h, req, res, info));
  srv.on("error", (e: any) => done(e));
  srv.on("close", () => done());
  srv.listen(port, hostname, () => {
    const a = srv.address();
    listening(a.address, a.port);
  });
  return () => {
    srv.close();
    srv.closeAllConnections?.();
  };
}

function joinHostPort(host: string, port: number): string {
  return host.includes(":") ? `[${host}]:${port}` : `${host}:${port}`;
}

// nodeServe serves one node:http request: the handler sees a request-like
// object (method, url, headers as [name, value] pairs, body, arrayBuffer, signal), and the
// Response is written back to res.
function nodeServe(h: Iface | null, req: any, res: any, info: (remote: string) => any): void {
  const chunks: Uint8Array[] = [];
  req.on("data", (c: Uint8Array) => chunks.push(c));
  req.on("end", async () => {
    const ac = new AbortController();
    res.on("close", () => ac.abort());
    let body: Uint8Array | null = null;
    if (chunks.length > 0) {
      let n = 0;
      for (const c of chunks) n += c.length;
      body = new Uint8Array(n);
      n = 0;
      for (const c of chunks) {
        body.set(c, n);
        n += c.length;
      }
    }
    const raw: string[] = req.rawHeaders;
    const pairs: [string, string][] = [];
    for (let i = 0; i < raw.length; i += 2) pairs.push([raw[i], raw[i + 1]]);
    const scheme = req.socket?.encrypted ? "https" : "http";
    const request = {
      method: req.method,
      url: `${scheme}://${req.headers.host ?? "localhost"}${req.url}`,
      headers: pairs,
      body,
      arrayBuffer: () => Promise.resolve(body!.buffer.slice(body!.byteOffset, body!.byteOffset + body!.byteLength)),
      signal: ac.signal,
    };
    let response: Response;
    try {
      response = await serveOne(h, request, info(joinHostPort(req.socket.remoteAddress ?? "", req.socket.remotePort ?? 0)));
    } catch {
      req.socket.destroy(); // a panic before the header: Go's server closes the connection
      return;
    }
    const headers: Record<string, string | string[]> = {};
    response.headers.forEach((v, k) => {
      if (k !== "set-cookie") headers[k] = v;
    });
    const cookies = response.headers.getSetCookie?.() ?? [];
    if (cookies.length > 0) headers["set-cookie"] = cookies;
    res.writeHead(response.status, headers);
    if (response.body === null) {
      res.end();
      return;
    }
    const reader = response.body.getReader();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        res.write(value);
      }
      res.end();
    } catch {
      res.destroy();
    }
  });
}
