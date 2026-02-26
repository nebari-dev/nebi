---
sidebar_label: "Conda-Forge Releases"
---

# Conda-Forge Release Process

Nebi publishes two packages to conda-forge:

- **`nebi`** — the CLI (pure Go, `go-nocgo`)
- **`nebi-desktop`** — the Wails desktop app (`go-cgo` + GTK3 + WebKit2GTK on Linux)

## How releases work

Releases to conda-forge are **fully automated** after initial setup:

1. Tag a new release (e.g., `git tag v0.8`) and push it
2. GoReleaser creates a GitHub release with source tarball
3. The conda-forge bot (`regro-cf-autotick-bot`) detects the new release
4. Bot opens a PR to each feedstock (`nebi-feedstock`, `nebi-desktop-feedstock`) with the updated version and SHA256
5. CI builds and tests the package on Linux, macOS, and Windows
6. With `bot: automerge: 'version'` enabled, the PR auto-merges when CI passes
7. Package is available on conda-forge within a few hours

**No manual intervention is needed for version bumps.**

## Feedstock repos

Once the staged-recipes PR is merged, conda-forge creates:

- [`conda-forge/nebi-feedstock`](https://github.com/conda-forge/nebi-feedstock)
- [`conda-forge/nebi-desktop-feedstock`](https://github.com/conda-forge/nebi-desktop-feedstock)

Maintainers listed in the recipe get commit access to these repos.

## Recipe structure

Recipes live in `recipes/` in this repo for reference, but the canonical versions are in the feedstock repos. The recipes use the **v1 format** (`recipe.yaml`) with `rattler-build`.

### nebi (CLI)

- **Compiler**: `go-nocgo` (pure Go, no CGO)
- **Build**: installs npm deps → builds React frontend → embeds in Go binary via `//go:embed` → `go build ./cmd/nebi`
- **License**: `go-licenses` collects all transitive Go dependency licenses
- **Platforms**: linux-64, linux-aarch64, osx-64, osx-arm64, win-64

### nebi-desktop (Desktop App)

- **Compiler**: `go-cgo` + C/C++ compilers
- **Host deps (Linux)**: `gtk3`, `webkit2gtk4.1`, `glib`, `libsoup`, plus transitive deps (`gdk-pixbuf`, `zlib`, `expat`, `fontconfig`) needed for pkg-config resolution at compile time
- **Build**: installs npm deps → builds frontend → `wails build` with `-tags webkit2_41` on Linux
- **Platforms**: linux-64, osx-64, osx-arm64, win-64

## Testing recipes locally

Install `rattler-build`:

```bash
pixi global install rattler-build
```

Build and test a recipe:

```bash
rattler-build build --recipe recipes/nebi/recipe.yaml
rattler-build build --recipe recipes/nebi-desktop/recipe.yaml
```

The desktop recipe needs a `conda_build_config.yaml` for local builds (not needed on conda-forge CI):

```yaml
# recipes/nebi-desktop/conda_build_config.yaml (local builds only)
c_stdlib:
  - sysroot
c_stdlib_version:
  - "2.17"
```

Install the built package locally:

```bash
pixi global install --channel ./output --channel conda-forge nebi
pixi global install --channel ./output --channel conda-forge nebi-desktop
```

## Updating recipes

For most changes (dependency updates, build fixes), edit the recipe in the feedstock repo directly and open a PR. The feedstock CI will test the changes.

For version bumps, just tag a new release — the bot handles it automatically.
