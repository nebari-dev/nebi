import { BookOpen, Check, ChevronDown, Code, Copy, X } from 'lucide-react';
import { useEffect, useId, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { openExternal } from '@/lib/openExternal';

const PULL_DOCS_URL = 'https://nebi.nebari.dev/docs/cli-team#pull';
const INSTALL_DOCS_URL = 'https://nebi.nebari.dev/docs/installation';

interface UseLocallyButtonProps {
  workspaceName: string;
}

export const UseLocallyButton = ({ workspaceName }: UseLocallyButtonProps) => {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const panelId = useId();
  const titleId = useId();
  const command = `nebi login ${window.location.origin} && nebi pull ${workspaceName}`;

  useEffect(() => {
    if (!open) return;

    const handlePointerDown = (event: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(event.target as Node)
      ) {
        setOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
      }
    };

    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [open]);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(command);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div ref={containerRef} className="relative">
      <Button
        variant="outline"
        size="sm"
        className="gap-2"
        onClick={() => setOpen((current) => !current)}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={panelId}
      >
        <Code className="h-4 w-4" />
        Use locally
        <ChevronDown
          className={`h-4 w-4 transition-transform ${open ? 'rotate-180' : ''}`}
        />
      </Button>

      {open && (
        <div
          id={panelId}
          role="dialog"
          aria-labelledby={titleId}
          className="absolute right-0 top-full z-50 mt-2 w-[360px] rounded-md border border-input bg-popover text-popover-foreground shadow-xl"
        >
          <div className="space-y-3 p-4 pr-10">
            <h2 id={titleId} className="text-sm font-semibold">
              Use this workspace locally
            </h2>
            <button
              type="button"
              className="absolute right-3 top-3 rounded-sm p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => setOpen(false)}
              aria-label="Close use locally instructions"
            >
              <X className="h-4 w-4" />
            </button>
            <p className="text-sm leading-6 text-muted-foreground">
              Run this in your terminal to download the workspace spec to your
              machine. Sync again whenever a teammate pushes changes.
            </p>
            <div className="flex items-start gap-2 rounded-md border border-input bg-muted/60 p-1.5">
              <pre className="min-w-0 flex-1 whitespace-pre-wrap break-words px-2 py-1.5 font-mono text-sm leading-6">
                <code>
                  <span className="text-muted-foreground">$ </span>
                  {command}
                </code>
              </pre>
              <Button
                variant="outline"
                size="icon"
                className="h-8 w-8 shrink-0 bg-background"
                onClick={handleCopy}
                aria-label={
                  copied ? 'Copied nebi pull command' : 'Copy nebi pull command'
                }
              >
                {copied ? (
                  <Check className="h-4 w-4" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </Button>
            </div>
          </div>

          <div className="flex items-center justify-between gap-3 border-t border-input px-4 py-3 text-xs">
            <button
              type="button"
              className="inline-flex items-center gap-1 text-primary transition-colors hover:underline"
              onClick={() => {
                openExternal(PULL_DOCS_URL);
                setOpen(false);
              }}
            >
              <BookOpen className="h-3.5 w-3.5" />
              nebi pull docs
            </button>
            <div className="flex items-center gap-1">
              <span className="text-muted-foreground">No nebi CLI?</span>
              <button
                type="button"
                className="text-primary transition-colors hover:underline"
                onClick={() => {
                  openExternal(INSTALL_DOCS_URL);
                  setOpen(false);
                }}
              >
                Install it →
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
