// Owner: U1 (projects). Shared by Create project (U1) and New version (U3).
// The props value/onChange/showName/disabled/errors are a cross-unit
// contract (CONTRACTS.md): keep them stable. Everything else is optional.
//
// Renders the prototype's environment sections as siblings of the page's
// own FormSections: "Environment" (platforms, channels) and "Packages" in
// form mode, or one "pixi.toml" section in toml mode. The mode switch
// ("Open in TOML editor" / "Open in GUI editor") is rendered above the
// sections unless the page puts <ManifestModeSwitch> in its header and
// passes hideModeSwitch.
import { Input } from '@nebi/ui/components/ui/input';
import { ChipsField } from './ChipsField';
import { FormRow, FormSection } from './FormLayout';
import { ManifestModeSwitch } from './ManifestModeSwitch';
import { CREATE_PROJECT_COPY, type ManifestEditorCopy } from './manifestCopy';
import type { ManifestEditorValue } from './manifestValue';
import { PackagesEditor } from './PackagesEditor';
import {
  looksLikePlatform,
  machinePlatform,
  PLATFORM_LABELS,
  platformDisplayName,
} from './rules';
import { TomlEditor } from './TomlEditor';

export type { ManifestEditorCopy } from './manifestCopy';

export type ManifestEditorErrors = Partial<
  Record<'name' | 'platforms' | 'channels' | 'requested' | 'manifest', string>
>;

const CHANNEL_SUGGESTIONS = ['conda-forge', 'bioconda', 'pytorch', 'nvidia'];

export interface ManifestEditorProps {
  value: ManifestEditorValue;
  onChange: (value: ManifestEditorValue) => void;
  /** Show the project name field (Create project); New version hides it. */
  showName?: boolean;
  disabled?: boolean;
  errors?: ManifestEditorErrors;
  /** The page renders <ManifestModeSwitch> itself (e.g. in its header). */
  hideModeSwitch?: boolean;
  /** Override section copy (New version: "this version", base version). */
  copy?: Partial<ManifestEditorCopy>;
  /** This machine's pixi platform ("· this machine" label). */
  machine?: string;
}

export const ManifestEditor = ({
  value,
  onChange,
  showName,
  disabled,
  errors = {},
  hideModeSwitch,
  copy,
  machine = machinePlatform(),
}: ManifestEditorProps) => {
  const text = { ...CREATE_PROJECT_COPY, ...copy };
  const form = value.form;
  const setForm = (patch: Partial<typeof form>) =>
    onChange({ ...value, form: { ...form, ...patch } });
  const platformChoices = [
    machine,
    ...Object.keys(PLATFORM_LABELS).filter((p) => p !== machine),
  ];

  return (
    <>
      {!hideModeSwitch && (
        <div className="flex justify-end pb-2">
          <ManifestModeSwitch
            value={value}
            onChange={onChange}
            disabled={disabled}
          />
        </div>
      )}
      {value.mode === 'form' ? (
        <>
          <FormSection
            id="environment"
            title="Environment"
            intro={text.environmentIntro}
          >
            {showName && (
              <FormRow label="Project name" error={errors.name}>
                <Input
                  value={form.name}
                  onChange={(e) => setForm({ name: e.target.value })}
                  placeholder="e.g., my-data-project"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={disabled}
                />
              </FormRow>
            )}
            <ChipsField
              label="Platforms"
              value={form.platforms}
              onChange={(platforms) => setForm({ platforms })}
              suggestions={platformChoices}
              itemLabel={(p) => platformDisplayName(p, machine)}
              normalize={(input) => input.trim().toLowerCase()}
              acceptCustom={looksLikePlatform}
              placeholder="Add a platform"
              emptyText="No match. Type pixi’s name for it, e.g. linux-ppc64le."
              description={text.platformsDescription}
              error={errors.platforms}
              disabled={disabled}
            />
            <ChipsField
              label="Channels"
              value={form.channels}
              onChange={(channels) => setForm({ channels })}
              suggestions={CHANNEL_SUGGESTIONS}
              placeholder="Add a channel"
              description="Where pixi looks for packages, in priority order. conda-forge is used when this is empty."
              error={errors.channels}
              disabled={disabled}
            />
          </FormSection>
          <FormSection
            id="packages"
            title="Packages"
            intro={text.packagesIntro}
          >
            <PackagesEditor
              requested={form.requested}
              onChange={(requested) => setForm({ requested })}
              emptyText={text.packagesEmptyText}
              error={errors.requested}
              disabled={disabled}
            />
          </FormSection>
        </>
      ) : (
        <FormSection id="toml" title="pixi.toml" intro={text.tomlIntro}>
          <div className="flex flex-col gap-1">
            <TomlEditor
              value={value.pixiToml}
              onChange={(pixiToml) => onChange({ ...value, pixiToml })}
              invalid={!!errors.manifest}
              describedBy={errors.manifest ? 'toml-error' : undefined}
              disabled={disabled}
            />
            {errors.manifest && (
              <p
                id="toml-error"
                className="text-destructive-foreground text-xs"
              >
                {errors.manifest}
              </p>
            )}
          </div>
        </FormSection>
      )}
    </>
  );
};
