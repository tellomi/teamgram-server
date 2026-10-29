# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added

- Vendored `proto/` fork of teamgram/proto at API layer 232, with a Docker Compose local deployment guide (`docs/local-docker.md`).
- Configurable dev SMS code (`Code.FixedCode`, default `12345`) for the `none` provider.

### Changed

- API layer is 232 so `invokeWithLayer` 232 no longer encodes constructor 0. `userFull`, `channel`, `channelFull`, `inputReplyToMessage`, `messageReplyHeader`, `wallet.nftItem`, and `payments.getStarGiftWithdrawalUrl` use the layer 232 constructors.

### Fixed

### Security

---

See [specs/release-and-changelog.md](specs/release-and-changelog.md) for version and release policy.
