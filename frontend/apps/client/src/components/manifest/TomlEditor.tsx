// Owner: U1 (projects). The pixi.toml editor (prototype): a header with the
// file name and a copy button, line numbers and a monospace textarea.
import { Button } from '@nebi/ui/components/ui/button';
import { cn } from '@nebi/ui/lib/utils';
import { Check, Copy } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

export interface TomlEditorProps {
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
  describedBy?: string;
  minRows?: number;
  disabled?: boolean;
}

export const TomlEditor = ({
  value,
  onChange,
  invalid,
  describedBy,
  minRows = 12,
  disabled,
}: TomlEditorProps) => {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const lines = value.split('\n').length;

  return (
    <div
      className={cn(
        'overflow-hidden rounded-md border border-border bg-muted focus-within:ring-2 focus-within:ring-ring',
        invalid &&
          'border-destructive-foreground ring-2 ring-destructive-foreground focus-within:ring-destructive-foreground',
      )}
    >
      <div className="flex items-center justify-between py-0.5 pr-0.5 pl-1.5">
        <span className="font-mono text-muted-foreground text-xs leading-4">
          pixi.toml
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={copied ? 'Copied' : 'Copy pixi.toml'}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
              window.clearTimeout(timer.current);
              timer.current = window.setTimeout(() => setCopied(false), 2000);
            } catch {
              setCopied(false);
            }
          }}
        >
          {copied ? <Check /> : <Copy />}
        </Button>
      </div>
      <div className="flex gap-2.5 border-border border-t bg-card p-1.5 font-mono text-xs leading-4">
        <div
          aria-hidden
          className="select-none text-right text-muted-foreground tabular-nums"
        >
          {Array.from({ length: lines }, (_, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: line numbers are positional
            <div key={i}>{i + 1}</div>
          ))}
        </div>
        <textarea
          aria-label="pixi.toml"
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={Math.max(lines, minRows)}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          wrap="off"
          disabled={disabled}
          className="min-w-0 flex-1 resize-none overflow-x-auto overflow-y-hidden whitespace-pre bg-transparent text-foreground outline-none"
        />
      </div>
    </div>
  );
};
