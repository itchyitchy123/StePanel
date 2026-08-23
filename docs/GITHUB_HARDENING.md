# GitHub repository hardening

Some security controls are account or repository settings and cannot be
enabled by a committed file. For the public StePanel repository, enable these
settings under **Settings → Code security and analysis**:

- Dependabot alerts and security updates
- Secret scanning and push protection
- Code scanning using the CodeQL workflow in this repository
- Private vulnerability reporting

Under **Settings → Branches**, protect `main` with:

- Pull requests required before merging
- At least one approving review
- Dismiss stale approvals after new commits
- Required status checks for `CI / test` and `CodeQL / Analyze Go`
- Conversations resolved before merging
- Force pushes and branch deletion disabled

For releases, verify that GitHub displays the build provenance attestation and
SBOM on the release page. Pin production deployments to a reviewed release
digest rather than `latest`.
