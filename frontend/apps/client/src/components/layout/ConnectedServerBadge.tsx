// Owner: U5 (remote). Header badge linking to the Server page when a
// server is connected (prototype: outline badge with the server URL).
import { Badge } from '@nebi/ui/components/ui/badge';
import { Server } from 'lucide-react';
import { Link } from 'react-router-dom';
import { displayServerUrl } from '@/components/server/format';
import { useServerConnection } from '@/hooks/client/remote';

export const ConnectedServerBadge = () => {
  const { data } = useServerConnection();
  if (!data?.connected) return null;
  const shown = displayServerUrl(data.url);
  return (
    <Badge
      variant="outline"
      className="border-border font-medium text-foreground"
      render={
        <Link
          to="/server"
          aria-label={`Connected to ${data.url}. Open the Server page`}
          title={`Signed in as ${data.username}`}
        />
      }
    >
      <Server aria-hidden />
      {shown}
    </Badge>
  );
};
