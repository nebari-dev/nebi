// Owner: U1 (projects). Value model of the ManifestEditor (cross-unit
// contract, CONTRACTS.md) plus pure helpers shared by Create project (U1)
// and New version (U3).
import type { ManifestForm, RequestedPackage } from '@/types/contracts';

export type ManifestEditorMode = 'form' | 'toml';

export interface ManifestEditorValue {
  mode: ManifestEditorMode;
  /** Form view (authoritative in "form" mode). */
  form: ManifestForm;
  /** pixi.toml text (authoritative in "toml" mode). */
  pixiToml: string;
}

export const DEFAULT_CHANNEL = 'conda-forge';

export const emptyManifestValue = (name = ''): ManifestEditorValue => ({
  mode: 'form',
  form: {
    name,
    channels: [DEFAULT_CHANNEL],
    platforms: [],
    requested: [],
  },
  pixiToml: '',
});

const tomlString = (s: string) =>
  `"${s.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
const tomlKey = (k: string) => (/^[A-Za-z0-9_-]+$/.test(k) ? k : tomlString(k));
const tomlArray = (xs: string[]) => `[${xs.map(tomlString).join(', ')}]`;

/**
 * Renders a form as a fresh pixi.toml in the same layout as the server's
 * artifact.RenderManifest. Used for the editor's first value only; mode
 * switches go through POST /client/manifest/render so tables the form
 * cannot show survive.
 */
export function formToToml(form: ManifestForm): string {
  const lines = ['[workspace]', `name = ${tomlString(form.name)}`];
  if (form.description)
    lines.push(`description = ${tomlString(form.description)}`);
  lines.push(`channels = ${tomlArray(form.channels)}`);
  lines.push(`platforms = ${tomlArray(form.platforms)}`);
  const dep = (p: RequestedPackage) =>
    `${tomlKey(p.name)} = ${tomlString(p.constraint || '*')}`;
  const conda = form.requested.filter((p) => p.source !== 'pypi');
  const pypi = form.requested.filter((p) => p.source === 'pypi');
  lines.push('', '[dependencies]', ...conda.map(dep));
  if (pypi.length) lines.push('', '[pypi-dependencies]', ...pypi.map(dep));
  return `${lines.join('\n')}\n`;
}

/**
 * Rewrites the `name = ...` line of the [workspace] (or legacy [project])
 * table, leaving every other line alone. Used to keep the pixi.toml editor
 * in step with the project name field.
 */
export function setTomlName(toml: string, name: string): string {
  let table = '';
  let done = false;
  return toml
    .split('\n')
    .map((line) => {
      const header = line.match(/^\s*\[([^\]]+)\]/);
      if (header) table = header[1].trim();
      if (
        !done &&
        (table === 'workspace' || table === 'project') &&
        /^\s*name\s*=/.test(line)
      ) {
        done = true;
        return `name = ${tomlString(name)}`;
      }
      return line;
    })
    .join('\n');
}

/** Same JSON shape (order-sensitive), for dirty checks. */
export const sameValue = (a: unknown, b: unknown) =>
  JSON.stringify(a) === JSON.stringify(b);
