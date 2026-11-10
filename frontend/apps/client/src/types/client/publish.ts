// Owner: the publish unit. Unit-specific frontend types; the API contract types
// live in types/contracts.ts (frozen).

/**
 * PUT /client/registries/:registryId. Omitted fields stay unchanged;
 * password "" removes the stored password.
 */
export interface UpdateRegistryRequest {
  name?: string;
  url?: string;
  namespace?: string;
  username?: string;
  password?: string;
  is_default?: boolean;
}

/** A repository:tag pair inside a registry. */
export interface RepositoryTag {
  repository: string;
  tag: string;
}

/** Validation copy for a malformed repository:tag (prototype). */
export const REFERENCE_HINT = 'Use repository:tag, e.g. nebari/project_1:5.';

const REFERENCE_RE =
  /^([a-z0-9][a-z0-9._/-]*):([A-Za-z0-9_][A-Za-z0-9._-]{0,127})$/;

/** Parses "repository:tag" (the publish dialog's editable text). */
export function parseReference(text: string): RepositoryTag | null {
  const m = text.trim().match(REFERENCE_RE);
  if (!m) return null;
  const repository = m[1];
  if (repository.endsWith('/') || repository.includes('//')) return null;
  return { repository, tag: m[2] };
}

/** The registry host of a registry URL (no scheme, no namespace path). */
export function registryHost(url: string): string {
  return url
    .replace(/^https?:\/\//, '')
    .replace(/\/.*$/, '')
    .trim();
}

/** registry/repository:tag */
export function formatReference(host: string, ref: RepositoryTag): string {
  return `${host}/${ref.repository}:${ref.tag}`;
}

/** "a", "a and b", "a, b and c" (prototype list wording). */
export function joinNames(names: string[]): string {
  if (names.length <= 2) return names.join(' and ');
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
}
