# Release Process

This document describes how `iguazu-engine` is versioned, released and verified, as
required by the [LF Decentralized Trust release signing policy](https://lf-decentralized-trust.github.io/governance/governing-documents/release-signing/).

## Versioning

`iguazu-engine` is a Go module. Releases are [Semantic Versioning](https://semver.org/)
tags of the form `vMAJOR.MINOR.PATCH` on the `main` branch. While the major version
is `0`, a minor release may contain breaking API changes; they are listed in the
release notes. Versions that must not be used are retracted in `go.mod`.

## Making a release

1. Make sure CI passes on the commit to release.
2. Create and push an annotated, signed-off tag:

   ```shell
   git tag -a vX.Y.Z -m "iguazu-engine vX.Y.Z"
   git push origin vX.Y.Z
   ```

3. The [Release workflow](.github/workflows/release.yml) then, on a GitHub-hosted runner:
   - builds the source archive `iguazu-engine-X.Y.Z.tar.gz` from the tag and its `SHA256SUMS`;
   - generates [SLSA](https://slsa.dev/) build provenance for both files, signed with
     [Sigstore](https://www.sigstore.dev/) through
     [GitHub artifact attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations);
   - publishes a GitHub Release with the files and generated release notes.

   Releases are immutable once published: the tag and the attached files cannot be
   changed. To publish a release for an existing tag, run the workflow manually with
   the tag as input.

The repository does not ship binaries; Go users consume the module source.

## Verifying a release

Verify the provenance of the downloaded files with the
[GitHub CLI](https://cli.github.com/):

```shell
gh attestation verify iguazu-engine-X.Y.Z.tar.gz --repo LFDT-Iguazu/iguazu-engine
gh attestation verify SHA256SUMS --repo LFDT-Iguazu/iguazu-engine
sha256sum -c SHA256SUMS
```

When the module is used through `go get`, the Go toolchain verifies its content
against the public [Go checksum database](https://sum.golang.org/) automatically;
`go mod verify` re-checks the local module cache.

## Supply-chain security

- Releases are built on GitHub-hosted runners from the tagged source and carry signed
  provenance, which meets SLSA Build Level 2 (LFDT incubation projects must meet at
  least Level 1).
- GitHub Actions are pinned by commit SHA and kept up to date by Dependabot.
- The repository is analyzed by [OpenSSF Scorecard](https://scorecard.dev/viewer/?uri=github.com/LFDT-Iguazu/iguazu-engine);
  the result is shown as a badge in the README.
