// Owner: U8 (jobs). Jobs page (prototype screens Jobs, JobsProjectFilter,
// JobsRowMenu): every background job on this machine, with search,
// Status / Job type / Project filters (kept in the URL), sort by Started,
// Refresh, and per-row View project / Cancel job.
import { Button } from '@nebi/ui/components/ui/button';
import { Input } from '@nebi/ui/components/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@nebi/ui/components/ui/table';
import { cn } from '@nebi/ui/lib/utils';
import { ArrowDown, ArrowUp, RefreshCw, Search } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ALL, FilterMenu } from '@/components/jobs2/FilterMenu';
import {
  EMPTY_VALUE,
  formatRelative,
  jobDuration,
  jobStartedAt,
  jobTitle,
} from '@/components/jobs2/format';
import { JobRowMenu } from '@/components/jobs2/JobRowMenu';
import { JobStatusBadge } from '@/components/jobs2/JobStatusBadge';
import { useProjectIndex } from '@/components/jobs2/useProjectIndex';
import { useCancelJob, useJobs, useNow } from '@/hooks/client/jobs';
import type { JobsPageFilters } from '@/types/client/jobs';
import {
  JOB_STATUS_LABELS,
  JOB_TYPE_LABELS,
  type JobListFilter,
  type JobStatus,
  type JobSummary,
  type JobType,
  VERSION_MODEL_JOB_TYPES,
} from '@/types/contracts';

/** Minimum spin of the Refresh icon, so a fast refresh is still visible. */
const REFRESH_SPIN_MS = 700;
/** Jobs page list size (newest first). */
const LIST_LIMIT = 500;

const STATUS_OPTIONS = (Object.keys(JOB_STATUS_LABELS) as JobStatus[]).map(
  (value) => ({ value, label: JOB_STATUS_LABELS[value] }),
);

function readFilters(params: URLSearchParams): JobsPageFilters {
  const status = params.get('status');
  const type = params.get('type');
  return {
    status: status && status in JOB_STATUS_LABELS ? (status as JobStatus) : ALL,
    type: type && type in JOB_TYPE_LABELS ? (type as JobType) : ALL,
    project: params.get('project') || ALL,
    newestFirst: params.get('sort') !== 'oldest',
  };
}

