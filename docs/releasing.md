# Releasing goesm

[日本語](releasing.ja.md)

Releases are cut by pushing a tag. Nothing is published until a tag is pushed.

## Version scheme

While goesm is experimental: `v0.0.1-beta.N`, where `N` counts up from 1 (`v0.0.1-beta.1`, `v0.0.1-beta.2`, ...). These are semver prereleases, so the Go module proxy accepts them and `go install ...@latest` picks the newest one until a non-prerelease version exists.

Mind that semver orders prerelease identifiers numerically only when they are pure digits: `beta.10` sorts after `beta.9`, as intended. Do not write `beta10`.

## Cutting a release

1. Make sure CI on `main` is green.
2. Tag the commit and push the tag:

   ```sh
   git checkout main && git pull
   git tag v0.0.1-beta.1
   git push origin v0.0.1-beta.1
   ```

3. `.github/workflows/release.yml` runs the tests again, then [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) builds `goesm` for linux / darwin / windows × amd64 / arm64 with the version stamped in (`goesm version`), and creates a GitHub release marked as a prerelease with the archives, `checksums.txt` and a changelog.
4. The Go module proxy fetches the version the first time someone asks for it. To make it available right away:

   ```sh
   GOPROXY=https://proxy.golang.org go list -m github.com/goesm-dev/goesm@v0.0.1-beta.1
   ```

## Things that cannot be undone

* The Go module proxy and checksum database keep a version forever, even if the tag is deleted. Never move or re-push a tag that was pushed once; release a new `N` instead.
* A broken version can be marked with a `retract` directive in `go.mod` in a later release, which hides it from `@latest`.

## Checking the release locally

```sh
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser check
goreleaser release --snapshot --clean   # builds into dist/, publishes nothing
```
