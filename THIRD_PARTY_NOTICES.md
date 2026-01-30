# Third-Party Notices

The source code in this repository is licensed under the MIT License (see `LICENSE`).

Release archives produced by `make package` / `make release` may bundle third-party
software under their own licenses. Those components are NOT covered by the MIT
license of this repository.

## Bundled Components

### mihomo

- Purpose: proxy core (Clash kernel)
- Included in release archives as: `mihomo`
- Stored in this repo as: `third_party/mihomo/mihomo-<os>-<arch>` and `third_party/mihomo/*.version`
- Upstream: `MetaCubeX/mihomo` (Alpha branch)
- Version used by default in this repo: `1.19.19` (see `MIHOMO_VERSION` in `Makefile`)
- License: GPL-3.0 (per upstream)

Source code for the exact version can be obtained from the upstream release tag
`v1.19.19` (the release page includes "Source code (zip)" / "Source code (tar.gz)").

### proxychains-ng

- Purpose: optional LD_PRELOAD-based proxy wrapper used by `c4cgo` (when available)
- Included in release archives as: `proxychains4` and `libproxychains4.so`
- Stored in this repo as: `third_party/proxychains/<os>-<arch>/...`
- Upstream: `rofl0r/proxychains-ng`
- Version used by default in this repo: `4.17` (see `PROXYCHAINS_NG_VERSION` in `Makefile`)
- License: GPL-2.0-or-later

The proxychains-ng license text is included in this repo under:
`third_party/proxychains/<os>-<arch>/COPYING.proxychains-ng`.

### Go module dependencies

This project uses Go modules; their licenses are governed by their upstream projects.
See `go.mod` / `go.sum` for the full dependency list.