export const JobsPage = () => {
  const navigate = useNavigate();
  const now = useNow();
  const [params, setParams] = useSearchParams();
  const filters = readFilters(params);
  const [search, setSearch] = useState('');
  const [spinning, setSpinning] = useState(false);

  const query: JobListFilter = {
    status: filters.status === ALL ? undefined : [filters.status],
    type: filters.type === ALL ? undefined : [filters.type],
    project_id: filters.project === ALL ? undefined : filters.project,
    sort: filters.newestFirst ? '-started_at' : 'started_at',
    limit: LIST_LIMIT,
  };
  const jobs = useJobs(query, { idlePollMs: 10_000 });
  const projects = useProjectIndex();
  const cancel = useCancelJob();

  const setFilter = (
    key: 'status' | 'type' | 'project' | 'sort',
    value: string,
  ) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value === ALL || value === '') next.delete(key);
        else next.set(key, value);
        return next;
      },
      { replace: true },
    );

  const clearFilters = () => {
    setSearch('');
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const k of ['status', 'type', 'project']) next.delete(k);
        return next;
      },
      { replace: true },
    );
  };

  const projectName = (job: JobSummary) =>
    job.project_name ||
    projects.projects.find((p) => p.id === job.project_id)?.name ||
    job.project_id;

  const typeOptions = useMemo(() => {
    const types = [...VERSION_MODEL_JOB_TYPES];
    if (filters.type !== ALL && !types.includes(filters.type)) {
      types.push(filters.type);
    }
    return types.map((value) => ({ value, label: JOB_TYPE_LABELS[value] }));
  }, [filters.type]);

  const projectOptions = useMemo(
    () =>
      [...projects.projects]
        .sort((a, b) => a.name.localeCompare(b.name))
        .map((p) => ({ value: p.id, label: p.name })),
    [projects.projects],
  );

  const q = search.trim().toLowerCase();
  const rows = (jobs.data ?? []).filter(
    (job) =>
      !q ||
      jobTitle(job).toLowerCase().includes(q) ||
      projectName(job).toLowerCase().includes(q),
  );
  const filtered =
    q !== '' ||
    filters.status !== ALL ||
    filters.type !== ALL ||
    filters.project !== ALL;

  const refresh = () => {
    setSpinning(true);
    window.setTimeout(() => setSpinning(false), REFRESH_SPIN_MS);
    void jobs.refetch();
  };

  const emptyMessage = () => {
    if (jobs.isPending) return 'Loading jobs…';
    if (jobs.isError) {
      return (
        <>
          Couldn’t load jobs.{' '}
          <Button
            variant="link"
            size="xs"
            className="px-0"
            onClick={() => void jobs.refetch()}
          >
            Try again
          </Button>
        </>
      );
    }
    if (filtered) {
      return (
        <>
          No jobs match these filters.{' '}
          <Button
            variant="link"
            size="xs"
            className="px-0"
            onClick={clearFilters}
          >
            Clear filters
          </Button>
        </>
      );
    }
    return 'No jobs have run on this machine yet.';
  };

  return (
    <main className="flex flex-col gap-5 px-12 py-12">
      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-2">
          <h1 className="font-bold text-3xl text-foreground">Jobs</h1>
          <p className="text-base text-muted-foreground">
            Background tasks running on this machine
          </p>
        </div>
        <Button variant="outline" onClick={refresh} disabled={spinning}>
          <RefreshCw className={cn(spinning && 'motion-safe:animate-spin')} />
          Refresh
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-80 max-w-full">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 z-10 size-[18px] -translate-y-1/2 text-muted-foreground"
            aria-hidden
          />
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search jobs"
            aria-label="Search jobs"
            className="pl-9"
          />
        </div>
        <FilterMenu
          label="Status"
          value={filters.status}
          options={STATUS_OPTIONS}
          onChange={(v) => setFilter('status', v)}
        />
        <FilterMenu
          label="Job type"
          value={filters.type}
          options={typeOptions}
          onChange={(v) => setFilter('type', v)}
        />
        <FilterMenu
          label="Project"
          value={filters.project}
          options={projectOptions}
          onChange={(v) => setFilter('project', v)}
          searchable
        />
      </div>

      <Table aria-label="Jobs">
        <TableHeader>
          <TableRow>
            <TableHead>Job</TableHead>
            <TableHead className="w-[340px]">Project</TableHead>
            <TableHead className="w-[180px]">Status</TableHead>
            <TableHead
              className="w-[180px]"
              aria-sort={filters.newestFirst ? 'descending' : 'ascending'}
              onClick={() =>
                setFilter('sort', filters.newestFirst ? 'oldest' : '')
              }
            >
              <span className="flex-1">Started</span>
              {filters.newestFirst ? (
                <ArrowDown className="size-3.5" aria-hidden />
              ) : (
                <ArrowUp className="size-3.5" aria-hidden />
              )}
            </TableHead>
            <TableHead className="w-[160px]">Duration</TableHead>
            <TableHead className="w-[74px]">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((job) => {
            const name = projectName(job);
            const exists = projects.exists(job.project_id);
            const href = `/jobs/${job.id}`;
            return (
              <TableRow
                key={job.id}
                className="cursor-pointer"
                onClick={(e) => {
                  const target = e.target as HTMLElement;
                  if (
                    target.closest(
                      'a, button, [role="menuitem"], [role="menu"]',
                    ) ||
                    window.getSelection()?.toString()
                  ) {
                    return;
                  }
                  navigate(href);
                }}
              >
                <TableCell className="h-10 whitespace-nowrap py-2">
                  <Link
                    to={href}
                    className="underline-offset-4 hover:underline focus-visible:underline"
                  >
                    {jobTitle(job)}
                  </Link>
                </TableCell>
                <TableCell className="h-10 py-2">
                  {exists ? (
                    <Link
                      to={`/projects/${job.project_id}`}
                      className="underline underline-offset-4"
                    >
                      {name}
                    </Link>
                  ) : (
                    <span className="text-muted-foreground-strong">{name}</span>
                  )}
                </TableCell>
                <TableCell className="h-10 py-2">
                  <JobStatusBadge status={job.status} />
                </TableCell>
                <TableCell className="h-10 py-2">
                  <time
                    dateTime={job.started_at ?? job.created_at}
                    title={new Date(jobStartedAt(job)).toLocaleString()}
                  >
                    {formatRelative(jobStartedAt(job), now)}
                  </time>
                </TableCell>
                <TableCell className="h-10 py-2 tabular-nums">
                  {jobDuration(job, now) ?? EMPTY_VALUE}
                </TableCell>
                <TableCell className="h-10 py-1">
                  <JobRowMenu
                    job={job}
                    projectName={name}
                    projectExists={exists}
                    onCancel={(j) => cancel.mutate(j.id)}
                  />
                </TableCell>
              </TableRow>
            );
          })}
          {rows.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={6}
                className="h-24 text-center text-muted-foreground"
              >
                {emptyMessage()}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </main>
  );
};
