import { describe, it, expect } from 'vitest';
import { buildImportCommand } from './registry';

describe('buildImportCommand', () => {
  it('builds correct command from registry URL with namespace in repo', () => {
    const cmd = buildImportCommand(
      'https://quay.io',
      'nebari_environments/data-science-demo',
      '0.1.0'
    );
    expect(cmd).toBe('nebi import quay.io/nebari_environments/data-science-demo:0.1.0');
  });

  it('does not double the namespace', () => {
    const cmd = buildImportCommand(
      'https://quay.io',
      'nebari_environments/data-science-demo',
      'latest'
    );
    expect(cmd).not.toContain('nebari_environments/nebari_environments');
  });

  it('handles registry URL without protocol', () => {
    const cmd = buildImportCommand('ghcr.io', 'myorg/my-env', 'v1');
    expect(cmd).toBe('nebi import ghcr.io/myorg/my-env:v1');
  });

  it('strips trailing slash from registry URL', () => {
    const cmd = buildImportCommand('https://quay.io/', 'org/repo', 'latest');
    expect(cmd).toBe('nebi import quay.io/org/repo:latest');
  });

  it('handles deeply nested repo paths', () => {
    const cmd = buildImportCommand('https://quay.io', 'org/sub/repo', '1.0');
    expect(cmd).toBe('nebi import quay.io/org/sub/repo:1.0');
  });

  it('works with pre-combined namespace/repo (publication record style)', () => {
    // Simulates WorkspaceDetail where namespace and repo are combined before calling
    const namespace = 'nebari_environments';
    const repository = 'data-science-demo';
    const repo = namespace ? `${namespace}/${repository}` : repository;
    const cmd = buildImportCommand('https://quay.io', repo, 'v1.0.0');
    expect(cmd).toBe('nebi import quay.io/nebari_environments/data-science-demo:v1.0.0');
  });

  it('works with bare repo when no namespace', () => {
    const cmd = buildImportCommand('https://ghcr.io', 'my-env', 'latest');
    expect(cmd).toBe('nebi import ghcr.io/my-env:latest');
  });
});
