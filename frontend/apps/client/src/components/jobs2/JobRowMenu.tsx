// Owner: U8 (jobs). Row actions on the Jobs page: View project (disabled
// when the project is gone) and Cancel job (queued or running jobs).
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { Ellipsis } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { isActiveJob, type JobSummary } from '@/types/contracts';

export interface JobRowMenuProps {
  job: JobSummary;
  projectName: string;
  projectExists: boolean;
  onCancel: (job: JobSummary) => void;
}

export const JobRowMenu = ({
  job,
  projectName,
  projectExists,
  onCancel,
}: JobRowMenuProps) => {
  const navigate = useNavigate();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        variant="ghost"
        className="size-7 px-0"
        aria-label={`Actions for ${job.type_label} on ${projectName}`}
      >
        <Ellipsis />
      </DropdownMenuTrigger>
      <DropdownMenuPortal>
        <DropdownMenuContent align="end" className="min-w-40">
          <DropdownMenuItem
            disabled={!projectExists}
            onClick={() => navigate(`/projects/${job.project_id}`)}
          >
            View project
          </DropdownMenuItem>
          {isActiveJob(job) && (
            <DropdownMenuItem onClick={() => onCancel(job)}>
              Cancel job
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenuPortal>
    </DropdownMenu>
  );
};
