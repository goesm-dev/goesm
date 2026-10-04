// The kernels written by hand in idiomatic JavaScript: the reference for
// what the JS engine itself does with each workload. They compute the same
// checksums as the Go kernels (bench/kernels) but are not Go: typed arrays
// instead of slices, Map instead of Go maps, JSON.stringify instead of
// encoding/json, no bounds or nil checks beyond the engine's own. Channels
// has no counterpart.

export function Fib(n) {
  return n < 2 ? n : Fib(n - 1) + Fib(n - 2);
}

export function Sieve(n) {
  const composite = new Uint8Array(n);
  let count = 0;
  for (let i = 2; i < n; i++) {
    if (composite[i]) continue;
    count++;
    for (let j = i * i; j < n; j += i) composite[j] = 1;
  }
  return count;
}

export function Mandelbrot(size) {
  let inside = 0;
  for (let y = 0; y < size; y++) {
    const ci = (2 * y) / size - 1;
    for (let x = 0; x < size; x++) {
      const cr = (2 * x) / size - 1.5;
      let zr = 0, zi = 0, i = 0;
      for (; i < 50 && zr * zr + zi * zi <= 4; i++) {
        const t = zr * zr - zi * zi + cr;
        zi = 2 * zr * zi + ci;
        zr = t;
      }
      if (i === 50) inside++;
    }
  }
  return inside;
}

function testBytes(n) {
  const b = new Uint8Array(n);
  for (let i = 0; i < n; i++) b[i] = (i * 31 + 7) & 255;
  return b;
}

export function FNV32(n) {
  const data = testBytes(n);
  let h = 2166136261;
  for (let r = 0; r < 8; r++) {
    for (let i = 0; i < data.length; i++) h = Math.imul(h ^ data[i], 16777619);
  }
  return (h >>> 0) >>> 1;
}

export function FNV64(n) {
  const data = testBytes(n);
  let h = 14695981039346656037n;
  for (let r = 0; r < 8; r++) {
    for (let i = 0; i < data.length; i++) h = BigInt.asUintN(64, (h ^ BigInt(data[i])) * 1099511628211n);
  }
  return Number(h >> 34n);
}

const SOLAR_MASS = 4 * Math.PI * Math.PI;
const DAYS_PER_YEAR = 365.24;

class Body {
  constructor(x, y, z, vx, vy, vz, mass) {
    Object.assign(this, { x, y, z, vx: vx * DAYS_PER_YEAR, vy: vy * DAYS_PER_YEAR, vz: vz * DAYS_PER_YEAR, mass: mass * SOLAR_MASS });
  }
}

function newSystem() {
  const bodies = [
    new Body(0, 0, 0, 0, 0, 0, 1),
    new Body(4.84143144246472090e+00, -1.16032004402742839e+00, -1.03622044471123109e-01,
      1.66007664274403694e-03, 7.69901118419740425e-03, -6.90460016972063023e-05, 9.54791938424326609e-04),
    new Body(8.34336671824457987e+00, 4.12479856412430479e+00, -4.03523417114321381e-01,
      -2.76742510726862411e-03, 4.99852801234917238e-03, 2.30417297573763929e-05, 2.85885980666130812e-04),
    new Body(1.28943695621391310e+01, -1.51111514016986312e+01, -2.23307578892655734e-01,
      2.96460137564761618e-03, 2.37847173959480950e-03, -2.96589568540237556e-05, 4.36624404335156298e-05),
    new Body(1.53796971148509165e+01, -2.59193146099879641e+01, 1.79258772950371181e-01,
      2.68067772490389322e-03, 1.62824170038242295e-03, -9.51592254519715870e-05, 5.15138902046611451e-05),
  ];
  let px = 0, py = 0, pz = 0;
  for (const b of bodies) {
    px += b.vx * b.mass;
    py += b.vy * b.mass;
    pz += b.vz * b.mass;
  }
  bodies[0].vx = -px / SOLAR_MASS;
  bodies[0].vy = -py / SOLAR_MASS;
  bodies[0].vz = -pz / SOLAR_MASS;
  return bodies;
}

function advance(bodies, dt) {
  for (let i = 0; i < bodies.length; i++) {
    const bi = bodies[i];
    for (let j = i + 1; j < bodies.length; j++) {
      const bj = bodies[j];
      const dx = bi.x - bj.x, dy = bi.y - bj.y, dz = bi.z - bj.z;
      const d2 = dx * dx + dy * dy + dz * dz;
      const mag = dt / (d2 * Math.sqrt(d2));
      bi.vx -= dx * bj.mass * mag;
      bi.vy -= dy * bj.mass * mag;
      bi.vz -= dz * bj.mass * mag;
      bj.vx += dx * bi.mass * mag;
      bj.vy += dy * bi.mass * mag;
      bj.vz += dz * bi.mass * mag;
    }
  }
  for (const b of bodies) {
    b.x += dt * b.vx;
    b.y += dt * b.vy;
    b.z += dt * b.vz;
  }
}

function energy(bodies) {
  let e = 0;
  for (let i = 0; i < bodies.length; i++) {
    const b = bodies[i];
    e += 0.5 * b.mass * (b.vx * b.vx + b.vy * b.vy + b.vz * b.vz);
    for (let j = i + 1; j < bodies.length; j++) {
      const b2 = bodies[j];
      const dx = b.x - b2.x, dy = b.y - b2.y, dz = b.z - b2.z;
      e -= (b.mass * b2.mass) / Math.sqrt(dx * dx + dy * dy + dz * dz);
    }
  }
  return e;
}

