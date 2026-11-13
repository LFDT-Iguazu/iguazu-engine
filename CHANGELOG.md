# Changelog

Release notes for every version are published with its
[GitHub Release](https://github.com/LFDT-Iguazu/iguazu-engine/releases).

## v0.2.0

### Changed

- Requires Go 1.26 (`go 1.26.0`, toolchain `go1.26.11`); modules depending on `iguazu-engine` must use Go 1.26 or later.
- Depends on `iguazu-types` v0.4.0 instead of the retracted v0.1.0.
- Dependencies updated: `github.com/holiman/uint256` v1.3.2, `github.com/minio/sha256-simd` v1.0.1, `golang.org/x/crypto` v0.59.0, `golang.org/x/sys` v0.50.0.
- Reimplemented the `validator/shuffle` (swap-or-not shuffle) and `dag/slotticker` packages. The validator permutation is unchanged. `SlotTicker.Done` may now be called more than once.
- `slotticker.NewSlotTicker` now takes `*types.SlotInfo` and follows the slot duration fork. `Dag.HandleSyncSlotInfo` accepts the base and the forked slot duration from the coordinator and completes its settings with the local fork settings. With the fork disabled, timing is unchanged.

### Fixed

- The validator processor applies all ValSyncProc fork rules to validator sync operations: the operation from the transaction is compared with the data saved from the coordinator, including the stake amount, which is required for withdrawals after the fork; operations with an unsupported version are rejected; the activation and exit epochs and the balance are taken from the saved coordinator data; withdrawals are split between the stake and the rewards by the stake amount.
- Forward finalization no longer stalls on a spine whose finalization chain includes blocks of a parallel branch with slots after the previous spine. The chain is checked to consist of the spine ancestors in the canonical order instead of comparing slots.
- `Dag.HandleFinalize` in main sync mode skips fetching from peers when all blocks of the spines are already loaded. Peers may not serve such blocks, and every finalization step waited for them. If the check fails, the main sync runs as before.
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
