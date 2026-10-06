# goesm のリリース手順

[English](releasing.md)

リリースは tag の push で行います。tag を push するまで何も公開されません。

## version の付け方

goesm が実験段階のあいだは `v0.0.1-beta.N` とし、`N` は 1 から数えます (`v0.0.1-beta.1`、`v0.0.1-beta.2`、...)。これは semver の prerelease なので Go module proxy が受け付け、prerelease でない version ができるまでは `go install ...@latest` が最新のものを選びます。

semver が prerelease の識別子を数値として比べるのは数字だけのときです。`beta.10` は意図どおり `beta.9` の後に並びます。`beta10` とは書かないでください。

## リリースする

1. リリースノートを書きます。[CHANGELOG.md](../CHANGELOG.md) と [CHANGELOG.ja.md](../CHANGELOG.ja.md) の両方で、`## Unreleased` の節の見出しをバージョン名 `## v0.0.1-beta.4` に変え、節の最初の行をタグの日付と前のバージョンからの比較リンクに書き換えます。その上に、空の `## Unreleased` の節を新しく作ります。この変更を `main` に merge します。
2. `main` の CI が green であることを確認します。
3. commit に tag を付けて push します。

   ```sh
   git checkout main && git pull
   git tag v0.0.1-beta.1
   git push origin v0.0.1-beta.1
   ```

4. `.github/workflows/release.yml` がテストをもう一度実行し、その tag の GitHub release を **draft** として作ります。version に `-` の接尾辞があれば prerelease になります。notes は CHANGELOG.md のその tag の節に CHANGELOG.ja.md の節を続けたもので、`.github/scripts/release-notes.sh <tag>` の出力と同じです。CHANGELOG.md にその tag の節がなければ、merge された PR から notes を生成します。draft を確認して publish してください。binary は添付しません。利用者は `go install` / `go get -tool` でインストールします。
5. Go module proxy は、誰かが最初にその version を要求したときに取得します。すぐ使えるようにするには:

   ```sh
   GOPROXY=https://proxy.golang.org go list -m github.com/goesm-dev/goesm@v0.0.1-beta.1
   ```

## リリースノートを直す

CHANGELOG.md と CHANGELOG.ja.md のそのバージョンの節を直し、`main` に merge します。すると `.github/workflows/release-notes.yml` が、節のある既存の release すべての notes を書き換えます。draft も対象です。このワークフローは draft を publish せず、release を新しく作ることもありません。Actions タブから手動で実行することもできます。

## 取り消せないこと

* Go module proxy と checksum database は、tag を消しても version を永久に保持します。一度 push した tag を付け替えたり push し直したりせず、次の `N` をリリースしてください。
* 壊れた version は、後の release の `go.mod` に `retract` directive を書くことで `@latest` から外せます。
