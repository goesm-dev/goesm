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
  assert.equal(m.Sum([1, 2, 3]), 6);
});

test("struct + method", async () => {
  const m = await load("GOESM_BASICS");
  // A struct value is a plain object; its methods are functions of the module.
  const child = m.NewUser("b", 3);
  assert.deepEqual(child, { Name: "b", Age: 3 });
  assert.equal(m.User$Adult({ Name: "a", Age: 20 }), true);
  assert.equal(m.User$Adult(child), false);
  // Value semantics are Go's, not JS reference semantics.
  assert.deepEqual(m.StructCopy(), [19, 30, 100, 19]);
});

test("slice + range", async () => {
  const m = await load("GOESM_BASICS");
  assert.equal(m.SliceRange(), 10);
  assert.deepEqual(m.SliceAliasing(), [3, 10, 5, 5, 9, 7, 2, 9]);
});

test("map keeps Go semantics", async () => {
  const m = await load("GOESM_BASICS");
  // value, ok := m["a"]; missing value; deleted key; len; m["b"]
  assert.deepEqual(m.Map(), [1, true, 0, false, false, 1, 2]);
  assert.deepEqual(m.MapStructKeys(), [15, 1, 2]);
  // int(1), int64(1), "1" and Point{1,1} are four distinct interface keys.
  assert.equal(m.MapInterfaceKeys()[0], 4);
});

test("pointer", async () => {
  const m = await load("GOESM_BASICS");
  assert.equal(m.PointerExample(), 10);
  assert.deepEqual(m.PointerIdentity(), [true, false, true, false, true, true]);
});

test("defer follows Go return semantics", async () => {
  const m = await load("GOESM_BASICS");
  assert.deepEqual(m.DeferExample(), [1, 2]);
  assert.deepEqual(m.DeferNamed(), [1, 2, 3]);
});

test("interface dispatch", async () => {
  const m = await load("GOESM_BASICS");
  // An object of a Go struct class whose type has the methods is the interface value.
  assert.equal(m.Format(new m.Named("gopher")), "gopher");
  // A value whose type lacks String() is not a Stringer, whatever its JS shape.
  assert.deepEqual(m.TypeAssert(), ["x", true, false, "x", true]);
});

test("generics", async () => {
  const m = await load("GOESM_GENERICS");
  assert.equal(m.FirstInt(), 7);
  assert.equal(m.FirstString(), "a");
  assert.deepEqual(m.GenericMethod(), ["10", "20", "30"]);
  assert.equal(m.GenericMethodOnGenericType(), "123");
});

test("goroutine + channel", async () => {
  const m = await load("GOESM_GOROUTINES");
  const p = m.Example();
  assert.ok(p instanceof Promise, "blocking Go functions return Promises at the JS boundary");
  assert.equal(await p, 42);
  assert.deepEqual(await m.Pipeline(), [1, 4, 9, 16]);
});

test("panic surfaces as a GoPanic with a .go stack", async () => {
  const m = await load("GOESM_PANICS");
  assert.equal(m.Recover(), "recovered: boom");
  assert.throws(() => m.MustPositive(-1), (e) => {
    assert.ok(e instanceof Error && e.name === "GoPanic");
    assert.equal(e.message, "panic: negative");
    // With --enable-source-maps, frames point at the original Go file.
    assert.match(e.stack, /panics\.go:\d+/);
    return true;
  });
});

test("a nil dereference surfaces as a GoPanic, not a TypeError", async () => {
  const m = await load("GOESM_PANICS");
  for (const f of [() => m.Deref(null), () => m.IndexArrayPtr(null), () => m.FieldOf(null), () => m.NextVal({ Next: null, Val: 0 }), () => m.Call(null)]) assert.throws(f, (e) => {
    assert.equal(e.name, "GoPanic");
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
    assert.ok(e instanceof Error && e.name !== "GoPanic");
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
      assert.ok(e instanceof Error && e.name !== "GoPanic");
      assert.match(e.message, /^[xy]$/);
      return true;
    });
  }
  await assert.rejects(f.blockingPanic("boom"), (e) => {
    assert.equal(e.name, "GoPanic");
    assert.equal(e.message, "panic: boom");
    return true;
  });
});

