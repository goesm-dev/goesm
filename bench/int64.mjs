// Benchmark of 64-bit integer representations for goesm's lowering of
// int64 / uint64 (see ARCHITECTURE.md, "64-bit integers").
//
//   node bench/int64.mjs      bun bench/int64.mjs
//
// Each workload is written the way goesm would lower it under each
// representation, and every representation must produce the same checksum
// as the exact BigInt version (except "number", the inexact float64
// baseline goesm used before):
//
//   number   float64 (exact only below 2^53; the old lowering, for reference)
//   bigint   one BigInt per value, BigInt.asIntN/asUintN after each op
//   pairobj  an immutable {hi, lo} object per value (GopherJS-style)
//   pair     two uint32 locals per value; results of helpers come back in a
//            module-level scratch slot, so nothing allocates
//   hybrid   a Number while the value is a safe integer, a BigInt otherwise
//
// Workloads: sum (an int64 counter and accumulator that stay small), fnv
// (FNV-1a 64 over a byte slice), xorshift (xorshift64* random numbers) and
// time (Unix nanoseconds, beyond 2^53: add, divide, remainder).

const N_SUM = 10_000_000; // literal 10000000n in the bigint version
const BYTES = new Uint8Array(1 << 20).map((_, i) => (i * 31 + 7) & 255);
const N_FNV = 4;
const N_XS = 2_000_000;
const N_TIME = 1_000_000; // literal 1000000n in the bigint version

// ---- pair helpers (uint32 halves; signedness is a matter of interpretation)
let H = 0; // high half of the last helper result

function add64(ah, al, bh, bl) {
  const lo = (al + bl) >>> 0;
  H = (ah + bh + (lo < al ? 1 : 0)) >>> 0;
  return lo;
}

// high 32 bits of the 64-bit product of two uint32
function mulhi32(a, b) {
  const a1 = a >>> 16, a0 = a & 0xffff, b1 = b >>> 16, b0 = b & 0xffff;
  const p00 = a0 * b0, p01 = a0 * b1, p10 = a1 * b0, p11 = a1 * b1;
  const mid = (p00 >>> 16) + (p01 & 0xffff) + (p10 & 0xffff);
  return (p11 + Math.floor(p01 / 65536) + Math.floor(p10 / 65536) + Math.floor(mid / 65536)) >>> 0;
}

function mul64(ah, al, bh, bl) {
  H = (mulhi32(al, bl) + Math.imul(ah, bl) + Math.imul(al, bh)) >>> 0;
  return Math.imul(al, bl) >>> 0;
}

function shr64(h, l, n) { // logical, 0 < n < 32
  H = h >>> n;
  return ((l >>> n) | (h << (32 - n))) >>> 0;
}

function shl64(h, l, n) { // 0 < n < 32
  H = ((h << n) | (l >>> (32 - n))) >>> 0;
  return (l << n) >>> 0;
}

function toBig(h, l) {
  return BigInt.asIntN(64, (BigInt(h) << 32n) | BigInt(l));
}

function fromBig(v) {
  const u = BigInt.asUintN(64, v);
  H = Number(u >> 32n);
  return Number(u & 0xffffffffn);
}

// signed division: exact through a Number when both fit in 53 bits
function div64(ah, al, bh, bl) {
  const a = ah | 0, b = bh | 0;
  if ((a >= -0x200000 && a < 0x200000) && (b >= -0x200000 && b < 0x200000)) {
    const q = Math.trunc((a * 4294967296 + al) / (b * 4294967296 + bl));
    H = Math.floor(q / 4294967296) >>> 0;
    return q >>> 0;
  }
  return fromBig(toBig(ah, al) / toBig(bh, bl));
}

function rem64(ah, al, bh, bl) {
  const a = ah | 0, b = bh | 0;
  if ((a >= -0x200000 && a < 0x200000) && (b >= -0x200000 && b < 0x200000)) {
    const r = (a * 4294967296 + al) % (b * 4294967296 + bl);
    H = Math.floor(r / 4294967296) >>> 0;
    return r >>> 0;
  }
  return fromBig(toBig(ah, al) % toBig(bh, bl));
}

// ---- pairobj
class I64 {
  constructor(hi, lo) { this.hi = hi; this.lo = lo; }
}
const add = (a, b) => { const l = add64(a.hi, a.lo, b.hi, b.lo); return new I64(H, l); };
const mul = (a, b) => { const l = mul64(a.hi, a.lo, b.hi, b.lo); return new I64(H, l); };
const xor = (a, b) => new I64((a.hi ^ b.hi) >>> 0, (a.lo ^ b.lo) >>> 0);
const shr = (a, n) => { const l = shr64(a.hi, a.lo, n); return new I64(H, l); };
const shl = (a, n) => { const l = shl64(a.hi, a.lo, n); return new I64(H, l); };
const div = (a, b) => { const l = div64(a.hi, a.lo, b.hi, b.lo); return new I64(H, l); };
const rem = (a, b) => { const l = rem64(a.hi, a.lo, b.hi, b.lo); return new I64(H, l); };
const objOf = (v) => { const l = fromBig(BigInt(v)); return new I64(H, l); };
const objBig = (a) => toBig(a.hi, a.lo);

