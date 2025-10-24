import { Badge } from '@/components/ui/badge';
import { Crown, Edit, Eye } from 'lucide-react';

interface RoleBadgeProps {
  role: 'owner' | 'editor' | 'viewer';
}

export const RoleBadge = ({ role }: RoleBadgeProps) => {
  const configs = {
    owner: {
      className: 'bg-purple-500/10 text-purple-500 border-purple-500/20',
      icon: Crown,
      label: 'Owner',
    },
    editor: {
      className: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
      icon: Edit,
      label: 'Editor',
    },
    viewer: {
      className: 'bg-gray-500/10 text-gray-500 border-gray-500/20',
      icon: Eye,
      label: 'Viewer',
    },
  };

  const config = configs[role];
  const Icon = config.icon;

  return (
    <Badge className={config.className}>
      <Icon className="h-3 w-3 mr-1" />
      {config.label}
    </Badge>
  );
};
