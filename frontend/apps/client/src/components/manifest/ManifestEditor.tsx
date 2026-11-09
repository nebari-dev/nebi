// Owner: U1 (projects). Shared by Create project (U1) and New version (U3).
// The props are a cross-unit contract (CONTRACTS.md): keep them stable.
// Foundation stub: a plain pixi.toml textarea; U1 builds the prototype's
// form + pixi.toml editor tabs with the "You may lose some data" warning.
import { Textarea } from '@nebi/ui/components/ui/textarea';
import type { ManifestEditorValue } from './manifestValue';

export type ManifestEditorErrors = Partial<
  Record<'name' | 'platforms' | 'channels' | 'requested' | 'manifest', string>
>;

export interface ManifestEditorProps {
  value: ManifestEditorValue;
  onChange: (value: ManifestEditorValue) => void;
  /** Show the project name field (Create project); New version hides it. */
  showName?: boolean;
  disabled?: boolean;
  errors?: ManifestEditorErrors;
}

export const ManifestEditor = ({
  value,
  onChange,
  disabled,
}: ManifestEditorProps) => (
  <Textarea
    aria-label="pixi.toml"
    value={value.pixiToml}
    disabled={disabled}
    onChange={(e) =>
      onChange({ ...value, mode: 'toml', pixiToml: e.target.value })
    }
  />
);
