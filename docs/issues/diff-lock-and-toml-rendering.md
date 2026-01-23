# Issue: `nebi ws diff --lock` Reports "No Package Changes" & TOML Rendering Shows `map[...]`

## Observed Behavior

### Bug 1: Lock File Diff Inconsistency

`nebi ws diff` shows inconsistent behavior with the lock file:

- **Without `--lock`**: correctly shows `pixi.lock (changed)` indicator
- **With `--lock`**: incorrectly says `pixi.lock: no package changes`

```
$ pixi add fastapi
$ nebi ws diff
--- pulled (data-science:v1, sha256:208ad206753a...)
+++ local
@@ pixi.toml @@
 [dependencies]
+fastapi = ">=0.128.0,<0.129"

@@ pixi.lock (changed) @@
[Use --lock for full lock file details]

$ nebi ws diff --lock
--- pulled (data-science:v1, sha256:208ad206753a...)
+++ local
@@ pixi.toml @@
 [dependencies]
+fastapi = ">=0.128.0,<0.129"

  pixi.lock: no package changes
```

The lock file clearly DID change (fastapi and its transitive deps were added), but `--lock` mode reports no changes.

### Bug 2: TOML Rendering of Nested Sections

When features or other deeply-nested TOML sections are added, the diff output shows Go's internal map representation instead of proper TOML:

**Actual output:**
```
 [feature]
+test = "map[dependencies:map[pytest:*]]"
```

**Expected output:**
```
+[feature.test.dependencies]
+pytest = "*"
```

---

## Root Cause Analysis

### Bug 1: Wrong pixi.lock YAML Structure Assumption

**Location:** `internal/diff/lock.go` -- `parseLockPackages()` (lines 56-91)

The parser assumes pixi.lock has this structure:

```yaml
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
  pypi:
    - name: flask
      version: "2.3.0"
```

But **real pixi.lock files (version 6)** have a completely different format:

```yaml
version: 6
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/.../numpy-2.4.1-py314h2b28147_0.conda
      - pypi: https://files.pythonhosted.org/.../fastapi-0.128.0-py3-none-any.whl
packages:
- conda: https://conda.anaconda.org/.../numpy-2.4.1-py314h2b28147_0.conda
  sha256: abc123...
  depends:
  - python >=3.14
- pypi: https://files.pythonhosted.org/.../fastapi-0.128.0-py3-none-any.whl
  name: fastapi
  version: 0.128.0
```

Key structural differences:

| Aspect | Parser expects | Real v6 format |
|--------|---------------|----------------|
| `packages` layout | Nested map: `packages.conda[]`, `packages.pypi[]` | Flat list of entries |
| Conda package identity | `name` and `version` fields | URL-only (e.g., `conda: https://.../numpy-2.4.1-py314h...conda`); **no** `name`/`version` fields |
| PyPI package identity | `name` and `version` fields | Has `name` and `version` fields |
| Platform grouping | Not handled | Referenced per-platform in `environments.*.packages.<platform>[]` |

#### Execution Trace

1. `cmd/nebi/diff.go:152` -- `bytesEqual()` correctly detects lock bytes differ, calls `CompareLock()`
2. `lock.go:62` -- `yaml.Unmarshal(content, &lf)` succeeds (valid YAML), but `lf.Packages.Conda` and `lf.Packages.Pypi` are **empty** because the real format doesn't have those sub-keys
3. `lock.go:86` -- `len(packages) == 0`, falls through to `parseFlatLockPackages(content)`
4. `parseFlatLockPackages` tries parsing `packages` as `[]struct{ Name, Version }` -- but for conda entries there are no `name`/`version` fields, so they're all empty strings and get skipped by the `if pkg.Name != ""` check
5. Returns empty map, `diffPackages(emptyMap, emptyMap)` produces `LockSummary` with all zeros
6. `cmd/nebi/diff.go:325` -- `diffLock && lockSummary != nil` calls `FormatLockDiffText(lockSummary)`
7. `lock.go:181` -- `total == 0` outputs `"pixi.lock: no package changes"`

#### Why It Works Without `--lock`

The non-`--lock` path (line 324) just checks `bytesEqual(sourceLock, targetLock)` which correctly returns `false`, so it shows `"@@ pixi.lock (changed) @@"`. It never relies on the parsed summary for change detection -- only for the optional count display.

---

### Bug 2: `addChangesForValue` Only Recurses One Level

**Location:** `internal/diff/toml.go` -- `addChangesForValue()` (lines 157-187) and `formatValue()` (lines 190-215)

