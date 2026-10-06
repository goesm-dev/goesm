# Releasing goesm

[日本語](releasing.ja.md)

Releases are cut by pushing a tag. Nothing is published until a tag is pushed.

## Version scheme

While goesm is experimental: `v0.0.1-beta.N`, where `N` counts up from 1 (`v0.0.1-beta.1`, `v0.0.1-beta.2`, ...). These are semver prereleases, so the Go module proxy accepts them and `go install ...@latest` picks the newest one until a non-prerelease version exists.

Mind that semver orders prerelease identifiers numerically only when they are pure digits: `beta.10` sorts after `beta.9`, as intended. Do not write `beta10`.

## Cutting a release

1. Write the release notes. In both [CHANGELOG.md](../CHANGELOG.md) and [CHANGELOG.ja.md](../CHANGELOG.ja.md), rename the `## Unreleased` section to the version (`## v0.0.1-beta.4`), replace its first line with the tag date and the compare link from the previous version, and start a new empty `## Unreleased` section above it. Merge that to `main`.
2. Make sure CI on `main` is green.
3. Tag the commit and push the tag:

   ```sh
   git checkout main && git pull
   git tag v0.0.1-beta.1
   git push origin v0.0.1-beta.1
   ```

4. `.github/workflows/release.yml` runs the tests again, then creates a **draft** GitHub release for the tag, marked as a prerelease when the version has a `-` suffix. Its notes are the tag's section of CHANGELOG.md followed by the one of CHANGELOG.ja.md, as `.github/scripts/release-notes.sh <tag>` prints them; when CHANGELOG.md has no section for the tag, they are generated from the merged PRs. Check the draft and publish it. No binaries are attached: users install with `go install` / `go get -tool`.
5. The Go module proxy fetches the version the first time someone asks for it. To make it available right away:

   ```sh
   GOPROXY=https://proxy.golang.org go list -m github.com/goesm-dev/goesm@v0.0.1-beta.1
   ```

## Fixing release notes

Edit the version's section in CHANGELOG.md and CHANGELOG.ja.md and merge it to `main`. `.github/workflows/release-notes.yml` then rewrites the notes of every existing release, drafts included, that has a section. It does not publish drafts or create releases. It can also be run by hand from the Actions tab.

## Things that cannot be undone

* The Go module proxy and checksum database keep a version forever, even if the tag is deleted. Never move or re-push a tag that was pushed once; release a new `N` instead.
* A broken version can be marked with a `retract` directive in `go.mod` in a later release, which hides it from `@latest`.
