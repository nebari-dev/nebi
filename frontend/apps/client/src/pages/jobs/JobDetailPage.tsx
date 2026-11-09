// Owner: U8 (jobs). Job page (prototype screens JobFailed, JobRunning):
// status, project, Started / Ended / Duration, the failed alert, Cancel
// job or View project, and the log (live while the job runs) with search,
// Copy and Download.
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
import { CircleAlert } from 'lucide-react';
import type { ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { httpStatus } from '@/api/client/base';
import {
  EMPTY_VALUE,
  formatDateTime,
  jobDuration,
  jobTitle,
} from '@/components/jobs2/format';
import { JobLog } from '@/components/jobs2/JobLog';
import { JobStatusBadge } from '@/components/jobs2/JobStatusBadge';
import { useProjectIndex } from '@/components/jobs2/useProjectIndex';
import {
  useCancelJob,
  useJob,
  useJobLogStream,
  useNow,
} from '@/hooks/client/jobs';
import { isActiveJob } from '@/types/contracts';

const Field = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex flex-col gap-0.5">
    <dt className="font-medium text-muted-foreground">{label}</dt>
    <dd className="text-foreground">{children}</dd>
  </div>
);

const NotFound = () => (
  <main className="flex flex-col items-start gap-3 px-12 py-12">
    <h1 className="font-bold text-3xl text-foreground">Job not found</h1>
    <p className="text-muted-foreground text-sm">
      It may have been cleared from this machine.
    </p>
    <Button variant="outline" render={<Link to="/jobs" />}>
      Back to Jobs
    </Button>
  </main>
);

export const JobDetailPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const now = useNow();
  const { data: job, error, isPending, refetch } = useJob(id);
  const active = !!job && isActiveJob(job);
  const stream = useJobLogStream(id, active);
  const projects = useProjectIndex();
  const cancel = useCancelJob();

  if (isPending) {
    // No heading yet: the page's h1 is the job title once it loads.
    return (
      <main className="flex flex-col gap-3 px-12 py-12" aria-busy>
        <p className="text-muted-foreground text-sm">Loading job…</p>
      </main>
    );
  }
  if (!job) {
    if (httpStatus(error) === 404 || httpStatus(error) === 400) {
      return <NotFound />;
    }
    return (
      <main className="flex flex-col items-start gap-3 px-12 py-12">
        <h1 className="font-bold text-3xl text-foreground">Job</h1>
        <p className="text-muted-foreground text-sm">Couldn’t load this job.</p>
        <Button variant="outline" onClick={() => void refetch()}>
          Try again
        </Button>
      </main>
    );
  }

  const title = jobTitle(job);
  const projectExists = projects.exists(job.project_id);
  const projectName = job.project_name || job.project_id;
  // The stream and the polled stored log are prefixes of the same log;
  // show whichever has more.
  const log =
    stream.logs !== null && stream.logs.length >= job.logs.length
      ? stream.logs
      : job.logs;

  return (
    <main className="flex flex-col gap-5 px-12 py-12">
      <div className="flex flex-col gap-3">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink render={<Link to="/jobs" />}>Jobs</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{title}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <div className="flex items-center gap-3">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <h1 className="font-bold text-3xl text-foreground">{title}</h1>
            <JobStatusBadge status={job.status} />
          </div>
          {active ? (
            <Button
              variant="outline"
              onClick={() => cancel.mutate(job.id)}
              disabled={cancel.isPending}
            >
              Cancel job
            </Button>
          ) : (
            <Button
              variant="outline"
              disabled={!projectExists}
              title={
                projectExists
                  ? undefined
                  : 'This project is no longer on this machine'
              }
              onClick={() => navigate(`/projects/${job.project_id}`)}
            >
              View project
            </Button>
          )}
        </div>
      </div>

      <dl className="flex flex-wrap gap-x-10 gap-y-3 text-sm leading-5">
        <Field label="Project">
          {projectExists ? (
            <Link
              to={`/projects/${job.project_id}`}
              className="underline underline-offset-4"
            >
              {projectName}
            </Link>
          ) : (
            projectName
          )}
        </Field>
        {job.version_number != null && (
          <Field label="Version">{job.version_number}</Field>
        )}
        <Field label="Started">
          {job.started_at ? formatDateTime(job.started_at) : EMPTY_VALUE}
        </Field>
        <Field label="Ended">
          {job.completed_at ? formatDateTime(job.completed_at) : EMPTY_VALUE}
        </Field>
        <Field label="Duration">
          <span className="tabular-nums">
            {jobDuration(job, now) ?? EMPTY_VALUE}
          </span>
        </Field>
      </dl>

      {job.status === 'failed' && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden />
          <AlertTitle>Job failed</AlertTitle>
          <AlertDescription>
            This job couldn’t be completed successfully. Please see logs for
            details.
          </AlertDescription>
        </Alert>
      )}

      <JobLog job={job} log={log} />
    </main>
  );
};
