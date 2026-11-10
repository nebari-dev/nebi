// Owner: U4 (compare). Compare versions (/projects/:id/compare?base=&to=).
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from '@nebi/ui/components/ui/alert';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@nebi/ui/components/ui/breadcrumb';
import { Button } from '@nebi/ui/components/ui/button';
import { Card } from '@nebi/ui/components/ui/card';
import { Spinner } from '@nebi/ui/components/ui/spinner';
import { X } from 'lucide-react';
import { type ReactNode, useMemo } from 'react';
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom';
import { clientApiError, httpStatus } from '@/api/client/base';
import { CompareSummary } from '@/components/compare/CompareSummary';
import { CompareTable } from '@/components/compare/CompareTable';
import {
  buildCompareRows,
  describeListDiff,
  formatVersionLabel,
  hasSpecChanges,
  resolveSelection,
  versionMarks,
  versionNumbers,
} from '@/components/compare/compareModel';
import { VersionPicker } from '@/components/compare/VersionPicker';
import { useCompare } from '@/hooks/client/compare';
import { useVersions } from '@/hooks/client/detail';
import { useProject } from '@/hooks/client/projects';
import type { ProjectListItem } from '@/types/contracts';

const PageHeader = ({
  project,
  title,
}: {
  project: ProjectListItem | undefined;
  title: string;
}) => (
  <header className="flex flex-col gap-1.5">
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem>
          <BreadcrumbLink render={<Link to="/" />}>Projects</BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        {project && (
          <>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to={`/projects/${project.id}`} />}>
                {project.name}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
          </>
        )}
        <BreadcrumbItem>
          <BreadcrumbPage>Compare versions</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
    <h1 className="font-bold text-3xl text-foreground">{title}</h1>
    {project?.path && (
      <p className="text-muted-foreground text-sm">{project.path}</p>
    )}
  </header>
);

const CompareCard = ({
  onExit,
  children,
}: {
  onExit: () => void;
  children: ReactNode;
}) => (
  <Card className="gap-4 p-4">
    <div className="flex items-start justify-between gap-4">
      <h2 className="font-semibold text-foreground text-xl leading-7">
        Compare versions
      </h2>
      <Button variant="outline" size="sm" onClick={onExit}>
        Exit Compare
        <X />
      </Button>
    </div>
    {children}
  </Card>
);

const compareErrorText = (err: unknown) => {
  const detail = clientApiError(err)?.error;
  if (httpStatus(err) === 502) {
    return {
      title: 'Can’t reach the server',
      description:
        'A version you picked is only on the server, and the server didn’t answer. Nebi keeps trying in the background.',
      detail,
    };
  }
  return {
    title: 'Couldn’t compare these versions',
    description: detail ?? 'Something went wrong. Try again.',
    detail: undefined,
  };
};

export const ComparePage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const project = useProject(id);
  const versions = useVersions(id);

  const items = versions.data ?? [];
  const marks = useMemo(
    () => versionMarks(project.data, versions.data ?? []),
    [project.data, versions.data],
  );
  const numbers = useMemo(
    () => versionNumbers(versions.data ?? []),
    [versions.data],
  );
  const selection = resolveSelection(numbers, marks, params);
  const compare = useCompare(id, selection?.base, selection?.target);

  const rows = useMemo(
    () => buildCompareRows(compare.data?.requested ?? []),
    [compare.data],
  );

  if (project.isError && httpStatus(project.error) === 404) {
    return (
      <main className="flex flex-col gap-3 p-9">
        <h1 className="font-bold text-3xl text-foreground">
          Project not found
        </h1>
        <Link to="/" className="text-sm underline underline-offset-4">
          Back to projects
        </Link>
      </main>
    );
  }

  const fromParam = selection?.from ?? params.get('from');
  const exit = () =>
    navigate(`/projects/${id}${fromParam ? `?v=${fromParam}` : ''}`);
  const format = (n: number) => formatVersionLabel(n, marks);
  const choose = (side: 'base' | 'to', n: number) => {
    if (!selection) return;
    const next = new URLSearchParams(params);
    next.set('base', String(side === 'base' ? n : selection.base));
    next.set('to', String(side === 'to' ? n : selection.target));
    setParams(next, { replace: true });
  };

  let body: ReactNode;
  if (project.isPending || versions.isPending) {
    body = (
      <div className="flex items-center gap-2 py-6 text-muted-foreground text-sm">
        <Spinner />
        Loading versions…
      </div>
    );
  } else if (project.isError || versions.isError) {
    body = (
      <Alert variant="destructive">
        <AlertTitle>Couldn’t load this project’s versions</AlertTitle>
        <AlertDescription>
          {clientApiError(project.error ?? versions.error)?.error ??
            'Something went wrong. Try again.'}
        </AlertDescription>
      </Alert>
    );
  } else if (!selection || items.length === 0) {
    body = (
      <p className="py-6 text-center text-muted-foreground text-sm">
        This project has no versions to compare yet.
      </p>
    );
  } else {
    const baseLabel = format(selection.base);
    const targetLabel = format(selection.target);
    const result = compare.data;
    const fresh =
      result?.base.number === selection.base &&
      result?.target.number === selection.target;
    const channels = result ? describeListDiff(result.channels) : '';
    const platforms = result ? describeListDiff(result.platforms) : '';
    let table: ReactNode;
    if (compare.isError) {
      const { title, description, detail } = compareErrorText(compare.error);
      table = (
        <Alert variant="destructive">
          <AlertTitle>{title}</AlertTitle>
          <AlertDescription className="flex flex-col items-start gap-2">
            <span>{description}</span>
            {detail && <span className="font-mono text-xs">{detail}</span>}
            <Button
              variant="secondary"
              size="xs"
              loading={compare.isFetching}
              onClick={() => compare.refetch()}
            >
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      );
    } else if (!result) {
      table = (
        <div className="flex items-center gap-2 py-6 text-muted-foreground text-sm">
          <Spinner />
          Comparing {baseLabel} and {targetLabel}…
        </div>
      );
    } else {
      table = (
        <>
          {fresh && (channels || platforms) && (
            <ul className="flex flex-col gap-1 text-muted-foreground text-sm">
              {channels && <li>Channels: {channels}</li>}
              {platforms && <li>Platforms: {platforms}</li>}
            </ul>
          )}
          <CompareTable
            rows={rows}
            baseLabel={fresh ? baseLabel : format(result.base.number)}
            targetLabel={fresh ? targetLabel : format(result.target.number)}
            busy={!fresh}
          />
        </>
      );
    }
    body = (
      <>
        <div className="flex flex-wrap items-center gap-3 rounded-md bg-muted p-2">
          <VersionPicker
            label="Base"
            value={selection.base}
            versions={numbers}
            format={format}
            onChange={(n) => choose('base', n)}
          />
          <VersionPicker
            label="Compare to"
            value={selection.target}
            versions={numbers}
            format={format}
            onChange={(n) => choose('to', n)}
          />
          {result && fresh && (
            <CompareSummary
              counts={result.requested_counts}
              specChanged={hasSpecChanges(result)}
            />
          )}
        </div>
        {table}
      </>
    );
  }

  return (
    <main className="flex flex-col gap-6 px-9 py-9">
      <PageHeader
        project={project.data}
        title={project.data?.name ?? 'Compare versions'}
      />
      <CompareCard onExit={exit}>{body}</CompareCard>
    </main>
  );
};
