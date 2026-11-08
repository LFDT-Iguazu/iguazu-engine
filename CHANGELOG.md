# Changelog

Release notes for every version are published with its
[GitHub Release](https://github.com/LFDT-Iguazu/iguazu-engine/releases).

## Unreleased

### Changed

- Reimplemented the `validator/shuffle` (swap-or-not shuffle) and `dag/slotticker` packages. The validator permutation is unchanged. `SlotTicker.Done` may now be called more than once.
- `slotticker.NewSlotTicker` now takes `*types.SlotInfo` and follows the slot duration fork. `Dag.HandleSyncSlotInfo` accepts the base and the forked slot duration from the coordinator and completes its settings with the local fork settings. With the fork disabled, timing is unchanged.

### Fixed

- `Dag.HandleFinalize` rejects a coordinated checkpoint whose spine block is not in the local chain instead of storing it. A stored checkpoint without its spine prevented the node from starting.

### Removed

- Unused APIs: `slotticker.NewSlotTickerWithOffset`, `slotticker.Ticker`, `slotticker.Since`, `slotticker.Until`, `slotticker.Now`, `shuffle.CustomSHA256Hasher`.

### Deprecated

- `v0.1.0` is retracted; use `v0.2.0` or later.

### Added

- GitHub Actions: CodeQL static analysis (security and quality queries).
- GitHub Actions: CI (build, vet, test), OpenSSF Scorecard and signed releases with SLSA provenance (see [RELEASE.md](RELEASE.md)); Dependabot.
- Repository files required by LF Decentralized Trust: `RELEASE.md`, `NOTICE`, `ADOPTERS.md`, issue and pull request templates.

## v0.1.0

- Initial release of `iguazu-engine` in the LF Decentralized Trust Iguazu project.
