// Owner: U5 (remote). Header badge linking to the Server page when a
// server is connected (prototype: outline badge with the server URL).
import { Badge } from '@nebi/ui/components/ui/badge';
import { Server } from 'lucide-react';
import { Link } from 'react-router-dom';
import { useServerConnection } from '@/hooks/client/remote';

export const ConnectedServerBadge = () => {
  const { data } = useServerConnection();
  if (!data?.connected) return null;
  return (
    <Badge
      variant="outline"
      className="border-border font-medium text-foreground"
      render={
        <Link
          to="/server"
          aria-label={`Connected to ${data.url}. Open the Server page`}
        />
      }
    >
      <Server aria-hidden />
      {data.url}
    </Badge>
  );
};