test("strings convert between UTF-8 Go strings and UTF-16 JS strings", async () => {
  const rt = await import(new URL("../../runtime/src/index.ts", import.meta.url).href);
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

test("JS calling ABI: strings, numbers and results", async () => {
  const m = await load("GOESM_JSEXPORT");
  // Strings cross as JS strings: no conversion by hand, and no mojibake.
  assert.equal(m.Weekday(2026, 10, 5), "月曜日");
  assert.deepEqual(m.Count("日本語"), [3, 9]);
  assert.equal(m.Join("・", "東京", "大阪", "名古屋"), "東京・大阪・名古屋");
  assert.equal(m.Join(","), "");
  assert.equal(m.Square64(3_000_000_000n), 9_000_000_000_000_000_000n);
  assert.equal(m.Square64(12), 144n);
  assert.equal(await m.Later("今"), "later 今");
});

test("JS calling ABI: errors are thrown and come back as Go errors", async () => {
  const m = await load("GOESM_JSEXPORT");
  assert.equal(m.Sum("1, 2, 3"), 6);
  assert.throws(() => m.Sum("1, x"), (e) => {
    assert.ok(e instanceof Error);
    assert.equal(e.name, "GoError");
    assert.equal(e.message, 'strconv.Atoi: parsing "x": invalid syntax');
    assert.equal(m.IsEmpty(e), false);
    return true;
  });
  assert.throws(() => m.Sum(" "), (e) => m.IsEmpty(e));
  assert.equal(m.IsEmpty(null), false);
  assert.equal(m.IsEmpty(new Error("other")), false);
  assert.equal(m.Check(1), undefined);
  assert.throws(() => m.Check(-2), { name: "GoError", message: "negative: -2" });
});

test("JS calling ABI: structs, slices and maps are plain values", async () => {
  const m = await load("GOESM_JSEXPORT");
  const items = [
    { name: "りんご", price: 120, quantity: 3 },
    { name: "pan", price: 250 },
  ];
  assert.equal(m.Total({ ID: 1n, Items: items }), 610);
  assert.equal(m.Total({}), 0);
  assert.deepEqual(m.Cheapest(items), { name: "りんご", price: 120, quantity: 3, Tags: null });
  assert.deepEqual(m.Counts(["a", "b", "a"]), { a: 2, b: 1 });
  assert.deepEqual(m.Lengths(["go", "js", "ts!"]), new Map([[2, ["go", "js"]], [3, ["ts!"]]]));
  const b = m.Bytes(new Uint8Array([1, 2, 3]));
  assert.ok(b instanceof Uint8Array);
  assert.deepEqual([...b], [3, 2, 1]);
  assert.deepEqual([...m.Bytes([4, 5])], [5, 4]);
  assert.deepEqual(m.Decode('{"a": [1, "x", true, null]}'), { a: [1, "x", true, null] });
  assert.equal(m.Describe("é"), "string é");
  assert.equal(m.Describe({ a: 1 }), "object of 1");
  assert.equal(m.Describe([1, 2]), "array of 2");
  assert.equal(m.Describe(undefined), "nil");
});

test("JS calling ABI: pointers to types with methods are handles", async () => {
  const m = await load("GOESM_JSEXPORT");
  const c = m.NewCart("かなで");
  assert.ok(c instanceof m.Cart);
  c.Add({ name: "みかん", price: 100 }, { name: "柿", price: 80 });
  m.Cart$Add(c, { name: "梨", price: 300 });
  assert.equal(c.Len(), 3);
  assert.equal(c.Owner(), "かなで");
  assert.equal(m.Cart$Owner(c), "かなで");
  assert.deepEqual(c.Items().map((it) => it.name), ["みかん", "柿", "梨"]);
  assert.equal(m.Describe(c), "cart of かなで");
});

test("JS calling ABI: functions cross both ways", async () => {
  const m = await load("GOESM_JSEXPORT");
  assert.deepEqual(m.MapStrings(["あ", "b"], (s) => s + s), ["ああ", "bb"]);
  const ex = m.Exclaim();
  assert.equal(ex("やった"), "やった!");
  // A Go function that comes back to Go is called directly.
  assert.deepEqual(m.MapStrings(["x"], ex), ["x!"]);
});
