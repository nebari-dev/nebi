import {
  ArrowLeft,
  Check,
  ChevronRight,
  ChevronUp,
  Copy,
  Download,
  Globe,
  Loader2,
  Lock,
  Package,
  Search,
  Settings,
} from 'lucide-react';
import { useId, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { RemoteUnreachableBanner } from '@/components/remote/RemoteUnreachableBanner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useIsAdmin } from '@/hooks/useAdmin';
import {
  useImportEnvironment,
  usePublicRegistries,
  useRegistryRepositories,
  useRepositoryTags,
} from '@/hooks/useRegistries';
import { useRemoteRegistries, useRemoteView } from '@/hooks/useRemote';
import { buildImportCommand } from '@/lib/registry';
import { cn } from '@/lib/utils';

export const Registries = () => {
  const navigate = useNavigate();
  const { data: isAdmin } = useIsAdmin();
  const { data: registries, isLoading: registriesLoading } =
    usePublicRegistries();

  // View mode support for local desktop app
  const { viewMode, isRemoteConnected, isRemoteView } = useRemoteView();
  const {
    data: remoteRegistries,
    isFirstLoad: remoteFirstLoad,
    isUnreachable: remoteIsUnreachable,
  } = useRemoteRegistries(isRemoteConnected);

  // Show registries based on view mode when connected to remote
  const displayedRegistries = useMemo(() => {
    if (!isRemoteConnected) {
      return registries || [];
    }
    // When connected, show based on viewMode
    if (viewMode === 'local') {
      return registries || [];
    } else {
      return remoteRegistries || [];
    }
  }, [registries, remoteRegistries, isRemoteConnected, viewMode]);

  const remoteUnreachable = isRemoteView && remoteIsUnreachable;
  // Full-page spinner only until the remote list first resolves or errors
  // (see isFirstLoad in useRemote.ts).
  const isLoading = registriesLoading || (isRemoteConnected && remoteFirstLoad);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <div className="flex-1">
          <h1 className="text-3xl font-bold">Registries</h1>
          <p className="text-muted-foreground">
            Browse OCI registries and import environments
          </p>
        </div>
        {isAdmin && (
          <Button
            variant="outline"
            onClick={() => navigate('/admin/registries')}
          >
            <Settings className="h-4 w-4 mr-2" />
            Manage Registries
          </Button>
        )}
      </div>

      {remoteUnreachable && <RemoteUnreachableBanner />}

      <Table aria-label="Registries">
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>URL</TableHead>
            <TableHead>Default</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {displayedRegistries.map((registry) => (
            <TableRow key={registry.id}>
              <TableCell className="font-medium">{registry.name}</TableCell>
              <TableCell className="font-mono text-muted-foreground">
                {registry.url}
              </TableCell>
              <TableCell>
                {registry.is_default && (
                  <Badge className="bg-blue-100 text-blue-800 border-blue-300">
                    Default
                  </Badge>
                )}
              </TableCell>
              <TableCell className="text-right">
                <Button
                  size="sm"
                  onClick={() => navigate(`/registries/${registry.id}`)}
                >
                  <Package className="mr-2 h-4 w-4" />
                  Browse
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {displayedRegistries.length === 0 && !remoteUnreachable && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No registries configured.{' '}
            {isAdmin ? (
              <Link
                to="/admin/registries"
                className="text-primary hover:underline"
              >
                Add one in Admin &rarr; Registries.
              </Link>
            ) : (
              'Ask an admin to add one.'
            )}
          </p>
        </div>
      )}
    </div>
  );
};

const RepositoryRow = ({
  registryId,
  repoName,
  registry,
  isPublic,
  showVisibility,
}: {
  registryId: string;
  repoName: string;
  registry: { url: string; name: string } | undefined;
  isPublic?: boolean;
  showVisibility: boolean;
}) => {
  const navigate = useNavigate();
  const { data: tagData, isLoading: tagsLoading } = useRepositoryTags(
    registryId,
    repoName,
  );
  const importMutation = useImportEnvironment();

  const [selectedTag, setSelectedTag] = useState('');
  const [copiedTag, setCopiedTag] = useState<string | null>(null);
  const [showImport, setShowImport] = useState(false);
  const [importName, setImportName] = useState('');
  const [error, setError] = useState('');
  const importNameId = useId();

  const tags = tagData?.tags || [];
  const defaultTag =
    tags.find((tag) => tag.name === 'latest')?.name || tags[0]?.name || '';
  const effectiveTag = selectedTag || defaultTag;

  const handleCopyImportCmd = async () => {
    if (!registry || !effectiveTag) return;
    const cmd = buildImportCommand(registry.url, repoName, effectiveTag);
    await navigator.clipboard.writeText(cmd);
    setCopiedTag(effectiveTag);
    setTimeout(() => setCopiedTag(null), 2000);
  };

  const handleToggleImport = () => {
    if (showImport) {
      setShowImport(false);
      return;
    }
    if (!effectiveTag) return;
    const repoBaseName = repoName.split('/').pop() || repoName;
    setImportName(`${repoBaseName}-${effectiveTag}`);
    setShowImport(true);
    setError('');
  };

  const handleImport = async () => {
    if (!registryId || !importName.trim()) return;
    setError('');
    try {
      await importMutation.mutateAsync({
        registryId,
        data: {
          repository_path: repoName,
          tag: effectiveTag,
          name: importName.trim(),
        },
      });
      setShowImport(false);
      navigate('/workspaces');
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } } };
      setError(e?.response?.data?.error || 'Failed to import environment.');
    }
  };

  const colSpan = showVisibility ? 5 : 4;

  return (
    <>
      <TableRow className={cn(showImport && 'border-0')}>
        <TableCell className="font-mono">{repoName}</TableCell>
        {showVisibility && (
          <TableCell>
            {isPublic === undefined ? (
              <Badge variant="outline" className="text-muted-foreground">
                Unknown
              </Badge>
            ) : isPublic ? (
              <Badge className="bg-green-100 text-green-800 border-green-300">
                <Globe className="mr-1 h-3 w-3" />
                Public
              </Badge>
            ) : (
              <Badge className="bg-orange-100 text-orange-800 border-orange-300">
                <Lock className="mr-1 h-3 w-3" />
                Private
              </Badge>
            )}
          </TableCell>
        )}
        <TableCell className="w-40">
          {tagsLoading ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          ) : tags.length > 0 ? (
            <Select
              value={effectiveTag}
              onValueChange={(tag: string | null) => setSelectedTag(tag ?? '')}
            >
              <SelectTrigger
                className="w-full"
                aria-label={`Select tag for ${repoName}`}
              >
                <SelectValue>
                  {(value: string | null) => value || 'Select tag'}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {tags.map((tag) => (
                  <SelectItem key={tag.name} value={tag.name}>
                    {tag.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <span className="text-sm text-muted-foreground">No tags</span>
          )}
        </TableCell>
        <TableCell>
          {registry && effectiveTag && !tagsLoading ? (
            <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2 max-w-sm">
              <code className="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-xs text-foreground">
                {buildImportCommand(registry.url, repoName, effectiveTag)}
              </code>
              <Button
                size="icon"
                variant="ghost"
                className="h-7 w-7 shrink-0"
                onClick={handleCopyImportCmd}
                title="Copy command"
              >
                {copiedTag === effectiveTag ? (
                  <Check className="h-3.5 w-3.5" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </Button>
            </div>
          ) : null}
        </TableCell>
        <TableCell className="text-right">
          <div className="flex items-center justify-end gap-2">
            <Button
              size="sm"
              variant={showImport ? 'outline' : 'default'}
              onClick={handleToggleImport}
              disabled={!showImport && (!effectiveTag || tagsLoading)}
              aria-expanded={showImport}
              title={
                showImport
                  ? 'Close the import panel'
                  : 'Import this environment into a new workspace'
              }
            >
              {showImport ? (
                <>
                  <ChevronUp className="mr-2 h-4 w-4" />
                  Close
                </>
              ) : (
                <>
                  <Download className="mr-2 h-4 w-4" />
                  Import
                </>
              )}
            </Button>
          </div>
        </TableCell>
      </TableRow>
      {showImport && registry && (
        <TableRow>
          <TableCell colSpan={colSpan} className="bg-muted/30">
            <div className="space-y-4">
              {error && (
                <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded text-sm">
                  {error}
                </div>
              )}
              <div className="grid grid-cols-[1fr_2fr_1fr] gap-4 text-sm">
                <div>
                  <span className="text-muted-foreground">Registry</span>
                  <p className="font-medium">{registry.name}</p>
                </div>
                <div className="min-w-0">
                  <span className="text-muted-foreground">Repository</span>
                  <p className="font-medium font-mono break-all">{repoName}</p>
                </div>
                <div>
                  <span className="text-muted-foreground">Tag</span>
                  <p className="font-medium font-mono">{effectiveTag}</p>
                </div>
              </div>
              <div className="space-y-3">
                <label
                  htmlFor={importNameId}
                  className="text-sm font-medium mb-2 block"
                >
                  Workspace Name
                </label>
                <Input
                  id={importNameId}
                  value={importName}
                  onChange={(e) => setImportName(e.target.value)}
                  placeholder="Enter workspace name"
                  autoFocus
                />
              </div>
              <div className="flex gap-2 justify-end">
                <Button variant="outline" onClick={() => setShowImport(false)}>
                  Cancel
                </Button>
                <Button
                  onClick={handleImport}
                  disabled={importMutation.isPending || !importName.trim()}
                >
                  {importMutation.isPending ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Importing...
                    </>
                  ) : (
                    <>
                      <Download className="mr-2 h-4 w-4" />
                      Import
                    </>
                  )}
                </Button>
              </div>
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  );
};

export const RegistryRepositories = () => {
  const navigate = useNavigate();
  const { registryId } = useParams<{ registryId: string }>();
  const { data: registries, isLoading: registriesLoading } =
    usePublicRegistries();

  const [searchQuery, setSearchQuery] = useState('');
  const selectedRegistry = registries?.find((r) => r.id === registryId);

  const { data: repoData, isLoading: reposLoading } = useRegistryRepositories(
    registryId || '',
    searchQuery || undefined,
  );

  if (registriesLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const hasRepos = (repoData?.repositories?.length || 0) > 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate('/registries')}
          aria-label="Back to registries"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link
              to="/registries"
              className="hover:text-foreground transition-colors"
            >
              Registries
            </Link>
            <ChevronRight className="h-3 w-3" />
            <span className="text-foreground">
              {selectedRegistry?.name || 'Registry'}
            </span>
          </div>
          <h1 className="text-3xl font-bold">
            {selectedRegistry?.name || 'Registry'}
          </h1>
          <p className="text-muted-foreground">
            Browse repositories in this registry
          </p>
        </div>
      </div>

      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            className="pl-9"
            placeholder="Search repositories..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      {reposLoading ? (
        <div className="flex items-center justify-center h-48">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <>
          {repoData?.fallback && (
            <div className="bg-yellow-500/10 border border-yellow-500/20 text-yellow-600 dark:text-yellow-400 px-4 py-3 rounded text-sm">
              Catalog API not available for this registry. Showing known
              publications below.
            </div>
          )}

          {hasRepos ? (
            <Table aria-label="Repositories">
              <TableHeader>
                <TableRow>
                  <TableHead>Repository</TableHead>
                  <TableHead>Visibility</TableHead>
                  <TableHead>Tag</TableHead>
                  <TableHead>Command</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {repoData?.repositories?.map((repo) => (
                  <RepositoryRow
                    key={repo.name}
                    registryId={registryId || ''}
                    repoName={repo.name}
                    registry={selectedRegistry}
                    isPublic={repo.is_public}
                    showVisibility={true}
                  />
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="text-center py-8">
              <p className="text-muted-foreground">
                No repositories found in this registry.
              </p>
            </div>
          )}
        </>
      )}
    </div>
  );
};
