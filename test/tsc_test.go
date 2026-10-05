package test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goesm-dev/goesm/internal/build"
	"github.com/goesm-dev/goesm/internal/lower"
)

// tscConfig is the strictest common setup a consumer of goesm's TypeScript
// may use: strict mode, imports with .ts extensions, verbatimModuleSyntax
// and erasableSyntaxOnly (the output runs under type-stripping runtimes).
const tscConfig = `{
  "compilerOptions": {
    "target": "es2022",
    "module": "esnext",
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "noEmit": true,
    "strict": true,
    "verbatimModuleSyntax": true,
    "erasableSyntaxOnly": true,
    "lib": ["es2022", "dom"]
  },
  "include": ["**/*.ts"]
}
`

// consumer uses exported Go APIs from TypeScript: the declared types must
// be the Go signatures (not any), so wrong uses are type errors.
const consumer = `import * as cart from "./example.com/examples/cart.ts";
import * as workers from "./example.com/examples/workers.ts";

const items = [{ Name: "りんご", Price: 120, Quantity: 3 }];
const total: number = cart.Total(items);
const discounted: number = cart.Discount(total, 15);
const receipt: string = cart.Receipt(items);
const sum: Promise<number> = workers.SumSquares(10, 2);
const square: number = workers.Square(3);
export { discounted, receipt, sum, square };

// @ts-expect-error a string is not a []Item
cart.Total("apple");
// @ts-expect-error Price is a number
cart.Total([{ Name: "apple", Price: "120" }]);
// @ts-expect-error Receipt returns a string
const notString: number = cart.Receipt(items);
// @ts-expect-error SumSquares blocks, so it returns a Promise
const notPromise: number = workers.SumSquares(10, 2);
export { notString, notPromise };
`

// projectConfig is a stock application setup, as create-next-app writes
// it, plus the unused-code checks many projects add. goesm's output does not
// pass it (BigInt literals need ES2020, generated code has unused locals), so
// the generated files carry @ts-nocheck; callers must keep their Go types.
const projectConfig = `{
  "compilerOptions": {
    "target": "es2017",
    "module": "esnext",
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "noEmit": true,
    "strict": true,
    "isolatedModules": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "lib": ["es2022", "dom"]
  },
  "include": ["**/*.ts"]
}
`

// TestTSC type-checks the TypeScript goesm emits (fixtures, examples and
// the runtime) with tsc (pinned in test/package.json) in strict mode, and
// checks that exported Go APIs carry their Go types for TypeScript callers.
// It is skipped when tsc is not installed, unless GOESM_REQUIRE_TOOLS is set
// (as in CI).
func TestTSC(t *testing.T) {
	bin, _ := filepath.Abs(filepath.Join("node_modules", ".bin", "tsc"))
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("GOESM_REQUIRE_TOOLS") != "" {
			t.Fatalf("tsc is not installed: run npm ci in test/")
		}
		t.Skip("tsc is not installed (run npm ci in test/)")
	}
	out := t.TempDir()
	examples, _ := filepath.Abs(filepath.Join("..", "examples"))

	// In a project with its own settings, @ts-nocheck silences the generated
	// files but not the consumer: its @ts-expect-error lines still need the
	// Go types.
	project := t.TempDir()
	for _, pattern := range []string{"./cart", "./workers"} {
		l, err := build.Lower(examples, []string{pattern})
		if err != nil {
			t.Fatalf("goesm %s: %v", pattern, err)
		}
		if err := build.WriteTS(project, l.Mods); err != nil {
			t.Fatal(err)
		}
	}
	for name, src := range map[string]string{"tsconfig.json": projectConfig, "consumer.ts": consumer} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if res, err := exec.Command(bin, "-p", project).CombinedOutput(); err != nil {
		t.Fatalf("tsc reported errors in a project that imports goesm's output: %v\n%s", err, res)
	}

	for _, tg := range []struct{ dir, pattern string }{
		{testdata("example"), "./main"},
		{testdata("semantics"), "./basics"},
		{testdata("semantics"), "./complexnum"},
		{testdata("semantics"), "./conformance"},
		{testdata("semantics"), "./generics"},
		{testdata("semantics"), "./goroutines"},
		{testdata("semantics"), "./int64s"},
		{testdata("semantics"), "./jsexport"},
		{testdata("semantics"), "./jsfuncs"},
		{testdata("semantics"), "./jsfuncsblocking"},
		{testdata("semantics"), "./panics"},
		{testdata("semantics"), "./stdlibuse"},
		{testdata("programs"), "./pipe"},
		{testdata("programs"), "./stdio"},
		{testdata("programs"), "./ifacevalues"},
		{testdata("programs"), "./condwait"},
		{testdata("programs"), "./lockforms"},
		{testdata("programs"), "./goexitmain"},
		{testdata("programs"), "./initpanic"},
		{testdata("programs"), "./fmtverbs"},
		{testdata("programs"), "./reflection"},
		{testdata("programs"), "./jsoncodec"},
		{testdata("programs"), "./timers"},
		{testdata("programs"), "./mathfuncs"},
		{testdata("programs"), "./jsnames"},
		{examples, "./cart"},
		{examples, "./textstats"},
		{examples, "./workers"},
	} {
		l, err := build.Lower(tg.dir, []string{tg.pattern})
		if err != nil {
			t.Fatalf("goesm %s: %v", tg.pattern, err)
		}
		if err := build.WriteTS(out, l.Mods); err != nil {
			t.Fatal(err)
		}
	}
	// The generated files turn type checking off for the projects that
	// import them (lower.NoCheck); here it stays on.
	err := filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".ts") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), lower.NoCheck) {
			return fmt.Errorf("%s does not have the @ts-nocheck header", p)
		}
		return os.WriteFile(p, []byte(strings.Replace(string(src), lower.NoCheck, "", 1)), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{"tsconfig.json": tscConfig, "consumer.ts": consumer} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(bin, "-p", out)
	res, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tsc reported errors in TypeScript generated by goesm: %v\n%s", err, res)
	}
}
