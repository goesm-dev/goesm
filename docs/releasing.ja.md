# goesm のリリース手順

[English](releasing.md)

リリースは tag の push で行います。tag を push するまで何も公開されません。

## version の付け方

goesm が実験段階のあいだは `v0.0.1-beta.N` とし、`N` は 1 から数えます (`v0.0.1-beta.1`、`v0.0.1-beta.2`、...)。これは semver の prerelease なので Go module proxy が受け付け、prerelease でない version ができるまでは `go install ...@latest` が最新のものを選びます。

semver が prerelease の識別子を数値として比べるのは数字だけのときです。`beta.10` は意図どおり `beta.9` の後に並びます。`beta10` とは書かないでください。

## リリースする

1. `main` の CI が green であることを確認します。
2. commit に tag を付けて push します。

   ```sh
   git checkout main && git pull
   git tag v0.0.1-beta.1
   git push origin v0.0.1-beta.1
   ```

3. `.github/workflows/release.yml` がテストをもう一度実行し、その tag の GitHub release を、merge された PR から生成した notes 付きの **draft** として作ります。version に `-` の接尾辞があれば prerelease になります。notes を編集して publish してください。binary は添付しません。利用者は `go install` / `go get -tool` でインストールします。
4. Go module proxy は、誰かが最初にその version を要求したときに取得します。すぐ使えるようにするには:

   ```sh
   GOPROXY=https://proxy.golang.org go list -m github.com/goesm-dev/goesm@v0.0.1-beta.1
   ```

## 取り消せないこと

* Go module proxy と checksum database は、tag を消しても version を永久に保持します。一度 push した tag を付け替えたり push し直したりせず、次の `N` をリリースしてください。
* 壊れた version は、後の release の `go.mod` に `retract` directive を書くことで `@latest` から外せます。
