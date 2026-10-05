// The JavaScript side of the jsimport benchmark (main.go).

export function add(a, b) {
  return a + b;
}

export function strlen(s) {
  return s.length;
}

export function upper(s) {
  return s.toUpperCase();
}

export function total(item) {
  return item.price * item.quantity;
}

export function sum(xs) {
  let t = 0;
  for (const x of xs) t += x;
  return t;
}

// jsLoop is the reference: n calls of f from JavaScript, in ns per call.
export function jsLoop(n, f, a, b) {
  let r;
  const t0 = performance.now();
  if (b === undefined) for (let i = 0; i < n; i++) r = f(a);
  else for (let i = 0; i < n; i++) r = f(a, b);
  const ns = ((performance.now() - t0) * 1e6) / n;
  globalThis.__sink = r;
  return ns;
}
