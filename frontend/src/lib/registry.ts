/**
 * Build a `nebi import` CLI command for a given registry + repo + tag.
 *
 * `repo` already contains the full path (e.g. "nebari_environments/data-science-demo")
 * as returned by the registry API, so the namespace must NOT be prepended again.
 */
export function buildImportCommand(registryUrl: string, repo: string, tag: string): string {
  const host = registryUrl.replace(/^https?:\/\//, '').replace(/\/$/, '');
  return `nebi import ${host}/${repo}:${tag}`;
}
