// Several results. A Go function with more than one result that does not
// block returns its first result and leaves the others in $R (r1, r2, ...),
// which its caller reads right after the call, before anything else can
// call Go code: no array is allocated for the results, which V8 does not
// optimize away for a call it does not inline. A function that blocks (an
// async function) returns a Promise of an array of all its results, since
// other goroutines run, and overwrite $R, before its caller resumes; the
// caller then moves them into $R with untuple.
// $R has its registers from the start: a register added later would change
// its shape, and every access to it would slow down. Results beyond r8 go
// to properties added on first use.
export const $R: any = { r1: null, r2: null, r3: null, r4: null, r5: null, r6: null, r7: null, r8: null };

// untuple returns t[0] and leaves t[1], t[2], ... in $R. The registers are
// named in the code up to r8: a computed name costs a lookup on each use.
export function untuple(t: any): any {
  const n = t.length;
  $R.r1 = t[1];
  if (n <= 2) return t[0];
  $R.r2 = t[2];
  if (n <= 3) return t[0];
  $R.r3 = t[3];
  if (n <= 4) return t[0];
  $R.r4 = t[4];
  if (n <= 5) return t[0];
  $R.r5 = t[5];
  if (n <= 6) return t[0];
  $R.r6 = t[6];
  if (n <= 7) return t[0];
  $R.r7 = t[7];
  if (n <= 8) return t[0];
  $R.r8 = t[8];
  for (let i = 9; i < n; i++) $R["r" + i] = t[i];
  return t[0];
}

// tuple returns the n results of the call that returned r0 as an array.
export function tuple(r0: any, n: number): any[] {
  const t = new Array(n);
  t[0] = r0;
  t[1] = $R.r1;
  for (let i = 2; i < n; i++) t[i] = reg(i);
  return t;
}

function reg(i: number): any {
  switch (i) {
    case 2: return $R.r2;
    case 3: return $R.r3;
    case 4: return $R.r4;
    case 5: return $R.r5;
    case 6: return $R.r6;
    case 7: return $R.r7;
    case 8: return $R.r8;
  }
  return $R["r" + i];
}
