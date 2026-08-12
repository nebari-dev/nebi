import { AlertTriangle } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';

// Shown when a configured remote server stops responding (see issue #217).
export const RemoteUnreachableBanner = () => (
  <Alert className="border-amber-500/20 bg-amber-500/10 text-amber-600 [&>svg]:text-amber-600">
    <AlertTriangle className="h-4 w-4" />
    <AlertDescription>
      Remote server unreachable. Check that the server is running, or disconnect
      from it in Settings.
    </AlertDescription>
  </Alert>
);
