export function getWorkspaceVersionLabel(version: {
  manifest_version?: string;
  version_number: number;
}): string {
  return version.manifest_version || `Snapshot ${version.version_number}`;
}
