// Owner: U5 (remote). Used by the Server page and the projects list empty
// state ("Connect to server"). Props are a cross-unit contract
// (CONTRACTS.md); `mode` is an optional addition: "connect" (default,
// prototype ConnectServer) or "manage" (prototype ManageConnection, with
// the "Disconnect from this server" section).
import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import { Tabs, TabsList, TabsTab } from '@nebi/ui/components/ui/tabs';
import { toast } from '@nebi/ui/components/ui/toast';
import { type FormEvent, useState } from 'react';
import { clientApiError } from '@/api/client/base';
import { useConnectServer, useServerConnection } from '@/hooks/client/remote';
import type { ServerAuth, ServerConnection } from '@/types/contracts';
import { DISCONNECT_COPY, MASKED_TOKEN } from './copy';
import { DisconnectDialog } from './DisconnectDialog';
import { displayServerUrl, serverUrlError } from './format';
import { TextField } from './TextField';

export interface ConnectServerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnected?: (connection: ServerConnection) => void;
  /** "connect" (default) or "manage" the current connection. */
  mode?: 'connect' | 'manage';
}

interface FieldErrors {
  url?: string;
  token?: string;
  form?: string;
}

export const ConnectServerDialog = ({
  open,
  onOpenChange,
  onConnected,
  mode = 'connect',
}: ConnectServerDialogProps) => {
  const { data: current } = useServerConnection();
  const [confirmDisconnect, setConfirmDisconnect] = useState(false);
  return (
    <>
      <Dialog open={open && !confirmDisconnect} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-[520px]">
          {open && (
            <ConnectForm
              mode={mode}
              current={current}
              onDone={(conn) => {
                onOpenChange(false);
                onConnected?.(conn);
              }}
              onDisconnect={() => setConfirmDisconnect(true)}
            />
          )}
        </DialogContent>
      </Dialog>
      {mode === 'manage' && current?.connected && (
        <DisconnectDialog
          url={current.url}
          open={open && confirmDisconnect}
          onOpenChange={(next) => {
            if (!next) setConfirmDisconnect(false);
          }}
          onDisconnected={() => {
            setConfirmDisconnect(false);
            onOpenChange(false);
          }}
        />
      )}
    </>
  );
};

const ConnectForm = ({
  mode,
  current,
  onDone,
  onDisconnect,
}: {
  mode: 'connect' | 'manage';
  current: ServerConnection | undefined;
  onDone: (conn: ServerConnection) => void;
  onDisconnect: () => void;
}) => {
  const managing = mode === 'manage' && !!current?.connected;
  const [url, setUrl] = useState(
    managing && current ? displayServerUrl(current.url) : '',
  );
  const [auth, setAuth] = useState<ServerAuth>(
    managing && current ? current.auth : 'token',
  );
  const [token, setToken] = useState(
    managing && current?.auth === 'token' ? MASKED_TOKEN : '',
  );
  const [errors, setErrors] = useState<FieldErrors>({});
  const connect = useConnectServer();

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const next: FieldErrors = {
      url: serverUrlError(url),
      token:
        auth === 'token' && !token.trim()
          ? 'Paste a token, or choose None.'
          : undefined,
    };
    setErrors(next);
    if (next.url || next.token) return;
    const trimmed = url.trim();
    // Keep the scheme the user typed ("http://localhost:8460"); the
    // backend defaults to https when there is none.
    const sameAsStored =
      managing &&
      current &&
      displayServerUrl(current.url) === displayServerUrl(trimmed);
    connect.mutate(
      {
        url: sameAsStored && current ? current.url : trimmed,
        auth,
        token: auth === 'token' && token !== MASKED_TOKEN ? token.trim() : '',
      },
      {
        onSuccess: (conn) => {
          if (managing) {
            toast.add({ type: 'success', title: 'Connection updated' });
          } else {
            toast.add({
              type: 'success',
              title: `Connected to ${displayServerUrl(conn.url)}`,
              description:
                'Projects shared with you are listed on the Server page.',
            });
          }
          onDone(conn);
        },
        onError: (err) => {
          const body = clientApiError(err);
          if (body?.field === 'url' || body?.field === 'token') {
            setErrors({ [body.field]: body.error });
          } else {
            setErrors({
              form: body?.error ?? 'Couldn’t connect. Try again.',
            });
          }
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="grid gap-4" noValidate>
      <DialogHeader>
        <DialogTitle>
          {managing ? 'Manage connection' : 'Connect server'}
        </DialogTitle>
        <DialogDescription>
          You can connect to one server at a time.
        </DialogDescription>
      </DialogHeader>
      <TextField
        label="Server URL"
        error={errors.url}
        value={url}
        onChange={(e) => {
          setUrl(e.target.value);
          setErrors((prev) => ({ ...prev, url: undefined, form: undefined }));
        }}
        placeholder="nebi.example.com"
        autoFocus={!managing}
        spellCheck={false}
        autoComplete="url"
      />
      <div className="grid gap-1">
        <span id="auth-label" className="font-medium text-foreground text-sm">
          Authentication
        </span>
        <Tabs
          value={auth}
          onValueChange={(value) => {
            setAuth(value as ServerAuth);
            setErrors((prev) => ({ ...prev, token: undefined }));
          }}
        >
          <TabsList
            variant="toggle"
            aria-labelledby="auth-label"
            className="w-full"
          >
            <TabsTab value="none" className="flex-1">
              None
            </TabsTab>
            <TabsTab value="token" className="flex-1">
              Token
            </TabsTab>
          </TabsList>
        </Tabs>
      </div>
      {auth === 'token' && (
        <TextField
          label="Token"
          type="password"
          error={errors.token}
          value={token}
          onChange={(e) => {
            setToken(e.target.value);
            setErrors((prev) => ({
              ...prev,
              token: undefined,
              form: undefined,
            }));
          }}
          onFocus={(e) => {
            if (managing && token === MASKED_TOKEN) e.currentTarget.select();
          }}
          autoComplete="off"
        />
      )}
      {errors.form && (
        <p role="alert" className="text-destructive-foreground text-sm">
          {errors.form}
        </p>
      )}
      {managing && (
        <section
          className="grid gap-2 border-border border-t pt-4"
          aria-labelledby="disconnect-title"
        >
          <h3
            id="disconnect-title"
            className="font-medium text-foreground text-sm"
          >
            Disconnect from this server
          </h3>
          <p className="text-muted-foreground text-sm">{DISCONNECT_COPY}</p>
          <Button
            variant="destructive"
            size="sm"
            className="w-fit"
            onClick={onDisconnect}
          >
            Disconnect
          </Button>
        </section>
      )}
      <DialogFooter>
        <DialogClose render={<Button variant="secondary" />}>
          Cancel
        </DialogClose>
        <Button
          render={<button type="submit" />}
          loading={connect.isPending}
          loadingText="Connecting…"
        >
          {managing ? 'Save changes' : 'Connect'}
        </Button>
      </DialogFooter>
    </form>
  );
};
