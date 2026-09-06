# Releasing StePanel

The latest stable release is `v0.6.0`; the repository's documentation on
`main` is explicitly unreleased. The first public release uses the version
already recorded in `version.go`; do not retag it as
`v0.1.0`, because the release workflow intentionally rejects tags that do not
match the source version.

1. Update `version.go`, `CHANGELOG.md`, and any migration notes. Keep the
   Helm, Kubernetes, Terraform, and OpenAPI versions synchronized with
   `version.go`.
2. Run `make release-check` to verify that the Go version, Helm chart,
   OpenAPI document, and changelog agree. Then run
   `GOCACHE=/tmp/stepanel-go-cache GOFLAGS=-p=1 make check` locally when
   working on a constrained machine.
3. Review the generated release notes and confirm the supported upgrade path.
   Release automation rejects tags that do not match `version.go`.
4. Create and push an annotated tag:

   ```sh
   release_version=$(sed -n 's/const Version = "\([^"]*\)"/\1/p' version.go)
   git tag -a "v${release_version}" -m "StePanel v${release_version}"
   git push origin "v${release_version}"
   ```

5. The release workflow runs GoReleaser for Linux AMD64 and ARM64 installer
   archives containing the binary and installer support tree,
   creates tar archives and SHA-256 checksums, publishes provenance, pushes
   multi-architecture container images, and attaches the artifacts to the
   GitHub release. The workflow also attaches an SPDX SBOM generated for the
   checked-out source tree.

Never include database passwords, backup archives, production configuration, or
session secrets in release artifacts.
