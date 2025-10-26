import { useState } from 'react';
import { useJobs } from '@/hooks/useJobs';
import { useJobLogStream } from '@/hooks/useJobLogStream';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, ChevronDown, ChevronRight, Radio } from 'lucide-react';
import type { Job } from '@/types';

const statusColors = {
  pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  running: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  completed: 'bg-green-500/10 text-green-500 border-green-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
};

const typeColors = {
  create: 'bg-indigo-500/10 text-indigo-500 border-indigo-500/20',
  delete: 'bg-red-500/10 text-red-500 border-red-500/20',
  install: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  remove: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
  update: 'bg-purple-500/10 text-purple-500 border-purple-500/20',
};

const JobCard = ({ job, isFirst }: { job: Job; isFirst: boolean }) => {
  const [expanded, setExpanded] = useState(isFirst);
  const { logs: streamedLogs, isStreaming } = useJobLogStream(job.id, job.status);

  // Use streamed logs if job is running, otherwise use static logs from job
  const displayLogs = isStreaming ? streamedLogs : job.logs;

  return (
    <Card>
      <CardHeader className="cursor-pointer" onClick={() => setExpanded(!expanded)}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <CardTitle className="text-lg">Job #{job.id}</CardTitle>
            <Badge className={typeColors[job.type]}>
              {job.type}
            </Badge>
            <Badge className={statusColors[job.status]}>
              {job.status}
              {isStreaming && <Radio className="h-3 w-3 ml-1 inline animate-pulse" />}
            </Badge>
          </div>
          <span className="text-sm text-muted-foreground">
            {new Date(job.created_at).toLocaleString()}
          </span>
        </div>
      </CardHeader>
      {expanded && (
        <CardContent className="space-y-4">
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-muted-foreground">Environment ID:</span>
              <span className="ml-2 font-medium">{job.environment_id}</span>
            </div>
            <div>
              <span className="text-muted-foreground">Created:</span>
              <span className="ml-2">{new Date(job.created_at).toLocaleString()}</span>
            </div>
            {job.started_at && (
              <div>
                <span className="text-muted-foreground">Started:</span>
                <span className="ml-2">{new Date(job.started_at).toLocaleString()}</span>
              </div>
            )}
            {job.completed_at && (
              <div>
                <span className="text-muted-foreground">Completed:</span>
                <span className="ml-2">{new Date(job.completed_at).toLocaleString()}</span>
              </div>
            )}
          </div>

          {displayLogs && (
            <div>
              <div className="flex items-center gap-2 mb-2">
                <h4 className="font-semibold">Logs</h4>
                {isStreaming && (
                  <Badge variant="outline" className="text-xs">
                    <Radio className="h-2 w-2 mr-1 animate-pulse" />
                    Live
                  </Badge>
                )}
              </div>
              <pre className="bg-slate-900 text-slate-100 p-4 rounded-md overflow-x-auto max-h-96 overflow-y-auto font-mono whitespace-pre-wrap text-sm">
                {displayLogs}
              </pre>
            </div>
          )}

          {job.error && (
            <div>
              <h4 className="font-semibold text-destructive mb-2">Error</h4>
              <pre className="bg-red-950 text-red-100 p-4 rounded-md overflow-x-auto font-mono whitespace-pre-wrap">
                {job.error}
              </pre>
            </div>
          )}

          {job.metadata && Object.keys(job.metadata).length > 0 && (
            <div className="space-y-3">
              <h4 className="font-semibold">Metadata</h4>
              {Object.entries(job.metadata).map(([key, value]) => {
                const content = typeof value === 'string' ? value : JSON.stringify(value, null, 2);

                return (
                  <div key={key}>
                    <div className="text-sm font-medium text-muted-foreground mb-1 capitalize">
                      {key.replace(/_/g, ' ')}
                    </div>
                    <pre className="bg-slate-900 text-slate-100 p-4 rounded-md overflow-x-auto font-mono whitespace-pre">
                      {content}
                    </pre>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      )}
    </Card>
  );
};

export const Jobs = () => {
  const { data: jobs, isLoading } = useJobs();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Jobs</h1>
        <p className="text-muted-foreground">View all job executions and their status</p>
      </div>

      <div className="space-y-4">
        {jobs?.map((job, index) => (
          <JobCard key={job.id} job={job} isFirst={index === 0} />
        ))}
      </div>

      {jobs?.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No jobs yet</p>
        </div>
      )}
    </div>
  );
};
