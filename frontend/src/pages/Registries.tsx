import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useModeStore } from '@/store/modeStore';
import { usePublicRegistries, useRegistryRepositories, useRepositoryTags, useImportEnvironment } from '@/hooks/useRegistries';
import type { OCIRegistry } from '@/types';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, ArrowLeft, Search, Download, Package, ChevronRight, X, Globe, Lock } from 'lucide-react';

type View = 'registries' | 'repositories' | 'tags';

export const Registries = () => {
  const navigate = useNavigate();
  const isLocalMode = useModeStore((s) => s.isLocalMode());
  const { data: registries, isLoading: registriesLoading } = usePublicRegistries();
  const importMutation = useImportEnvironment();

  const [view, setView] = useState<View>('registries');
  const [selectedRegistry, setSelectedRegistry] = useState<OCIRegistry | null>(null);
  const [selectedRepo, setSelectedRepo] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [manualRepo, setManualRepo] = useState('');
  const [error, setError] = useState('');

  // Import dialog state
  const [showImport, setShowImport] = useState(false);
  const [importTag, setImportTag] = useState('');
  const [importName, setImportName] = useState('');

  const { data: repoData, isLoading: reposLoading } = useRegistryRepositories(
    selectedRegistry?.id || '',
    searchQuery || undefined
  );

  const { data: tagData, isLoading: tagsLoading } = useRepositoryTags(
    selectedRegistry?.id || '',
    selectedRepo
  );

  const handleBrowseRegistry = (registry: OCIRegistry) => {
    setSelectedRegistry(registry);
    setSelectedRepo('');
    setSearchQuery('');
    setManualRepo('');
    setError('');
    setView('repositories');
  };

  const handleViewTags = (repoName: string) => {
    setSelectedRepo(repoName);
    setError('');
    setView('tags');
  };

  const handleManualRepoSubmit = () => {
    if (manualRepo.trim()) {
      handleViewTags(manualRepo.trim());
    }
  };

  const handleOpenImport = (tag: string) => {
    // Pre-fill name from repo + tag
    const repoBaseName = selectedRepo.split('/').pop() || selectedRepo;
    setImportTag(tag);
    setImportName(`${repoBaseName}-${tag}`);
    setShowImport(true);
    setError('');
  };

  const handleImport = async () => {
    if (!selectedRegistry || !importName.trim()) return;

    setError('');
    try {
      await importMutation.mutateAsync({
        registryId: selectedRegistry.id,
        data: {
          repository: selectedRepo,
          tag: importTag,
          name: importName.trim(),
        },
      });
      setShowImport(false);
      navigate('/workspaces');
    } catch (err) {
      const error = err as { response?: { data?: { error?: string } } };
      setError(error?.response?.data?.error || 'Failed to import environment.');
    }
  };

  const handleBack = () => {
    if (view === 'tags') {
      setSelectedRepo('');
      setView('repositories');
    } else if (view === 'repositories') {
      setSelectedRegistry(null);
      setView('registries');
    }
    setError('');
  };

  if (registriesLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header with breadcrumbs */}
      <div className="flex items-center gap-4">
        {view !== 'registries' && (
          <Button variant="ghost" size="icon" onClick={handleBack}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
        )}
        <div>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <button
              className="hover:text-foreground transition-colors"
              onClick={() => {
                setView('registries');
                setSelectedRegistry(null);
                setSelectedRepo('');
              }}
            >
              Registries
            </button>
            {selectedRegistry && (
              <>
                <ChevronRight className="h-3 w-3" />
                <button
                  className="hover:text-foreground transition-colors"
                  onClick={() => {
                    setView('repositories');
                    setSelectedRepo('');
                  }}
                >
                  {selectedRegistry.name}
                </button>
              </>
            )}
            {selectedRepo && (
              <>
                <ChevronRight className="h-3 w-3" />
                <span className="text-foreground">{selectedRepo}</span>
              </>
            )}
          </div>
          <h1 className="text-3xl font-bold">
            {view === 'registries' && 'Registries'}
            {view === 'repositories' && selectedRegistry?.name}
            {view === 'tags' && selectedRepo}
          </h1>
          <p className="text-muted-foreground">
            {view === 'registries' && 'Browse OCI registries and import environments'}
            {view === 'repositories' && 'Browse repositories in this registry'}
            {view === 'tags' && 'Select a tag to import'}
          </p>
        </div>
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">
          {error}
        </div>
      )}

      {/* Import Dialog */}
      {showImport && selectedRegistry && (
        <Card>
          <CardHeader>
            <div className="flex justify-between items-center">
              <CardTitle>Import Environment</CardTitle>
              <Button variant="ghost" size="icon" onClick={() => setShowImport(false)}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-3 gap-4 text-sm">
              <div>
                <span className="text-muted-foreground">Registry</span>
                <p className="font-medium">{selectedRegistry.name}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Repository</span>
                <p className="font-medium font-mono">{selectedRepo}</p>
              </div>
              <div>
                <span className="text-muted-foreground">Tag</span>
                <p className="font-medium font-mono">{importTag}</p>
              </div>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Workspace Name</label>
              <Input
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
          </CardContent>
        </Card>
      )}

      {/* View 1: Registry List */}
      {view === 'registries' && (
        <Card>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="border-b bg-muted/50">
                  <tr>
                    <th className="text-left p-4 font-medium">Name</th>
                    <th className="text-left p-4 font-medium">URL</th>
                    <th className="text-left p-4 font-medium">Default</th>
                    <th className="text-right p-4 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {registries?.map((registry) => (
                    <tr key={registry.id} className="border-b last:border-0 hover:bg-muted/50">
                      <td className="p-4 font-medium">{registry.name}</td>
                      <td className="p-4 font-mono text-sm text-muted-foreground">{registry.url}</td>
                      <td className="p-4">
                        {registry.is_default && (
                          <Badge className="bg-blue-500/10 text-blue-500 border-blue-500/20">
                            Default
                          </Badge>
                        )}
                      </td>
                      <td className="p-4 text-right">
                        <Button size="sm" onClick={() => handleBrowseRegistry(registry)}>
                          <Package className="mr-2 h-4 w-4" />
                          Browse
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {view === 'registries' && (!registries || registries.length === 0) && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            No registries configured.{' '}
            {isLocalMode ? (
              <Link to="/admin/registries" className="text-primary hover:underline">
                Add one in Admin &rarr; Registries.
              </Link>
            ) : (
              'Ask an admin to add one.'
            )}
          </p>
        </div>
      )}

      {/* View 2: Repository List */}
      {view === 'repositories' && (
        <>
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
                  Catalog API not available for this registry. Showing known publications below.
                </div>
              )}

              {/* Manual repo input — always shown so users can navigate directly */}
              <div className="flex gap-2">
                <Input
                  placeholder="Enter repository path (e.g., org/env-name)"
                  value={manualRepo}
                  onChange={(e) => setManualRepo(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      handleManualRepoSubmit();
                    }
                  }}
                />
                <Button onClick={handleManualRepoSubmit} disabled={!manualRepo.trim()}>
                  View Tags
                </Button>
              </div>

              {repoData?.repositories && repoData.repositories.length > 0 && (
                <Card>
                  <CardContent className="p-0">
                    <div className="overflow-x-auto">
                      <table className="w-full">
                        <thead className="border-b bg-muted/50">
                          <tr>
                            <th className="text-left p-4 font-medium">Repository</th>
                            <th className="text-left p-4 font-medium">Visibility</th>
                            <th className="text-right p-4 font-medium">Actions</th>
                          </tr>
                        </thead>
                        <tbody>
                          {repoData.repositories.map((repo) => (
                            <tr key={repo.name} className="border-b last:border-0 hover:bg-muted/50">
                              <td className="p-4 font-mono text-sm">{repo.name}</td>
                              <td className="p-4">
                                {repo.is_public === undefined ? (
                                  <Badge variant="outline" className="text-muted-foreground">Unknown</Badge>
                                ) : repo.is_public ? (
                                  <Badge className="bg-green-500/10 text-green-600 border-green-500/20">
                                    <Globe className="mr-1 h-3 w-3" />
                                    Public
                                  </Badge>
                                ) : (
                                  <Badge className="bg-orange-500/10 text-orange-600 border-orange-500/20">
                                    <Lock className="mr-1 h-3 w-3" />
                                    Private
                                  </Badge>
                                )}
                              </td>
                              <td className="p-4 text-right">
                                <Button size="sm" variant="outline" onClick={() => handleViewTags(repo.name)}>
                                  View Tags
                                  <ChevronRight className="ml-2 h-4 w-4" />
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </CardContent>
                </Card>
              )}

              {repoData?.repositories?.length === 0 && (
                <div className="text-center py-8">
                  <p className="text-muted-foreground">No repositories discovered automatically. Use the field above to enter a repository path directly.</p>
                </div>
              )}
            </>
          )}
        </>
      )}

      {/* View 3: Tag List */}
      {view === 'tags' && (
        <>
          {tagsLoading ? (
            <div className="flex items-center justify-center h-48">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <>
              <Card>
                <CardContent className="p-0">
                  <div className="overflow-x-auto">
                    <table className="w-full">
                      <thead className="border-b bg-muted/50">
                        <tr>
                          <th className="text-left p-4 font-medium">Tag</th>
                          <th className="text-right p-4 font-medium">Actions</th>
                        </tr>
                      </thead>
                      <tbody>
                        {tagData?.tags?.map((tag) => (
                          <tr key={tag.name} className="border-b last:border-0 hover:bg-muted/50">
                            <td className="p-4 font-mono text-sm">{tag.name}</td>
                            <td className="p-4 text-right">
                              <Button size="sm" onClick={() => handleOpenImport(tag.name)}>
                                <Download className="mr-2 h-4 w-4" />
                                Import
                              </Button>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </CardContent>
              </Card>

              {tagData?.tags?.length === 0 && (
                <div className="text-center py-12">
                  <p className="text-muted-foreground">No tags found for this repository.</p>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
};