When a new deeply-nested section like `[feature.test.dependencies]` is added:

1. `compareMaps` sees `"feature"` is a new top-level key (not in old TOML)
2. Calls `addChangesForValue(section="feature", key="feature", val=map[test:map[dependencies:map[pytest:*]]], ChangeAdded, diff)`
3. `addChangesForValue` sees `val` is a map, iterates its keys
4. For key `"test"`, the value is `map[dependencies:map[pytest:*]]` -- **still a nested map**
5. But `addChangesForValue` doesn't recurse further. It creates `Change{Key: "test", NewValue: formatValue(map[...])}`
6. `formatValue` for `map[string]interface{}` falls through to: `fmt.Sprintf("%v", v)` producing `"map[dependencies:map[pytest:*]]"`

The section rendering in `FormatUnifiedDiff` then shows:
```
 [feature]
+test = "map[dependencies:map[pytest:*]]"
```

---

## Proposed Fixes

### Fix for Bug 1: Rewrite Lock File Parser for Real pixi.lock v6 Format

The `parseLockPackages` function needs to understand the real format:

1. **Parse the top-level `packages` as a flat list** where each entry is a map with either a `conda:` or `pypi:` key
2. **For conda entries**: extract name and version from the URL filename
   - URL pattern: `.../numpy-2.4.1-py314h2b28147_0.conda` or `.../numpy-2.4.1-py314h2b28147_0.tar.bz2`
   - Parse: split filename, extract `name` and `version` (conda filename convention is `name-version-build.ext`)
3. **For PyPI entries**: use the explicit `name` and `version` fields (these exist in the real format)
4. **Add version-awareness**: check the top-level `version` field and dispatch to the appropriate parser

Suggested struct for real format:

```go
type lockFileV6 struct {
    Version  int                   `yaml:"version"`
    Packages []lockFileV6Entry     `yaml:"packages"`
}

// Each entry has EITHER a "conda" or "pypi" key as the URL,
// plus metadata fields. Use map-based parsing since keys are dynamic.
```

Since entries have dynamic keys (`conda: <url>` or `pypi: <url>`), the most robust approach is to unmarshal each package entry as `map[string]interface{}` and inspect the keys.

### Fix for Bug 2: Recursive TOML Section Rendering

Three changes needed:

**A. Make `addChangesForValue` recurse into nested maps:**

When a value is a `map[string]interface{}` and any of its values are also maps, recurse deeper, building up the full section path. Only emit leaf key-value pairs as `Change` entries.

**B. Fix `formatValue` safety net:**

Change the `map[string]interface{}` case in `formatValue` from `fmt.Sprintf("%v", v)` to produce TOML-friendly inline table syntax (e.g., `{key = "val", ...}`) as a fallback. Ideally part A means maps never reach `formatValue`, but this prevents ugly output if they do.

**C. Update section paths in `Change` entries:**

The `Section` field should contain the full dotted path (e.g., `feature.test.dependencies`) so that `FormatUnifiedDiff` can render proper TOML section headers like `[feature.test.dependencies]`.

---

## Test Gaps

The existing tests in `lock_test.go` all use the invented `packages.conda[]`/`packages.pypi[]` format, not real pixi.lock content. New tests should use actual pixi.lock v6 content with:
- Conda packages (URL-only identity)
- PyPI packages (explicit name/version)
- Multiple platforms in the environments section
- Mixed additions, removals, and version changes

The TOML tests in `toml_test.go` don't cover deeply-nested sections (3+ levels). Add tests for:
- `[feature.test.dependencies]` additions/removals
- `[feature.cuda.system-requirements]` changes
- Nested map values in general

---

## Files Involved

| File | Role |
|------|------|
| `internal/diff/lock.go` | Lock file YAML parser and package differ |
| `internal/diff/lock_test.go` | Lock diff tests (need real format examples) |
| `internal/diff/toml.go` | TOML semantic diff engine |
| `internal/diff/toml_test.go` | TOML diff tests (need nested section cases) |
| `cmd/nebi/diff.go` | CLI command wiring, output formatting |
| `docs/design/diff-workflow.md` | Design spec (Lock File Package Diff section) |

## Impact

- **Bug 1**: The `--lock` flag is completely non-functional for real pixi.lock files. Users see "no package changes" even when packages were clearly added/removed.
- **Bug 2**: Any workspace using pixi features (a common pattern) produces unreadable diff output with Go internal map representations.
