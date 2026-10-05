// Runtime integration tests: import the ES modules goesm built and check
// the values the PoC must produce. Paths come from the Go test driver.
import { test } from "node:test";
import assert from "node:assert/strict";

const load = (env) => import(process.env[env]);

test("package import: Result() from example.com/app/main", async () => {
  const m = await load("GOESM_EXAMPLE");
  assert.equal(m.Result(), 3);
});

test("function", async () => {
  const m = await load("GOESM_BASICS");
  assert.equal(m.PointerExample(), 10);
  assert.equal(m.Sum(m.$runtime.sliceLit([1, 2, 3])), 6);
});

test("struct + method", async () => {
  const m = await load("GOESM_BASICS");
  const adult = new m.User("a", 20);
  const child = m.NewUser("b", 3);
  assert.equal(adult.Adult(), true);
  assert.equal(child.Adult(), false);
  // Value semantics are Go's, not JS reference semantics.
  assert.deepEqual(m.$runtime.toArray(m.StructCopy()), [19, 30, 100, 19]);
});

test("slice + range", async () => {
  const m = await load("GOESM_BASICS");
  assert.equal(m.SliceRange(), 10);
  assert.deepEqual(m.$runtime.toArray(m.SliceAliasing()), [3, 10, 5, 5, 9, 7, 2, 9]);
});

test("map keeps Go semantics", async () => {
  const m = await load("GOESM_BASICS");
  const rt = m.$runtime;
  const r = rt.toArray(m.Map()).map((x) => x.v);
  // value, ok := m["a"]; missing value; deleted key; len; m["b"]
  assert.deepEqual(r, [1, true, 0, false, false, 1, 2]);
  assert.deepEqual(rt.toArray(m.MapStructKeys()), [15, 1, 2]);
  // int(1), int64(1), "1" and Point{1,1} are four distinct interface keys.
  assert.equal(rt.toArray(m.MapInterfaceKeys())[0].v, 4);
});

test("pointer", async () => {
  const m = await load("GOESM_BASICS");
  assert.equal(m.PointerExample(), 10);
  assert.deepEqual(m.$runtime.toArray(m.PointerIdentity()), [true, false, true, false, true, true]);
});

test("defer follows Go return semantics", async () => {
  const m = await load("GOESM_BASICS");
  assert.deepEqual(m.$runtime.toArray(m.DeferExample()), [1, 2]);
  assert.deepEqual(m.$runtime.toArray(m.DeferNamed()), [1, 2, 3]);
});

test("interface dispatch", async () => {
  const m = await load("GOESM_BASICS");
  const rt = m.$runtime;
  const v = rt.box(m.Named$type, new m.Named("gopher"));
  assert.equal(m.Format(v), "gopher");
  // A value whose type lacks String() is not a Stringer, whatever its JS shape.
  assert.deepEqual(rt.toArray(m.TypeAssert()).map((x) => x.v), ["x", true, false, "x", true]);
});

test("generics", async () => {
  const m = await load("GOESM_GENERICS");
  assert.equal(m.FirstInt(), 7);
  assert.equal(m.FirstString(), "a");
  assert.deepEqual(m.$runtime.toArray(m.GenericMethod()), ["10", "20", "30"]);
  assert.equal(m.GenericMethodOnGenericType(), "123");
});

test("goroutine + channel", async () => {
  const m = await load("GOESM_GOROUTINES");
  const p = m.Example();
  assert.ok(p instanceof Promise, "blocking Go functions return Promises at the JS boundary");
  assert.equal(await p, 42);
  assert.deepEqual(m.$runtime.toArray(await m.Pipeline()), [1, 4, 9, 16]);
});

test("panic surfaces as a GoPanic with a .go stack", async () => {
  const m = await load("GOESM_PANICS");
  const rt = m.$runtime;
  assert.equal(m.Recover(), "recovered: boom");
  assert.throws(() => m.MustPositive(-1), (e) => {
    assert.ok(e instanceof rt.GoPanic);
    assert.equal(e.message, "panic: negative");
    // With --enable-source-maps, frames point at the original Go file.
    assert.match(e.stack, /panics\.go:\d+/);
    return true;
  });
});

test("a nil dereference surfaces as a GoPanic, not a TypeError", async () => {
  const m = await load("GOESM_PANICS");
  for (const f of [() => m.Deref(null), () => m.IndexArrayPtr(null), () => m.FieldOf(null), () => m.NextVal({ Next: null, Val: 0 }), () => m.Call(null)]) assert.throws(f, (e) => {
    assert.ok(e instanceof m.$runtime.GoPanic);
    assert.equal(e.message, "panic: runtime error: invalid memory address or nil pointer dereference");
    return true;
  });
});

test("js.FuncOf passes a returned Promise to JavaScript as it is", async () => {
  const m = await load("GOESM_JSFUNCS");
  m.Setup();
  const f = globalThis.jsfuncs;
  assert.equal(f.value(21), 42);
  const resolved = f.resolve("ok");
  assert.ok(resolved instanceof Promise);
  assert.equal(await resolved, "ok");
  await assert.rejects(f.reject("x"), (e) => {
    assert.ok(e instanceof Error && !(e instanceof m.$runtime.GoPanic));
    assert.equal(e.message, "x");
    return true;
  });
});

test("js.FuncOf of a blocking Go function returns a Promise of its result", async () => {
  const m = await load("GOESM_JSFUNCS_BLOCKING");
  m.Setup();
  const f = globalThis.jsfuncsblocking;
  assert.equal(await f.value(21), 42);
  assert.equal(await f.resolve("ok"), "ok");
  assert.equal(await f.blocking(5), 15);
  for (const p of [f.reject("x"), f.blockingReject("y")]) {
    await assert.rejects(p, (e) => {
      assert.ok(e instanceof Error && !(e instanceof m.$runtime.GoPanic));
      assert.match(e.message, /^[xy]$/);
      return true;
    });
  }
  await assert.rejects(f.blockingPanic("boom"), (e) => {
    assert.ok(e instanceof m.$runtime.GoPanic);
    assert.equal(e.message, "panic: boom");
    return true;
  });
});

test("strings convert between UTF-8 Go strings and UTF-16 JS strings", async () => {
  const rt = (await load("GOESM_BASICS")).$runtime;
  const latin1 = (bytes) => String.fromCharCode(...bytes);
  const samples = ["", "hello", "こんにちは、世界", "é ß ü", "😀 𝄞 emoji", "ǅࠀ￿\u{10000}\u{10ffff}", "あ".repeat(20000)];
  // Long runs of ASCII or of surrogate pairs still go to String.fromCharCode in bounded chunks.
  samples.push("é" + "a".repeat(1 << 20), "😀".repeat(1 << 18));
  for (const s of samples) {
    const goStr = rt.fromJSString(s);
    if (s.length < 100) assert.equal(goStr, latin1(new TextEncoder().encode(s)), s);
    else assert.equal(goStr.length, new TextEncoder().encode(s).length);
    assert.equal(rt.toJSString(goStr), s);
  }
  // A lone surrogate becomes U+FFFD, as Go converts it.
  assert.equal(rt.toJSString(rt.fromJSString("a\ud800b\udc00")), "a�b�");
  // Each byte of an invalid sequence becomes U+FFFD, as range over a Go string decodes it.
  assert.equal(rt.toJSString("\xff"), "�");
  assert.equal(rt.toJSString("\xe3\x81x"), "��x");
  assert.equal(rt.toJSString("\xed\xa0\x80"), "���"); // an encoded surrogate
  assert.equal(rt.toJSString("\xf0\x9f\x98"), "���");
});
