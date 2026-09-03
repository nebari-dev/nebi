---
title: "Conda-Forge Release Process"
sidebar:
  label: "Conda-Forge Releases"
---

The split Nebi release is packaged as four conda-forge packages after the feedstocks are set up:

- **`nebi-cli`** — the CLI package; it builds `./cmd/nebi-cli` and installs the `nebi` executable (pure Go, `go-nocgo`)
- **`nebi-server`** — the team server (pure Go, `go-nocgo`)
- **`nebi-web`** — the local web app (pure Go, `go-nocgo`)
- **`nebi-desktop`** — the Wails desktop app (`go-cgo` + GTK3 + WebKit2GTK on Linux)

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

Once the staged-recipes PRs are merged, conda-forge creates:

- `nebi-cli-feedstock`
- `nebi-server-feedstock`
- `nebi-web-feedstock`
- [`conda-forge/nebi-desktop-feedstock`](https://github.com/conda-forge/nebi-desktop-feedstock)

Maintainers listed in the recipe get commit access to these repos.

## Recipe structure

Recipes are maintained in the feedstock repos (linked above). They use the **v1 format** (`recipe.yaml`) with `rattler-build`.

### nebi-cli, nebi-server, and nebi-web

- **Compiler**: `go-nocgo` (pure Go, no CGO)
- **Build**: installs npm deps → builds React frontend → embeds in Go binaries via `//go:embed` → `go build -o nebi ./cmd/nebi-cli`, `go build ./cmd/nebi-server`, and `go build ./cmd/nebi-web`
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
gh repo clone conda-forge/nebi-cli-feedstock
rattler-build build --recipe nebi-cli-feedstock/recipe/recipe.yaml
```

Repeat with `nebi-server-feedstock`, `nebi-web-feedstock`, or `nebi-desktop-feedstock` when testing those packages.

The desktop recipe needs a `conda_build_config.yaml` for local builds (not needed on conda-forge CI):

```yaml
# conda_build_config.yaml (local builds only)
c_stdlib:
  - sysroot
c_stdlib_version:
  - "2.17"
```

Install the locally-built packages you want to verify:

```bash
pixi global install --channel ./output --channel conda-forge nebi-cli nebi-server nebi-web nebi-desktop
```

## Updating recipes

For most changes (dependency updates, build fixes), edit the recipe in the feedstock repo directly and open a PR. The feedstock CI will test the changes.

For version bumps, just tag a new release — the bot handles it automatically.
