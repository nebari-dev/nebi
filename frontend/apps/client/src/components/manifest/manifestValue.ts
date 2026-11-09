// Owner: U1 (projects). Value model of the ManifestEditor (cross-unit
// contract, CONTRACTS.md).
import type { ManifestForm } from '@/types/contracts';

export type ManifestEditorMode = 'form' | 'toml';

export interface ManifestEditorValue {
  mode: ManifestEditorMode;
  /** Form view (authoritative in "form" mode). */
  form: ManifestForm;
  /** pixi.toml text (authoritative in "toml" mode). */
  pixiToml: string;
}

export const emptyManifestValue = (name = ''): ManifestEditorValue => ({
  mode: 'form',
  form: {
    name,
    channels: ['conda-forge'],
    platforms: [],
    requested: [],
  },
  pixiToml: '',
});
