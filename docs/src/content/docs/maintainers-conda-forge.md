---
title: "Conda-Forge Release Process"
sidebar:
  label: "Conda-Forge Releases"
---

The split-binary codebase builds four executables, but Nebi currently publishes three conda-forge package names:

- **`nebi`** — the recommended package; it depends on `nebi-cli` and `nebi-desktop`
- **`nebi-cli`** — the CLI-only package; it builds the CLI entry point and installs the `nebi` executable, not a `nebi-cli` executable (pure Go, `go-nocgo`)
- **`nebi-desktop`** — the Wails desktop app (`go-cgo` + GTK3 + WebKit2GTK on Linux)

TODO for the split-binary release: decide and publish packages for `nebi-server` and `nebi-web`.

## How releases work

Releases to conda-forge are **fully automated** after initial setup:

1. Tag a new release (e.g., `git tag v0.8`) and push it
2. GoReleaser creates a GitHub release with source tarball
3. The conda-forge bot (`regro-cf-autotick-bot`) detects the new release
4. Bot opens a PR to each feedstock with the updated version and SHA256
5. CI builds and tests the package on Linux, macOS, and Windows
6. With `bot: automerge: 'version'` enabled, the PR auto-merges when CI passes
7. Package is available on conda-forge within a few hours

**No manual intervention is needed for version bumps.**

## Feedstock repos

Current feedstock repos:

- [`conda-forge/nebi-feedstock`](https://github.com/conda-forge/nebi-feedstock)
- [`conda-forge/nebi-desktop-feedstock`](https://github.com/conda-forge/nebi-desktop-feedstock)

Maintainers listed in the recipe get commit access to these repos.

## Recipe structure

Recipes are maintained in the feedstock repos (linked above). They use the **v1 format** (`recipe.yaml`) with `rattler-build`.

### nebi and nebi-cli

- **Compiler**: `go-nocgo` (pure Go, no CGO)
- **Build**: installs npm deps → builds React frontend → embeds in Go binary via `//go:embed` → builds the CLI executable as `nebi`
- **License**: `go-licenses` collects all transitive Go dependency licenses
- **Platforms**: linux-64, linux-aarch64, osx-64, osx-arm64, win-64

### nebi-desktop (Desktop App)

- **Compiler**: `go-cgo` + C/C++ compilers
- **Host deps (Linux)**: `gtk3`, `webkit2gtk4.1`, `glib`, `libsoup`, plus transitive deps (`gdk-pixbuf`, `zlib`, `expat`, `fontconfig`) needed for pkg-config resolution at compile time
- **Build**: installs npm deps → builds frontend → `wails build` with `-tags webkit2_41` on Linux
- **Platforms**: linux-64, osx-64, osx-arm64, win-64

## Testing recipes locally

Clone the feedstock you are changing and build locally with `rattler-build`:

```bash
pixi global install rattler-build
gh repo clone conda-forge/nebi-feedstock
rattler-build build --recipe nebi-feedstock/recipe/recipe.yaml
```

Repeat with `nebi-desktop-feedstock` when testing the desktop package.

The desktop recipe needs a `conda_build_config.yaml` for local builds (not needed on conda-forge CI):

```yaml
# conda_build_config.yaml (local builds only)
c_stdlib:
  - sysroot
c_stdlib_version:
  - "2.17"
```

Install a locally-built package:

```bash
pixi global install --channel ./output --channel conda-forge nebi
```

## Updating recipes

For most changes (dependency updates, build fixes), edit the recipe in the feedstock repo directly and open a PR. The feedstock CI will test the changes.

For version bumps, just tag a new release — the bot handles it automatically.
