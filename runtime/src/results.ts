// Several results. A Go function with more than one result that does not
// block returns its first result and leaves the others in $R (r1, r2, ...),
// which its caller reads right after the call, before anything else can
// call Go code: no array is allocated for the results, which V8 does not
// optimize away for a call it does not inline. A function that blocks (an
// async function) returns a Promise of an array of all its results, since
// other goroutines run, and overwrite $R, before its caller resumes; the
// caller then moves them into $R with untuple.
export const $R: any = { r1: null, r2: null, r3: null };

// untuple returns t[0] and leaves t[1], t[2], ... in $R.
export function untuple(t: any): any {
  $R.r1 = t[1];
  if (t.length > 2) {
    $R.r2 = t[2];
    for (let i = 3; i < t.length; i++) $R["r" + i] = t[i];
  }
  return t[0];
}

// tuple returns the n results of the call that returned r0 as an array.
export function tuple(r0: any, n: number): any[] {
  const t = new Array(n);
  t[0] = r0;
  t[1] = $R.r1;
  if (n > 2) {
    t[2] = $R.r2;
    for (let i = 3; i < n; i++) t[i] = $R["r" + i];
  }
  return t;
}
