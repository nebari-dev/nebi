// Owner: the newversion unit. Unit-specific frontend types; the API contract types
// live in types/contracts.ts (frozen).
import type { ManifestForm } from '@/types/contracts';

/**
 * GET /client/projects/:id/versions/:version/draft (version may be
 * "latest"): the starting point of a new version (Go: local.VersionDraft).
 */
export interface VersionDraft {
  project_id: string;
  project_name: string;
  base_version: number;
  latest_version: number;
  /** Local versions newer than the base, ascending. */
  newer_versions: number[];
  pixi_toml: string;
  form: ManifestForm;
  /** Tables and keys the GUI form cannot show (e.g. "[tasks]"). */
  unrepresentable: string[];
  /** The project's tags ("latest" first), for suggestions. */
  existing_tags: string[];
}

/**
 * POST /client/projects/:id/versions/preview (and the manifest part of
 * POST /versions): pixi_toml alone, or a form applied onto pixi_toml when
 * given, else onto the base version's pixi.toml.
 */
export interface PreviewChangesRequest {
  base_version: number;
  pixi_toml?: string;
  form?: ManifestForm;
}
