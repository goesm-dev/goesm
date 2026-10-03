// Complex numbers (complex64, complex128). A complex value is an immutable
// object, so Go's value semantics need no copies; every operation returns a
// new value. complex64 values hold float32-rounded parts.

export class Complex {
  readonly re: number;
  readonly im: number;
  constructor(re: number, im: number) {
    this.re = re;
    this.im = im;
  }
}

export const complexZero: Complex = new Complex(0, 0);

export function complex(re: number, im: number): Complex {
  return new Complex(re, im);
}

// c64 rounds both parts to float32 (complex64 results and conversions).
export function c64(c: Complex): Complex {
  return new Complex(Math.fround(c.re), Math.fround(c.im));
}

export function cadd(a: Complex, b: Complex): Complex {
  return new Complex(a.re + b.re, a.im + b.im);
}

export function csub(a: Complex, b: Complex): Complex {
  return new Complex(a.re - b.re, a.im - b.im);
}

export function cmul(a: Complex, b: Complex): Complex {
  return new Complex(a.re * b.re - a.im * b.im, a.re * b.im + a.im * b.re);
}

export function cneg(a: Complex): Complex {
  return new Complex(-a.re, -a.im);
}

export function ceq(a: Complex, b: Complex): boolean {
  return a.re === b.re && a.im === b.im;
}

const inf = Infinity;
// zero is multiplied, not folded: 0 * x keeps NaN and the sign of zero.
let zero = 0;

function copysign(x: number, sign: number): number {
  const neg = sign < 0 || Object.is(sign, -0);
  return neg ? -Math.abs(x) : Math.abs(x);
}

function isFinite(x: number): boolean {
  return Number.isFinite(x);
}

function isInf(x: number): boolean {
  return x === inf || x === -inf;
}

function inf2one(x: number): number {
  return copysign(isInf(x) ? 1 : 0, x);
}

// cdiv is the runtime's complex128div: Smith's algorithm with C99 Annex G
// fix-ups for infinities and zeros. Division by zero does not panic.
export function cdiv(n: Complex, m: Complex): Complex {
  let e: number;
  let f: number;
  if (Math.abs(m.re) >= Math.abs(m.im)) {
    const ratio = m.im / m.re;
    const denom = m.re + ratio * m.im;
    e = (n.re + n.im * ratio) / denom;
    f = (n.im - n.re * ratio) / denom;
  } else {
    const ratio = m.re / m.im;
    const denom = m.im + ratio * m.re;
    e = (n.re * ratio + n.im) / denom;
    f = (n.im * ratio - n.re) / denom;
  }
  if (Number.isNaN(e) && Number.isNaN(f)) {
    let a = n.re;
    let b = n.im;
    let c = m.re;
    let d = m.im;
    if (c === 0 && d === 0 && (!Number.isNaN(a) || !Number.isNaN(b))) {
      e = copysign(inf, c) * a;
      f = copysign(inf, c) * b;
    } else if ((isInf(a) || isInf(b)) && isFinite(c) && isFinite(d)) {
      a = inf2one(a);
      b = inf2one(b);
      e = inf * (a * c + b * d);
      f = inf * (b * c - a * d);
    } else if ((isInf(c) || isInf(d)) && isFinite(a) && isFinite(b)) {
      c = inf2one(c);
      d = inf2one(d);
      e = zero * (a * c + b * d);
      f = zero * (b * c - a * d);
    }
  }
  return new Complex(e, f);
}
