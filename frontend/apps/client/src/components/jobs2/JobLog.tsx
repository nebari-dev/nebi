// Owner: U8 (jobs). Job log panel for the job page: search with a match
// count, Copy, Download (nebi-<type>-<id8>.log), highlighted matches and
// the "Waiting to start" / streaming states.
import { Button } from '@nebi/ui/components/ui/button';
import { Input } from '@nebi/ui/components/ui/input';
import { toast } from '@nebi/ui/components/ui/toast';
import { Search } from 'lucide-react';
import { Fragment, type ReactNode, useMemo, useState } from 'react';
import type { JobSummary } from '@/types/contracts';
import { logFileName } from './format';

/** Splits a log into lines, dropping the empty line after a final newline. */
function logLines(log: string): string[] {
  if (!log) return [];
  const lines = log.split('\n');
  if (lines[lines.length - 1] === '') lines.pop();
  return lines;
}

/** "No matching lines", "1 matching line", "3 matching lines". */
function matchCountLabel(n: number): string {
  if (n === 0) return 'No matching lines';
  return n === 1 ? '1 matching line' : `${n} matching lines`;
}

/** Wraps every case-insensitive occurrence of query in <mark>. */
function highlight(line: string, query: string): ReactNode {
  if (!query) return line;
  const lower = line.toLowerCase();
  const parts: ReactNode[] = [];
  let from = 0;
  for (
    let at = lower.indexOf(query);
    at !== -1;
    at = lower.indexOf(query, from)
  ) {
    parts.push(line.slice(from, at));
    parts.push(
      <mark key={at} className="rounded-sm bg-warning text-warning-foreground">
        {line.slice(at, at + query.length)}
      </mark>,
    );
    from = at + query.length;
  }
  parts.push(line.slice(from));
  return parts;
}

export interface JobLogProps {
  job: Pick<JobSummary, 'id' | 'type' | 'type_label' | 'status'>;
  log: string;
}

export const JobLog = ({ job, log }: JobLogProps) => {
  const [query, setQuery] = useState('');
  const q = query.trim().toLowerCase();
  const lines = useMemo(() => logLines(log), [log]);
  const matches = useMemo(
    () => (q ? lines.filter((l) => l.toLowerCase().includes(q)).length : 0),
    [lines, q],
  );
  const text = lines.join('\n');

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      toast.add({ title: 'Log copied', type: 'success' });
    } catch {
      toast.add({
        title: 'Couldn’t copy the log',
        description: 'Select the text in the log and copy it instead.',
        type: 'error',
      });
    }
  };

  const download = () => {
    const url = URL.createObjectURL(
      new Blob([`${text}\n`], { type: 'text/plain' }),
    );
    const a = document.createElement('a');
    a.href = url;
    a.download = logFileName(job);
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-80 max-w-full">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
            aria-hidden
          />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search logs"
            aria-label="Search logs"
            className="pl-9"
          />
        </div>
        {q && (
          <span className="text-muted-foreground text-sm" role="status">
            {matchCountLabel(matches)}
          </span>
        )}
        <span className="flex-1" />
        <Button variant="outline" onClick={copy} disabled={!lines.length}>
          Copy
        </Button>
        <Button variant="outline" onClick={download} disabled={!lines.length}>
          Download
        </Button>
      </div>
      <div className="flex flex-col gap-3 rounded-md border border-border bg-card px-4 py-3 font-mono text-card-foreground text-xs leading-4">
        {lines.length > 0 ? (
          <pre
            role="log"
            aria-label={`${job.type_label} log`}
            className="whitespace-pre-wrap break-words font-mono"
          >
            {lines.map((line, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: log lines are append-only and may repeat.
              <Fragment key={i}>
                {highlight(line, q)}
                {'\n'}
              </Fragment>
            ))}
          </pre>
        ) : (
          <p className="text-muted-foreground">
            {job.status === 'pending' || job.status === 'running'
              ? 'Waiting to start. The log appears once the job runs.'
              : 'This job wrote no log.'}
          </p>
        )}
        {job.status === 'running' && (
          <p className="flex items-center gap-2 text-muted-foreground">
            <span
              className="size-1.5 rounded-full bg-success-foreground motion-safe:animate-pulse"
              aria-hidden
            />
            Streaming: job in progress
          </p>
        )}
      </div>
    </>
  );
};
