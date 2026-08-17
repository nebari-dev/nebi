import { AlertTriangle } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';

// Shown when requests to a configured remote server fail (see issue #217).
// The backend wraps every remote failure as a 502, including remote-side auth
// errors, so the copy covers an expired session as well as a down server.
export const RemoteUnreachableBanner = () => (
  <Alert className="border-amber-500/20 bg-amber-500/10 text-amber-600 [&>svg]:text-amber-600">
    <AlertTriangle className="h-4 w-4" />
    <AlertDescription>
      Can't reach the remote server. Check that it is running and that your
      connection in Settings is still valid, or disconnect from it there.
    </AlertDescription>
  </Alert>
);
