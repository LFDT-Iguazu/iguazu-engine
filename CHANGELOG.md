# Changelog

Release notes for every version are published with its
[GitHub Release](https://github.com/LFDT-Iguazu/iguazu-engine/releases).

## Unreleased

### Changed

- Reimplemented the `validator/shuffle` (swap-or-not shuffle) and `dag/slotticker` packages. The validator permutation is unchanged. `SlotTicker.Done` may now be called more than once.

### Removed

- Unused APIs: `slotticker.NewSlotTickerWithOffset`, `slotticker.Ticker`, `slotticker.Since`, `slotticker.Until`, `slotticker.Now`, `shuffle.CustomSHA256Hasher`.

### Deprecated

- `v0.1.0` is retracted; use `v0.2.0` or later.

### Added

- GitHub Actions: CI (build, vet, test), OpenSSF Scorecard and signed releases with SLSA provenance (see [RELEASE.md](RELEASE.md)); Dependabot.
- Repository files required by LF Decentralized Trust: `RELEASE.md`, `NOTICE`, `ADOPTERS.md`, issue and pull request templates.

## v0.1.0

- Initial release of `iguazu-engine` in the LF Decentralized Trust Iguazu project.
