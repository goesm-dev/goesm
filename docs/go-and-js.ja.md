# Go と JavaScript の違い

[English](go-and-js.md)

goesm は Go の意味論を保つので、JavaScript では計算結果が変わる処理でも、Go のコードはネイティブの Go と同じ結果を返します。JavaScript から移植したコードでは、正規表現、Unicode の正規化、丸め、一部の `math` 関数の最後のビット、文字列の位置でこの違いが現れます。パッケージ `github.com/goesm-dev/goesm/jscompat` は、必要な箇所で Go のコードに JavaScript と同じ振る舞いを提供します。文字列と数値を扱う関数は純粋な Go で書かれており、どこでもビルドできます。それ以外の関数は `js` ビルドでのみ使え、エンジンの機能を呼び出します。goesm では `//goesm:import` を、Go の WebAssembly 版では `syscall/js` を使って呼び出します。

## 正規表現

Go の `regexp` の構文は RE2 です。先読み、後読み、後方参照はなく、それらを使うパターンはコンパイルできません。`regexp` が受け付ける定数のパターンは、安全な場合には goesm がすでにエンジンの `RegExp` で照合します ([use-cases.ja.md](use-cases.ja.md#未対応))。そのようなパターンに追加の作業は要りません。JavaScript の構文で書くパターンには `jscompat.RegExp` を使います。

```go
var price = jscompat.MustCompileRegExp(`(?<=¥)(\d+)(?!\d*円)`, "u")

price.MatchString("¥1200")            // true
price.FindStringSubmatch("税込¥1200") // ["1200" "1200"]
```

メソッドは Go の文字列を受け取って Go の文字列を返し、JavaScript の規則に従います。照合に加わらなかったグループは `""` になります。マッチに隣接する空のマッチも数えるので、`x*` は `"axx"` から `""`、`"xx"`、`""` を見つけます。`ReplaceAllString` は、JavaScript の置換パターン (`$1`、`$<name>`、`$&`) を使います。位置を返すメソッドはありません。返す位置は UTF-16 の位置になるからです (後述)。

## Unicode の正規化

標準ライブラリには Unicode の正規化がありません。`golang.org/x/text/unicode/norm` は goesm でも動きますが、その表がバンドルに入ります。`jscompat.Normalize(s, "NFKC")` は JavaScript の `String.prototype.normalize` を呼び出します。指定できる形式は `"NFC"`、`"NFD"`、`"NFKC"`、`"NFKD"` です。

## 丸めと Math の関数

`math.Round` は 0.5 を 0 から遠い方へ丸め、JavaScript の `Math.round` は正の無限大の方へ丸めます。たとえば `math.Round(-2.5)` は -3 で、`Math.round(-2.5)` は -2 です。`jscompat.Round` は `Math.round` と同じく丸め、負のゼロも同じ結果になります。この関数は純粋な Go で書かれています。

`math.Cos`、`math.Sin`、`math.Exp`、`math.Log` など、正確に丸めることが保証されていない関数は Go のアルゴリズムで計算するので、ネイティブの Go と同じ結果になります。エンジンの `Math` の関数とは、最後のビットが違う場合があります。JavaScript のコードと完全に同じ結果が必要な場合は、エンジンの関数を呼ぶ `jscompat.Cos`、`Sin`、`Tan`、`Exp`、`Log`、`Pow`、`Atan2` を使います。`math.Sqrt` や `math.Floor` など、正確に丸める関数の結果はどちらでも同じです。

## 文字列の位置

Go の文字列は UTF-8 のバイト列で、JavaScript の文字列は UTF-16 のコード単位の列です。たとえば `"あ"` の長さは Go では 3、JavaScript では 1 で、`"😀"` の長さはそれぞれ 4 と 2 です。入力要素の `selectionStart` や `String.prototype.indexOf` の戻り値など、ブラウザから受け取る位置は UTF-16 のコード単位で数えます。`jscompat.UTF16Offset(s, i)` は `s` のバイト位置を UTF-16 の位置に変換し、`jscompat.ByteOffset(s, u)` はその逆に変換します。`jscompat.UTF16Len(s)` は、JavaScript での `s` の `length` を返します。サロゲートペアの 2 つの半分の間の位置は、Go では表せません。`ByteOffset` はその位置に対して文字の先頭の位置を返します。

```go
start := jscompat.ByteOffset(text, input.Get("selectionStart").Int())
end := jscompat.ByteOffset(text, input.Get("selectionEnd").Int())
selected := text[start:end]
```
