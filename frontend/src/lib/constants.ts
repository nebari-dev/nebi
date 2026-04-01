import type { BadgeVariant } from '@/components/ui/badge';

export const workspaceStatusVariant: Record<string, BadgeVariant> = {
  pending:  'warning',
  creating: 'info',
  ready:    'success',
  failed:   'error',
  deleting: 'warning',
};

export const jobStatusVariant: Record<string, BadgeVariant> = {
  pending:   'warning',
  running:   'info',
  completed: 'success',
  failed:    'error',
};

export const logActionVariant: Record<string, BadgeVariant> = {
  create_user: 'success',
  delete_user: 'error',
  grant_permission: 'info',
  revoke_permission: 'warning',
  make_admin: 'default',
  revoke_admin: 'error',
  create_workspace: 'success',
  delete_workspace: 'error',
  share_workspace: 'info',
  unshare_workspace: 'warning',
  create_package: 'success',
  install_package: 'success',
  remove_package: 'error',
  update_package: 'default',
};