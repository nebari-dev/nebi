import { ConfirmDialog } from '@nebi/ui/components/confirm-dialog';
import { PixiTomlEditor } from '@nebi/ui/components/project/PixiTomlEditor';
import { SplitButton } from '@nebi/ui/components/split-button';
import { Badge } from '@nebi/ui/components/ui/badge';
import { Button } from '@nebi/ui/components/ui/button';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@nebi/ui/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@nebi/ui/components/ui/table';
import { Check, Copy, Download, Loader2, Plus, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { InstallControls } from '@/components/project/InstallControls';
import {
  useCreateProject,
  useDeleteProject,
  useProjects,
} from '@/hooks/useProjects';
import { getInstallStatusColor, getProjectStatusColor } from '@/lib/status';
import { capitalize } from '@/lib/strings';
import { useProjectNavStore } from '@/store/projectNavStore';

const DEFAULT_PIXI_TOML = `[workspace]
name = ""
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
python = ">=3.11"
ipykernel = "*"
`;
// TODO: Robustify. Maybe use a proper TOML parser?
const getTomlName = (toml: string): string | null => {
  const match = toml.match(/^name\s*=\s*"([^"]*)"/m);
  return match ? match[1] : null;
};
export const Projects = () => {
  const navigate = useNavigate();
  const setPendingTab = useProjectNavStore((s) => s.setPendingTab);
  const { data: projects, isLoading } = useProjects();
  const createMutation = useCreateProject();
  const deleteMutation = useDeleteProject();
  const [showCreate, setShowCreate] = useState(false);
  const [pixiToml, setPixiToml] = useState(DEFAULT_PIXI_TOML);
  const [confirmDelete, setConfirmDelete] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [error, setError] = useState('');
  const [envJobNotice, setEnvJobNotice] = useState<{
    projectId: string;
    projectName: string;
    jobId: string;
    type: string;
  } | null>(null);
  const [copiedPullId, setCopiedPullId] = useState<string | null>(null);
  const displayedProjects = projects || [];
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const projectName = getTomlName(pixiToml);
    if (!projectName?.trim()) {
      setError(
        'Project name is required in the pixi.toml [workspace] section.',
      );
      return;
    }
    setError('');
    try {
      const tomlContent = pixiToml;
      const project = await createMutation.mutateAsync({
        name: projectName,
        pixi_toml: tomlContent,
      });
      setPixiToml(DEFAULT_PIXI_TOML);
      setShowCreate(false);
      setPendingTab('jobs');
      navigate(`/projects/${project.id}`);
    } catch (err) {
      const error = err as {
        response?: {
          data?: {
            error?: string;
          };
        };
      };
      const errorMessage =
        error?.response?.data?.error ||
        'Failed to create project. Please try again.';
      setError(errorMessage);
    }
  };
  const handleDelete = async () => {
    if (!confirmDelete) return;
    setError('');
    try {
      await deleteMutation.mutateAsync(confirmDelete.id);
      setConfirmDelete(null);
    } catch (err) {
      const error = err as {
        response?: {
          data?: {
            error?: string;
          };
        };
      };
      const errorMessage =
        error?.response?.data?.error ||
        'Failed to delete project. Please try again.';
      setError(errorMessage);
      setConfirmDelete(null);
    }
  };
  const handleCopyPull = async (
    e: React.MouseEvent,
    projectName: string,
    projectId: string,
  ) => {
    e.stopPropagation();
    const serverUrl = window.location.origin;
    const cmd = `nebi login ${serverUrl} && nebi pull ${projectName}`;
    await navigator.clipboard.writeText(cmd);
    setCopiedPullId(projectId);
    setTimeout(() => setCopiedPullId(null), 2000);
  };
  const isCreatePending = createMutation.isPending;
  const isDeletePending = deleteMutation.isPending;
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }
  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Projects</h1>
          <p className="text-muted-foreground">
            Manage your development projects
          </p>
        </div>
        {!showCreate && (
          <SplitButton
            onPrimary={() => {
              setShowCreate(true);
              setError('');
            }}
            primaryLabel={
              <>
                <Plus className="h-4 w-4 mr-2" />
                New Project
              </>
            }
            menuLabel="Open project actions"
            menuItems={[
              {
                label: 'Import Project from Registry',
                icon: <Download className="h-4 w-4" />,
                onClick: () => navigate('/registries'),
              },
            ]}
          />
        )}
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {envJobNotice && (
        <div className="rounded-md border border-blue-500/20 bg-blue-500/10 px-4 py-3 text-sm text-blue-700">
          <div className="flex items-center justify-between gap-3">
            <span>
              {envJobNotice.type === 'env_uninstall' ? 'Uninstall' : 'Install'}{' '}
              job started for "{envJobNotice.projectName}" (ID:{' '}
              {envJobNotice.jobId}
              ).
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setPendingTab('jobs');
                  navigate(`/projects/${envJobNotice.projectId}`);
                }}
              >
                View logs
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 text-blue-700 hover:bg-blue-500/10 hover:text-blue-700"
                onClick={() => setEnvJobNotice(null)}
                aria-label="Dismiss notification"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      )}

      {showCreate && (
        <Card>
          <CardHeader>
            <div className="flex justify-between items-center">
              <CardTitle>Create New Project</CardTitle>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setShowCreate(false)}
                aria-label="Close create project form"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleCreate} className="space-y-4">
              <PixiTomlEditor
                tomlValue={pixiToml}
                onTomlChange={setPixiToml}
                projectName={getTomlName(pixiToml) || ''}
              />

              <div className="flex gap-2 justify-end">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setShowCreate(false)}
                >
                  Cancel
                </Button>
                <Button
                  render={<button type="submit" />}
                  disabled={isCreatePending || !getTomlName(pixiToml)?.trim()}
                >
                  {isCreatePending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Creating...
                    </>
                  ) : (
                    'Create & Save'
                  )}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      <Table aria-label="Projects">
        <TableHeader>
          <TableRow
            className={displayedProjects.length > 0 ? undefined : 'border-0'}
          >
            <TableHead>Name</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Size</TableHead>
            <TableHead>Created</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {displayedProjects.map((project) => (
            <TableRow
              key={project.id}
              className="cursor-pointer"
              onClick={() => navigate(`/projects/${project.id}`)}
            >
              <TableCell className="font-medium">
                <div className="flex items-center gap-2">{project.name}</div>
                {project.path && (
                  <div
                    className="text-xs text-muted-foreground font-normal mt-0.5 font-mono truncate max-w-sm"
                    title={project.path}
                  >
                    {project.path}
                  </div>
                )}
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-1.5">
                  <Badge className={getProjectStatusColor(project.status)}>
                    {capitalize(project.status)}
                  </Badge>
                  {project.install_status && (
                    <Badge
                      className={getInstallStatusColor(project.install_status)}
                    >
                      {capitalize(project.install_status.replaceAll('_', ' '))}
                    </Badge>
                  )}
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {project.size_formatted || '-'}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {new Date(project.created_at).toLocaleDateString()}
              </TableCell>
              <TableCell>
                <div className="flex justify-end gap-2">
                  <InstallControls
                    projectId={project.id}
                    installStatus={project.install_status}
                    appearance="icon"
                    onStarted={(job) =>
                      setEnvJobNotice({
                        projectId: project.id,
                        projectName: project.name,
                        jobId: job.id,
                        type: job.type,
                      })
                    }
                  />
                  {project.source !== 'local' && (
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={(e) =>
                        handleCopyPull(e, project.name, project.id)
                      }
                      aria-label={`Copy pull command for ${project.name}`}
                      title="Copy nebi pull command"
                    >
                      {copiedPullId === project.id ? (
                        <Check className="h-4 w-4" />
                      ) : (
                        <Copy className="h-4 w-4" />
                      )}
                    </Button>
                  )}

                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={(e) => {
                      e.stopPropagation();
                      setConfirmDelete({
                        id: project.id,
                        name: project.name,
                      });
                    }}
                    disabled={isDeletePending}
                    aria-label={`Delete ${project.name}`}
                    title="Delete project"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {displayedProjects.length === 0 && !showCreate && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No projects yet. Create your first one!
          </p>
        </div>
      )}

      <ConfirmDialog
        open={!!confirmDelete}
        onOpenChange={(open) => !open && setConfirmDelete(null)}
        onConfirm={handleDelete}
        title="Delete Project"
        description={`Are you sure you want to delete "${confirmDelete?.name}"? This action cannot be undone. All data associated with this project will be permanently removed.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />
    </div>
  );
};
