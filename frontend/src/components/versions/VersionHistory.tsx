import { useState } from 'react';
import { useVersions, useRollback, useDownloadLockFile, useDownloadManifest } from '@/hooks/useVersions';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Loader2, Clock, RotateCcw, FileCode, FileText, ChevronRight, Eye } from 'lucide-react';
import type { EnvironmentVersion } from '@/types';
import { environmentsApi } from '@/api/environments';

interface VersionHistoryProps {
  environmentId: string;
  environmentStatus: string;
}

export const VersionHistory = ({ environmentId, environmentStatus }: VersionHistoryProps) => {
  const { data: versions, isLoading } = useVersions(environmentId);
  const rollbackMutation = useRollback(environmentId);
  const downloadLock = useDownloadLockFile();
  const downloadManifest = useDownloadManifest();

  const [confirmRollback, setConfirmRollback] = useState<EnvironmentVersion | null>(null);
  const [expandedVersion, setExpandedVersion] = useState<string | null>(null);

  const handleRollback = async () => {
    if (confirmRollback) {
      await rollbackMutation.mutateAsync({ version_number: confirmRollback.version_number });
      setConfirmRollback(null);
    }
  };

  const handleDownloadLock = (version: EnvironmentVersion) => {
    downloadLock.mutate({ environmentId, versionNumber: version.version_number });
  };

  const handleDownloadManifest = (version: EnvironmentVersion) => {
    downloadManifest.mutate({ environmentId, versionNumber: version.version_number });
  };

  const handleViewLock = async (version: EnvironmentVersion) => {
    try {
      const content = await environmentsApi.downloadLockFile(environmentId, version.version_number);
      const blob = new Blob([content], { type: 'text/plain' });
      const url = window.URL.createObjectURL(blob);
      window.open(url, '_blank');
      setTimeout(() => window.URL.revokeObjectURL(url), 100);
    } catch (error) {
      console.error('Failed to view lock file:', error);
    }
  };

  const handleViewManifest = async (version: EnvironmentVersion) => {
    try {
      const content = await environmentsApi.downloadManifest(environmentId, version.version_number);
      const blob = new Blob([content], { type: 'text/plain' });
      const url = window.URL.createObjectURL(blob);
      window.open(url, '_blank');
      setTimeout(() => window.URL.revokeObjectURL(url), 100);
    } catch (error) {
      console.error('Failed to view manifest:', error);
    }
  };

  const toggleExpand = (versionId: string) => {
    setExpandedVersion(expandedVersion === versionId ? null : versionId);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!versions || versions.length === 0) {
    return (
      <Card>
        <CardContent className="pt-6">
          <div className="text-center py-12">
            <Clock className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
            <h3 className="text-lg font-semibold mb-2">No Version History Yet</h3>
            <p className="text-muted-foreground text-sm">
              Version snapshots will be created automatically when you install or remove packages.
            </p>
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-2xl font-bold">Version History</h2>
            <p className="text-muted-foreground text-sm mt-1">
              {versions.length} version{versions.length !== 1 ? 's' : ''} • Roll back to any previous state
            </p>
          </div>
        </div>

        <div className="relative">
          {/* Timeline line */}
          <div className="absolute left-[21px] top-8 bottom-8 w-0.5 bg-border" />

          <div className="space-y-4">
            {versions.map((version, index) => {
              const isLatest = index === 0;
              const isExpanded = expandedVersion === version.id;

              return (
                <Card
                  key={version.id}
                  className={`relative transition-all ${
                    isLatest ? 'border-primary shadow-md' : ''
                  } ${isExpanded ? 'ring-2 ring-primary/20' : ''}`}
                >
                  <CardContent className="pt-6">
                    <div className="flex items-start gap-4">
                      {/* Timeline dot */}
                      <div className="relative flex-shrink-0 mt-1">
                        <div
                          className={`w-10 h-10 rounded-full border-4 border-background flex items-center justify-center font-bold text-sm ${
                            isLatest
                              ? 'bg-primary text-primary-foreground'
                              : 'bg-muted text-muted-foreground'
                          }`}
                        >
                          v{version.version_number}
                        </div>
                      </div>

                      {/* Content */}
                      <div className="flex-1 min-w-0">
                        <div className="flex items-start justify-between gap-4 mb-3">
                          <div className="flex-1">
                            <div className="flex items-center gap-2 mb-1">
                              <h3 className="font-semibold text-lg">
                                Version {version.version_number}
                              </h3>
                              {isLatest && (
                                <Badge className="bg-primary/10 text-primary border-primary/20">
                                  Current
                                </Badge>
                              )}
                            </div>
                            {version.description && (
                              <p className="text-sm text-muted-foreground mb-2">
                                {version.description}
                              </p>
                            )}
                            <div className="flex items-center gap-4 text-xs text-muted-foreground">
                              <div className="flex items-center gap-1">
                                <Clock className="h-3 w-3" />
                                {new Date(version.created_at).toLocaleString()}
                              </div>
                            </div>
                          </div>

                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => toggleExpand(version.id)}
                            className="flex-shrink-0"
                          >
                            <ChevronRight
                              className={`h-4 w-4 transition-transform ${
                                isExpanded ? 'rotate-90' : ''
                              }`}
                            />
                          </Button>
                        </div>

                        {/* Expanded actions */}
                        {isExpanded && (
                          <div className="mt-4 pt-4 border-t space-y-3">
                            <div className="flex flex-wrap gap-2">
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleViewLock(version)}
                              >
                                <Eye className="h-4 w-4 mr-2" />
                                View Lock File
                              </Button>

                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleDownloadLock(version)}
                                disabled={downloadLock.isPending}
                              >
                                <FileCode className="h-4 w-4 mr-2" />
                                Download Lock File
                              </Button>

                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleViewManifest(version)}
                              >
                                <Eye className="h-4 w-4 mr-2" />
                                View Manifest
                              </Button>

                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleDownloadManifest(version)}
                                disabled={downloadManifest.isPending}
                              >
                                <FileText className="h-4 w-4 mr-2" />
                                Download Manifest
                              </Button>

                              {!isLatest && (
                                <Button
                                  variant="default"
                                  size="sm"
                                  onClick={() => setConfirmRollback(version)}
                                  disabled={
                                    rollbackMutation.isPending || environmentStatus !== 'ready'
                                  }
                                >
                                  <RotateCcw className="h-4 w-4 mr-2" />
                                  Rollback to This Version
                                </Button>
                              )}
                            </div>

                            {!isLatest && environmentStatus !== 'ready' && (
                              <p className="text-xs text-muted-foreground">
                                Environment must be ready to perform rollback
                              </p>
                            )}
                          </div>
                        )}
                      </div>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={!!confirmRollback}
        onOpenChange={(open) => !open && setConfirmRollback(null)}
        onConfirm={handleRollback}
        title="Rollback Environment"
        description={
          confirmRollback
            ? `Are you sure you want to rollback to version ${confirmRollback.version_number}? This will restore the environment to its state at that version and create a new version snapshot.`
            : ''
        }
        confirmText="Rollback"
        cancelText="Cancel"
      />
    </>
  );
};