export function NBody(n) {
  const bodies = newSystem();
  for (let i = 0; i < n; i++) advance(bodies, 0.01);
  // Go's math.Round rounds half away from zero; the value is not a half.
  return Math.round(-energy(bodies) * 1e9);
}

class Node {
  constructor(left, right) {
    this.left = left;
    this.right = right;
  }
  check() {
    return this.left === null ? 1 : 1 + this.left.check() + this.right.check();
  }
}

function bottomUp(depth) {
  return depth === 0 ? new Node(null, null) : new Node(bottomUp(depth - 1), bottomUp(depth - 1));
}

export function BinaryTrees(maxDepth) {
  let total = 0;
  for (let depth = 4; depth <= maxDepth; depth += 2) {
    const iterations = 1 << (maxDepth - depth + 4);
    for (let i = 0; i < iterations; i++) total += bottomUp(depth).check();
  }
  return total;
}

class Rect {
  constructor(w, h) { this.w = w; this.h = h; }
  area() { return this.w * this.h; }
  scale(f) { return new Rect(this.w * f, this.h * f); }
}
class Circle {
  constructor(r) { this.r = r; }
  area() { return Math.PI * this.r * this.r; }
  scale(f) { return new Circle(this.r * f); }
}
class Triangle {
  constructor(b, h) { this.b = b; this.h = h; }
  area() { return (this.b * this.h) / 2; }
  scale(f) { return new Triangle(this.b * f, this.h * f); }
}

export function Interfaces(n) {
  const shapes = [];
  for (let i = 0; i < 1000; i++) {
    const f = (i % 10) + 1;
    shapes.push(i % 3 === 0 ? new Rect(f, f + 1) : i % 3 === 1 ? new Circle(f) : new Triangle(f, f * 2));
  }
  let sum = 0;
  for (let i = 0; i < n; i++) sum += shapes[i % shapes.length].scale(1.5).area();
  return Math.trunc(sum) % 1000000007;
}

export function MapInt(n) {
  const m = new Map();
  for (let i = 0; i < n; i++) m.set(i * 7, i);
  let sum = 0;
  for (let i = 0; i < n; i++) {
    sum += m.get(i * 7) & 0xff;
    if (i % 2 === 0) m.delete(i * 7);
  }
  return sum + m.size;
}

function lcg(seed) {
  let s = seed;
  return () => (s = (Math.imul(s, 1664525) + 1013904223) >>> 0);
}

export function MapString(n) {
  const words = [];
  for (let i = 0; i < 1000; i++) words.push("word" + i * 7919);
  const counts = new Map();
  const next = lcg(1);
  for (let i = 0; i < n; i++) {
    const w = words[next() % 1000];
    counts.set(w, (counts.get(w) ?? 0) + 1);
  }
  let best = 0;
  for (const c of counts.values()) if (c > best) best = c;
  return best * 10000 + counts.size;
}

export function Strings(n) {
  const ids = [];
  for (let i = 0; i < n; i++) ids.push("id" + i);
  const parts = ids.join(",").split(",");
  let total = 0;
  for (const p of parts) if (p.endsWith("7")) total += p.toUpperCase().length;
  return total + parts.join(";").length;
}

export function Sort(n) {
  const next = lcg(42);
  const ints = new Int32Array(n);
  const strs = new Array(n);
  for (let i = 0; i < n; i++) {
    ints[i] = next() >>> 2;
    strs[i] = String(next() % 100000);
  }
  ints.sort();
  strs.sort();
  let sum = 0;
  for (let i = 0; i < n; i += Math.trunc(n / 100)) sum = (sum + (ints[i] % 1000) + strs[i].length) % 1000000007;
  return sum + strs[n >> 1].length;
}

export function JSON_(n) {
  const records = [];
  for (let i = 0; i < n; i++) {
    records.push({
      id: i,
      name: "user" + i,
      email: "user" + i + "@example.com",
      active: i % 3 === 0,
      score: i * 1.25,
      tags: ["a", "b", String(i % 10)],
      attrs: { k: "v" + i },
    });
  }
  const data = new TextEncoder().encode(JSON.stringify(records));
  const back = JSON.parse(new TextDecoder().decode(data));
  let sum = data.length;
  for (const r of back) sum += (r.id % 7) + r.tags.length + r.attrs.k.length;
  return sum;
}
export { JSON_ as JSON };

export function Sprintf(n) {
  let total = 0;
  for (let i = 0; i < n; i++) {
    total += `${i}:item:${(i / 3).toFixed(2)}:${(i * 31).toString(16)}|${i % 2 === 0}`.length;
  }
  return total;
}

export function Add(a, b) {
  return a + b;
}

export function Upper(s) {
  return s.toUpperCase();
}

export function Handle(req) {
  let r;
  try {
    r = JSON.parse(req);
  } catch (e) {
    return JSON.stringify({ error: String(e) });
  }
  let count = 0, total = 0;
  for (const it of r.items ?? []) {
    count += it.qty;
    total += it.price * it.qty;
  }
  return JSON.stringify({ user: r.user, count, total });
}
