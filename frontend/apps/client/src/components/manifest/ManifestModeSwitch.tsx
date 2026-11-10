// Owner: U1 (projects). Switches the ManifestEditor between the GUI form
// and the pixi.toml editor through POST /client/manifest/render|parse
// (internal/artifact round trip), with the prototype's warning and toast.
import { Button } from '@nebi/ui/components/ui/button';
import { CodeXml, LayoutTemplate } from 'lucide-react';
import type { ManifestEditorValue } from './manifestValue';
import {
  LOSE_DATA_WARNING,
  useManifestModeSwitch,
} from './useManifestModeSwitch';

export interface ManifestModeSwitchProps {
  value: ManifestEditorValue;
  onChange: (value: ManifestEditorValue) => void;
  /** Called with the converted value after a successful switch. */
  onSwitched?: (value: ManifestEditorValue) => void;
  disabled?: boolean;
}

/** "Open in TOML editor" / "Open in GUI editor" (prototype header button). */
export const ManifestModeSwitch = ({
  value,
  onChange,
  onSwitched,
  disabled,
}: ManifestModeSwitchProps) => {
  const { switchMode, pending } = useManifestModeSwitch(value, onChange);
  const inForm = value.mode === 'form';
  return (
    <div className="flex w-[300px] max-w-full shrink-0 flex-col items-end gap-0.5">
      <Button
        type="button"
        variant="outline"
        loading={pending}
        disabled={disabled}
        onClick={async () => {
          const next = await switchMode();
          if (next) onSwitched?.(next);
        }}
      >
        {inForm ? <CodeXml /> : <LayoutTemplate />}
        {inForm ? 'Open in TOML editor' : 'Open in GUI editor'}
      </Button>
      {!inForm && (
        <p className="text-right text-muted-foreground text-sm">
          {LOSE_DATA_WARNING}
        </p>
      )}
    </div>
  );
};
