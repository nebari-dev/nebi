// Owner: U1 (projects). Client-side mirrors of internal/artifact rules and
// copy (validate.go, constraint.go, platform.go) so forms can validate as
// the user types. The server validates again; its messages are the same.
import { VERSION_OPERATORS } from '@/types/contracts';

export const PROJECT_NAME_PATTERN = /^[A-Za-z0-9_.-]+$/;
const PACKAGE_NAME_PATTERN = /^[A-Za-z0-9_.-]+$/;
const TAG_PATTERN = /^[A-Za-z][A-Za-z0-9._-]*$/;
/** Looks like a pixi platform name (e.g. linux-ppc64le). */
const PLATFORM_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)+$/;

export const NAME_MESSAGES = {
  required: 'Enter a project name.',
  invalid: 'Use letters, numbers, hyphens, underscores or dots.',
  taken: 'A project with that name already exists.',
} as const;

/** Project name error, or null (strict rule for new projects). */
export function projectNameError(
  name: string,
  existingNames: string[] = [],
): string | null {
  const n = name.trim();
  if (!n) return NAME_MESSAGES.required;
  if (!PROJECT_NAME_PATTERN.test(n) || n === '.' || n === '..') {
    return NAME_MESSAGES.invalid;
  }
  const lower = n.toLowerCase();
  if (existingNames.some((e) => e.toLowerCase() === lower)) {
    return NAME_MESSAGES.taken;
  }
  return null;
}

export function packageNameError(name: string): string | null {
  const n = name.trim();
  if (!n) return 'Enter a package name.';
  if (!PACKAGE_NAME_PATTERN.test(n)) {
    return 'Package names use letters, numbers, hyphens, underscores or dots.';
  }
  return null;
}

/** Tag error, or null. */
export function tagError(tag: string): string | null {
  if (TAG_PATTERN.test(tag)) return null;
  return /^[A-Za-z]/.test(tag)
    ? `“${tag}” can’t be used: tags use letters, numbers, dots, hyphens and underscores.`
    : `“${tag}” can’t be used: tags must start with a letter.`;
}

export const looksLikePlatform = (p: string) => PLATFORM_PATTERN.test(p);

/** Common platforms offered in the picker (prototype order). */
export const PLATFORM_LABELS: Record<string, string> = {
  'linux-64': 'Linux',
  'linux-aarch64': 'Linux ARM',
  'osx-64': 'macOS Intel',
  'osx-arm64': 'macOS Apple silicon',
  'win-64': 'Windows',
};

/** "osx-arm64 · this machine", "linux-64 · Linux" or the bare name. */
export function platformDisplayName(platform: string, machine?: string) {
  if (platform === machine) return `${platform} · this machine`;
  const label = PLATFORM_LABELS[platform];
  return label ? `${platform} · ${label}` : platform;
}

/**
 * This machine's pixi platform from GET /client/capabilities `platform`
 * (GOOS/GOARCH), falling back to the browser's user agent.
 */
export function machinePlatform(goPlatform?: string): string {
  const map: Record<string, string> = {
    'linux/amd64': 'linux-64',
    'linux/arm64': 'linux-aarch64',
    'linux/ppc64le': 'linux-ppc64le',
    'darwin/amd64': 'osx-64',
    'darwin/arm64': 'osx-arm64',
    'windows/amd64': 'win-64',
    'windows/arm64': 'win-arm64',
  };
  if (goPlatform && map[goPlatform]) return map[goPlatform];
  const ua = typeof navigator === 'undefined' ? '' : navigator.userAgent;
  if (/Windows/i.test(ua)) return 'win-64';
  if (/Mac/i.test(ua)) return 'osx-arm64';
  if (/aarch64|arm64/i.test(ua)) return 'linux-aarch64';
  return 'linux-64';
}

export type VersionOperator = (typeof VERSION_OPERATORS)[number][0];

/** "None" for the as-written operator, else the operator itself. */
export const operatorLabel = (op: string) => (op === 'none' ? 'None' : op);

/** Joins operator and version into a constraint ("*" when blank). */
export function joinConstraint(op: string, version: string): string {
  const v = version.trim();
  if (!v) return '*';
  return /^[<>=!~]/.test(v) || op === 'none' ? v : `${op}${v}`;
}

/** Splits a constraint into the form's operator and version. */
export function splitConstraint(constraint: string): [VersionOperator, string] {
  if (!constraint || constraint === '*') return ['none', ''];
  const op = VERSION_OPERATORS.find(
    ([o]) => o !== 'none' && constraint.startsWith(o),
  )?.[0];
  return op ? [op, constraint.slice(op.length)] : ['none', constraint];
}