// ---- hybrid: number while safe, else bigint
function hadd(a, b) {
  if (typeof a === "number" && typeof b === "number") {
    const r = a + b;
    if (Number.isSafeInteger(r)) return r;
  }
  return hnorm(BigInt.asIntN(64, BigInt(a) + BigInt(b)));
}
function hmul(a, b) {
  if (typeof a === "number" && typeof b === "number") {
    const r = a * b;
    if (Number.isSafeInteger(r)) return r;
  }
  return hnorm(BigInt.asIntN(64, BigInt(a) * BigInt(b)));
}
function hdiv(a, b) {
  if (typeof a === "number" && typeof b === "number") return Math.trunc(a / b);
  return hnorm(BigInt(a) / BigInt(b));
}
function hrem(a, b) {
  if (typeof a === "number" && typeof b === "number") return a % b;
  return hnorm(BigInt(a) % BigInt(b));
}
function hnorm(v) {
  return v >= -9007199254740991n && v <= 9007199254740991n ? Number(v) : v;
}

// ---- workloads -------------------------------------------------------------

const workloads = {
  sum: {
    number() {
      let s = 0;
      for (let i = 0; i < N_SUM; i++) s += i;
      return BigInt(s);
    },
    bigint() {
      let s = 0n;
      for (let i = 0n; i < 10000000n; i = BigInt.asIntN(64, i + 1n)) s = BigInt.asIntN(64, s + i);
      return s;
    },
    pairobj() {
      let s = new I64(0, 0);
      const one = new I64(0, 1), n = objOf(N_SUM);
      for (let i = new I64(0, 0); i.hi < n.hi || (i.hi === n.hi && i.lo < n.lo); i = add(i, one)) s = add(s, i);
      return objBig(s);
    },
    pair() {
      let sh = 0, sl = 0;
      for (let ih = 0, il = 0; ih < 0 || (ih === 0 && il < N_SUM);) {
        sl = add64(sh, sl, ih, il); sh = H;
        il = add64(ih, il, 0, 1); ih = H;
      }
      return toBig(sh, sl);
    },
    hybrid() {
      let s = 0;
      for (let i = 0; i < N_SUM; i = hadd(i, 1)) s = hadd(s, i);
      return BigInt(s);
    },
  },

  fnv: {
    number() {
      let h = 14695981039346656037;
      for (let k = 0; k < N_FNV; k++) for (let i = 0; i < BYTES.length; i++) h = (h ^ BYTES[i]) * 1099511628211;
      return 0n; // inexact
    },
    bigint() {
      let h = 14695981039346656037n;
      for (let k = 0; k < N_FNV; k++) for (let i = 0; i < BYTES.length; i++) h = BigInt.asUintN(64, (h ^ BigInt(BYTES[i])) * 1099511628211n);
      return h;
    },
    pairobj() {
      let h = objOf(14695981039346656037n);
      const p = objOf(1099511628211);
      for (let k = 0; k < N_FNV; k++) for (let i = 0; i < BYTES.length; i++) h = mul(xor(h, new I64(0, BYTES[i])), p);
      return BigInt.asUintN(64, objBig(h));
    },
    pair() {
      let hh = 0xcbf29ce4, hl = 0x84222325;
      for (let k = 0; k < N_FNV; k++) for (let i = 0; i < BYTES.length; i++) {
        hl = mul64(hh, (hl ^ BYTES[i]) >>> 0, 0x100, 0x000001b3); hh = H;
      }
      return BigInt.asUintN(64, toBig(hh, hl));
    },
    hybrid() {
      // uint64 hashing leaves the safe range at once: this is the BigInt path
      // plus the typeof checks.
      let h = hnorm(BigInt.asIntN(64, 14695981039346656037n));
      for (let k = 0; k < N_FNV; k++) for (let i = 0; i < BYTES.length; i++) {
        const x = typeof h === "number" && (h | 0) === h ? h ^ BYTES[i] : hnorm(BigInt(h) ^ BigInt(BYTES[i]));
        h = hmul(x, 1099511628211);
      }
      return BigInt.asUintN(64, BigInt(h));
    },
  },

  xorshift: {
    number() {
      return 0n; // inexact; skipped
    },
    bigint() {
      let x = 88172645463325252n, out = 0n;
      for (let i = 0; i < N_XS; i++) {
        x ^= x >> 12n;
        x = BigInt.asUintN(64, x ^ (x << 25n));
        x ^= x >> 27n;
        out ^= BigInt.asUintN(64, x * 2685821657736338717n);
      }
      return out;
    },
    pairobj() {
      let x = objOf(88172645463325252n), out = new I64(0, 0);
      const m = objOf(2685821657736338717n);
      for (let i = 0; i < N_XS; i++) {
        x = xor(x, shr(x, 12));
        x = xor(x, shl(x, 25));
        x = xor(x, shr(x, 27));
        out = xor(out, mul(x, m));
      }
      return BigInt.asUintN(64, objBig(out));
    },
    pair() {
      let xl = fromBig(88172645463325252n), xh = H, oh = 0, ol = 0;
      for (let i = 0; i < N_XS; i++) {
        let tl = shr64(xh, xl, 12); xh = (xh ^ H) >>> 0; xl = (xl ^ tl) >>> 0;
        tl = shl64(xh, xl, 25); xh = (xh ^ H) >>> 0; xl = (xl ^ tl) >>> 0;
        tl = shr64(xh, xl, 27); xh = (xh ^ H) >>> 0; xl = (xl ^ tl) >>> 0;
        tl = mul64(xh, xl, 0x2545f491, 0x4f6cdd1d); oh = (oh ^ H) >>> 0; ol = (ol ^ tl) >>> 0;
      }
      return BigInt.asUintN(64, toBig(oh, ol));
    },
    hybrid() {
      return null; // shifts and xor of 64-bit values have no Number fast path
    },
  },

  time: {
    number() {
      let base = 1759500000123456789, acc = 0;
      for (let i = 0; i < N_TIME; i++) {
        const ns = base + i * 1000003;
        acc += Math.trunc(ns / 1e9) % 1000 + (ns % 1e9);
      }
      return 0n; // inexact
    },
    bigint() {
      const base = 1759500000123456789n;
      let acc = 0n;
      for (let i = 0n; i < 1000000n; i = BigInt.asIntN(64, i + 1n)) {
        const ns = BigInt.asIntN(64, base + BigInt.asIntN(64, i * 1000003n));
        acc = BigInt.asIntN(64, acc + (ns / 1000000000n) % 1000n + ns % 1000000000n);
      }
      return acc;
    },
    pairobj() {
      const base = objOf(1759500000123456789n), k = objOf(1000003), sec = objOf(1e9), th = objOf(1000);
      let acc = new I64(0, 0);
      for (let i = 0; i < N_TIME; i++) {
        const ns = add(base, mul(objOf(i), k));
        acc = add(acc, add(rem(div(ns, sec), th), rem(ns, sec)));
      }
      return objBig(acc);
    },
    pair() {
      const bl = fromBig(1759500000123456789n), bh = H;
      let ah = 0, al = 0;
      for (let i = 0; i < N_TIME; i++) {
        let nl = mul64(0, i, 0, 1000003), nh = H;
        nl = add64(bh, bl, nh, nl); nh = H;
        let ql = div64(nh, nl, 0, 1e9), qh = H;
        ql = rem64(qh, ql, 0, 1000); qh = H;
        const rl = rem64(nh, nl, 0, 1e9), rh = H;
        al = add64(ah, al, qh, ql); ah = H;
        al = add64(ah, al, rh, rl); ah = H;
      }
      return toBig(ah, al);
    },
    hybrid() {
      const base = hnorm(1759500000123456789n);
      let acc = 0;
      for (let i = 0; i < N_TIME; i++) {
        const ns = hadd(base, hmul(i, 1000003));
        acc = hadd(acc, hadd(hrem(hdiv(ns, 1000000000), 1000), hrem(ns, 1000000000)));
      }
      return BigInt(acc);
    },
  },
};

const reps = ["number", "bigint", "pairobj", "pair", "hybrid"];
const rows = [];
for (const [name, impls] of Object.entries(workloads)) {
  const want = impls.bigint();
  const row = { workload: name };
  for (const r of reps) {
    let got;
    let ms = Infinity;
    for (let k = 0; k < 5; k++) { // the first runs warm up; report the best
      const t0 = performance.now();
      got = impls[r]();
      ms = Math.min(ms, performance.now() - t0);
    }
    if (got === null) {
      row[r] = "n/a";
      continue;
    }
    const exact = r === "number" ? "" : got === want ? "" : " WRONG";
    row[r] = ms.toFixed(1) + " ms" + exact;
  }
  rows.push(row);
}
const runtime = typeof Bun !== "undefined" ? `Bun ${Bun.version}` : `Node ${process.version}`;
console.log(runtime);
console.table(rows);
