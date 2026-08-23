# Releasing StePanel

1. Update `version.go`, `CHANGELOG.md`, and any migration notes.
2. Run `GOCACHE=/tmp/stepanel-go-cache GOFLAGS=-p=1 make check` locally when
   working on a constrained machine.
3. Review the generated release notes and confirm the supported upgrade path.
4. Create and push an annotated tag:

   ```sh
   git tag -a v0.2.0 -m 'StePanel v0.2.0'
   git push origin v0.2.0
   ```

5. The release workflow builds Linux AMD64 and ARM64 binaries, creates checksums
   and an SBOM, publishes provenance, pushes multi-architecture container
   images, and attaches artifacts to the GitHub release.

Never include database passwords, backup archives, production configuration, or
session secrets in release artifacts.
