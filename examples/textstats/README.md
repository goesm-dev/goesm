# textstats

[日本語](README.ja.md)

Text utilities written against the standard library, which goesm compiles from its Go source: `strings` (FieldsFunc, ToLower, Split, Join, TrimSpace), `strconv.Atoi`, `sort.Slice`, `unicode`, and `errors.Is` / `errors.Join`.

```sh
../bin/goesm build -o textstats/dist ./textstats && node textstats/index.mjs   # in examples/
```

`Slug` keeps letters of any script (`go-パッケージを-es-module-に`): strings are UTF-8 byte strings as in Go. `SumCSV` returns `(int, error)`, which arrives in JS as `[sum, err]`; `IsEmpty` shows `errors.Is` seeing through `errors.Join`'s wrapping.
