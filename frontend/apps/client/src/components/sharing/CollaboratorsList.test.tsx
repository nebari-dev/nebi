import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { Collaborator } from '@/types/models';
import { CollaboratorsList } from './CollaboratorsList';

const owner: Collaborator = {
  kind: 'user',
  user_id: 'u-1',
  username: 'alice',
  email: 'alice@example.com',
  role: 'owner',
  is_owner: true,
};

const groupViewer: Collaborator = {
  kind: 'group',
  group_id: 'g-1',
  name: 'admin',
  source: 'oidc',
  role: 'viewer',
  is_owner: false,
};

describe('CollaboratorsList', () => {
  it('renders user collaborators', () => {
    render(<CollaboratorsList collaborators={[owner]} />);
    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.getByText('alice@example.com')).toBeInTheDocument();
    expect(screen.getByText('Owner')).toBeInTheDocument();
  });

  it('renders group collaborators alongside users', () => {
    render(<CollaboratorsList collaborators={[owner, groupViewer]} />);
    // The group name and its source label must be visible.
    expect(screen.getByText('admin')).toBeInTheDocument();
    expect(screen.getByText('OIDC group')).toBeInTheDocument();
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('labels native groups distinctly from OIDC groups', () => {
    render(
      <CollaboratorsList
        collaborators={[{ ...groupViewer, source: 'native', name: 'devs' }]}
      />,
    );
    expect(screen.getByText('devs')).toBeInTheDocument();
    expect(screen.getByText('Native group')).toBeInTheDocument();
  });

  it('shows an empty state when there are no collaborators', () => {
    render(<CollaboratorsList collaborators={[]} />);
    expect(screen.getByText('No collaborators yet')).toBeInTheDocument();
  });
});
