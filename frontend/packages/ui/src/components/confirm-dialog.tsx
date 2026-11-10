import { Button } from '@nebi/ui/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@nebi/ui/components/ui/dialog';
import type { ReactNode } from 'react';

// Local-only: the @nebari registry does not publish confirm-dialog.
// Keep this app-specific wrapper on top of the registry Dialog composition.

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  title: ReactNode;
  description?: ReactNode;
  confirmText?: ReactNode;
  cancelText?: ReactNode;
  variant?: 'default' | 'destructive';
  /** Extra body content rendered between the header and the buttons. */
  children?: ReactNode;
  /** Shows a spinner on the confirm button and keeps the dialog open. */
  confirmLoading?: boolean;
  /** Keep the dialog open after confirming (caller closes it). */
  keepOpenOnConfirm?: boolean;
  className?: string;
}

export const ConfirmDialog = ({
  open,
  onOpenChange,
  onConfirm,
  title,
  description,
  confirmText = 'Continue',
  cancelText = 'Cancel',
  variant = 'default',
  children,
  confirmLoading = false,
  keepOpenOnConfirm = false,
  className,
}: ConfirmDialogProps) => {
  const handleConfirm = () => {
    onConfirm();
    if (!keepOpenOnConfirm) onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} disablePointerDismissal>
      <DialogContent
        role="alertdialog"
        showCloseButton={false}
        className={className}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description !== undefined && (
            <DialogDescription>{description}</DialogDescription>
          )}
        </DialogHeader>
        {children}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {cancelText}
          </Button>
          <Button
            variant={variant === 'destructive' ? 'destructive' : 'default'}
            onClick={handleConfirm}
            loading={confirmLoading}
          >
            {confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
