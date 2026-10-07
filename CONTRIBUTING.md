# Contributing

Thank you for your interest in contributing to Iguazu, an LF Decentralized Trust
project. All contributions are welcome: code, documentation, tests, bug reports
and reviews.

By participating you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
To report a security vulnerability, do **not** open a public issue; follow
[SECURITY.md](SECURITY.md) instead.

## Reporting issues

Use GitHub Issues to report bugs or propose features. Please include the
version (tag or commit), the steps to reproduce, the expected and the actual
behavior.

## Submitting changes

We follow the [LF Decentralized Trust GitHub contribution guide](https://lf-decentralized-trust.github.io/governance/guidelines/github-contribution-guide/):

1. Fork the repository and create a feature branch from `main`.
2. Make your change, keeping it focused on one logical topic.
3. Make sure the code builds, is formatted and the tests pass:

   ```shell
   gofmt -l .
   go vet ./...
   go test ./...
   ```

4. Commit with a sign-off (see below) and open a Pull Request against `main`.
5. Address review comments by amending your commits. Squash multiple commits
   that form one logical change into a single commit.

A Pull Request is merged after approval by at least one maintainer listed in
[MAINTAINERS.md](MAINTAINERS.md) and passing CI.

### Commit messages

Use a short summary line (we use [Conventional Commits](https://www.conventionalcommits.org/)
prefixes such as `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`),
followed by an empty line and a body explaining why the change is needed and how
it was approached.

### Developer Certificate of Origin (DCO)

All commits must be signed off to certify that you wrote the code or otherwise
have the right to submit it under the project's license, as described in the
[Developer Certificate of Origin](https://developercertificate.org/):

```shell
git commit -s
```

This adds a `Signed-off-by: Your Name <your@email>` trailer matching the commit
author. Pull Requests with unsigned commits cannot be merged.

### License headers

Source files are licensed under the [Apache License 2.0](LICENSE). New files
should start with the license header and include
`SPDX-License-Identifier: Apache-2.0`.
