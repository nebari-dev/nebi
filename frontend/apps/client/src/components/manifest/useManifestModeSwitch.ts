// Owner: U1 (projects). Converts the ManifestEditor value between the GUI
// form and pixi.toml through POST /client/manifest/render|parse
// (internal/artifact round trip), with the prototype's toast for what the
// form cannot show.
import { toast } from '@nebi/ui/components/ui/toast';
import { useCallback } from 'react';
import { clientApiError } from '@/api/client/base';
import { useParseManifest, useRenderManifest } from '@/hooks/client/projects';
import type { ManifestEditorValue } from './manifestValue';

export const LOSE_DATA_WARNING =
  'You may lose some data that isn’t supported in the GUI editor.';

/**
 * Returns a function that flips value.mode, converting the content: form
 * to pixi.toml (applied onto the last pixi.toml so tables the form cannot
 * show survive) or pixi.toml to form (reporting what the form cannot show).
 * Resolves to the new value, or null when the conversion failed.
 */
export function useManifestModeSwitch(
  value: ManifestEditorValue,
  onChange: (value: ManifestEditorValue) => void,
) {
  const parse = useParseManifest();
  const render = useRenderManifest();
  const switchMode =
    useCallback(async (): Promise<ManifestEditorValue | null> => {
      if (value.mode === 'form') {
        try {
          const res = await render.mutateAsync({
            form: value.form,
            base_pixi_toml: value.pixiToml.trim() ? value.pixiToml : undefined,
          });
          const next: ManifestEditorValue = {
            ...value,
            mode: 'toml',
            pixiToml: res.pixi_toml,
          };
          onChange(next);
          return next;
        } catch (err) {
          toast.add({
            type: 'error',
            title: 'Couldn’t open the TOML editor',
            description: clientApiError(err)?.error,
          });
          return null;
        }
      }
      try {
        const res = await parse.mutateAsync(value.pixiToml);
        const next: ManifestEditorValue = {
          ...value,
          mode: 'form',
          form: {
            ...res.form,
            channels: res.form.channels.length
              ? res.form.channels
              : value.form.channels,
          },
        };
        onChange(next);
        if (res.unrepresentable.length) {
          toast.add({
            type: 'info',
            title: `The form can’t show ${res.unrepresentable.join(', ')}`,
            description:
              'Those tables were left out. Switch back to the pixi.toml editor to add them again.',
          });
        }
        return next;
      } catch (err) {
        toast.add({
          type: 'error',
          title: 'Couldn’t open the GUI editor',
          description: clientApiError(err)?.error,
        });
        return null;
      }
    }, [value, onChange, parse, render]);
  return { switchMode, pending: parse.isPending || render.isPending };
}
