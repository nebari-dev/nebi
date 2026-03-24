import { describe, it, expect } from 'vitest';
import { screen } from '@testing-library/react';
import { RoleBadge } from './RoleBadge';
import { renderWithProviders } from '@/test/utils';

describe('RoleBadge', () => {
  it('renders "Owner" for owner role', () => {
    renderWithProviders(<RoleBadge role="owner" />);
    expect(screen.getByText('Owner')).toBeInTheDocument();
  });

  it('renders "Editor" for editor role', () => {
    renderWithProviders(<RoleBadge role="editor" />);
    expect(screen.getByText('Editor')).toBeInTheDocument();
  });

  it('renders "Viewer" for viewer role', () => {
    renderWithProviders(<RoleBadge role="viewer" />);
    expect(screen.getByText('Viewer')).toBeInTheDocument();
  });

  it('applies purple styling for owner', () => {
    renderWithProviders(<RoleBadge role="owner" />);
    const badge = screen.getByText('Owner').closest('[class]');
    expect(badge?.className).toContain('purple');
  });

  it('applies blue styling for editor', () => {
    renderWithProviders(<RoleBadge role="editor" />);
    const badge = screen.getByText('Editor').closest('[class]');
    expect(badge?.className).toContain('blue');
  });

  it('applies gray styling for viewer', () => {
    renderWithProviders(<RoleBadge role="viewer" />);
    const badge = screen.getByText('Viewer').closest('[class]');
    expect(badge?.className).toContain('gray');
  });
});
