import type { Collaborator } from '@/types/models';
import { RoleBadge } from './RoleBadge';

interface CollaboratorsListProps {
  collaborators: Collaborator[];
}

/**
 * Read-only list of everyone with access to a workspace: both individual users
 * and shared groups. The Share dialog renders the same data with extra controls
 * (remove buttons); this view is display-only for the Collaborators tab.
 */
export const CollaboratorsList = ({
  collaborators,
}: CollaboratorsListProps) => {
  if (collaborators.length === 0) {
    return (
      <p className="text-sm text-muted-foreground text-center py-8">
        No collaborators yet
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {collaborators.map((collab) =>
        collab.kind === 'user' ? (
          <div
            key={`u-${collab.user_id}`}
            className="flex justify-between items-center p-3 rounded-lg border"
          >
            <div className="flex-1">
              <div className="font-medium">{collab.username}</div>
              <div className="text-sm text-muted-foreground">
                {collab.email}
              </div>
            </div>
            <RoleBadge role={collab.role} />
          </div>
        ) : (
          <div
            key={`g-${collab.group_id}`}
            className="flex justify-between items-center p-3 rounded-lg border"
          >
            <div className="flex-1">
              <div className="font-medium">{collab.name}</div>
              <div className="text-sm text-muted-foreground">
                {collab.source === 'oidc' ? 'OIDC group' : 'Native group'}
              </div>
            </div>
            <RoleBadge role={collab.role} />
          </div>
        ),
      )}
    </div>
  );
};
