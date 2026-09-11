# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
## [Unreleased]

### Breaking Changes
- *(router)* Default to PathLocation instead of HashLocation (#27)


### Added
- Add end-to-end acceptance suite for the fullstack example app (#26)
- Add cross-platform CLI tests and update installation documentation (#28)

## [0.4.0] - 2026-09-10

### Breaking Changes
- Bump minimum Go version to 1.25 (#18)


### Added
- Add public API surface check to CI (#17)
- Enhance documentation with bundle size analysis and improve API reference accuracy (#20)
- Add tests for version command and argument reordering in CLI (#21)
- Enable mobile viewport testing in Playwright configuration (#22)


### Fixed
- Improve Untrack behavior to restore effect stack on panic and enhance Set documentation (#16)
- Repair tag-triggered CI, release, and Pages deploy pipeline (#24)

## [0.3.0] - 2026-09-08

### Breaking Changes
- Functional options for router.Route, rename Self() and LocalStorage API (#10)


### Added
- Enhance GitHub Actions workflow to support optional version input for vane-page build (#14)


### Fixed
- Update permissions in CI and release workflows; upgrade golang.org/x/crypto to v0.46.0 (#9)
- *(docs)* Add 404.html fallback for client-side routing in GitHub Pages deployment (#13)
- Detach worktree checkout to allow branch refs in pages.yml (#15)

## [0.2.0] - 2026-09-06

### Added
- *(router)* Decouple routing from URL via Location (Hash/Path) (#4)


### Fixed
- Adjust asset paths and URLs for GitHub Pages deployment (#5)
- Audit and fix memory/lifecycle disposal correctness (#6)

## [0.1.1] - 2026-09-05

### Added
- Add Playwright E2E pipeline and lifecycle coverage (#1)
- Enhance HeadConfig with additional Open Graph properties and update documentation pages (#3)

## [0.1.0] - 2026-07-18
[Unreleased]: https://github.com/FilipeJohansson/vane/compare/v0.4.0..HEAD
[0.4.0]: https://github.com/FilipeJohansson/vane/compare/v0.3.0..v0.4.0
[0.3.0]: https://github.com/FilipeJohansson/vane/compare/v0.2.0..v0.3.0
[0.2.0]: https://github.com/FilipeJohansson/vane/compare/v0.1.1..v0.2.0
[0.1.1]: https://github.com/FilipeJohansson/vane/compare/v0.1.0..v0.1.1
[0.1.0]: https://github.com/FilipeJohansson/vane/releases/tag/v0.1.0

