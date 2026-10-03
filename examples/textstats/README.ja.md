# textstats

[English](README.md)

標準 library で書いたテキスト処理です。標準 library は goesm が Go source から compile します: `strings` (FieldsFunc、ToLower、Split、Join、TrimSpace)、`strconv.Atoi`、`sort.Slice`、`unicode`、`errors.Is` / `errors.Join`。

```sh
../bin/goesm build -o textstats/dist ./textstats && node textstats/index.mjs   # examples/ で実行
```

`Slug` はどの文字体系の文字も残します (`go-パッケージを-es-module-に`)。string は Go と同じく UTF-8 の byte 列です。`SumCSV` は `(int, error)` を返し、JS には `[sum, err]` として届きます。`IsEmpty` は `errors.Join` で包まれた error も `errors.Is` が見分けることを示しています。
