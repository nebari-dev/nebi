import * as React from 'react';
import { X } from 'lucide-react';
import { Button } from './button';

interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: React.ReactNode;
}

export const Dialog = ({ open, onOpenChange, children }: DialogProps) => {
  const childrenArray = React.Children.toArray(children);
  const trigger = childrenArray.find(
    (child) => React.isValidElement(child) && child.type === DialogTrigger
  );
  const content = childrenArray.filter(
    (child) => React.isValidElement(child) && child.type !== DialogTrigger
  );

  return (
    <>
      {trigger}
      {open && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="fixed inset-0 bg-black/50"
            onClick={() => onOpenChange(false)}
          />
          <div className="relative bg-card border rounded-lg shadow-lg max-w-2xl w-full max-h-[90vh] overflow-auto m-4">
            {content}
          </div>
        </div>
      )}
    </>
  );
};

interface DialogContentProps {
  children: React.ReactNode;
  className?: string;
}

export const DialogContent = ({ children, className = '' }: DialogContentProps) => {
  return <div className={`p-6 ${className}`}>{children}</div>;
};

interface DialogHeaderProps {
  children: React.ReactNode;
}

export const DialogHeader = ({ children }: DialogHeaderProps) => {
  return <div className="mb-4">{children}</div>;
};

interface DialogTitleProps {
  children: React.ReactNode;
}

export const DialogTitle = ({ children }: DialogTitleProps) => {
  return <h2 className="text-lg font-semibold">{children}</h2>;
};

interface DialogDescriptionProps {
  children: React.ReactNode;
}

export const DialogDescription = ({ children }: DialogDescriptionProps) => {
  return <p className="text-sm text-muted-foreground mt-2">{children}</p>;
};

interface DialogTriggerProps {
  asChild?: boolean;
  children: React.ReactNode;
}

export const DialogTrigger = ({ children }: DialogTriggerProps) => {
  return <>{children}</>;
};

interface DialogCloseProps {
  onClick?: () => void;
}

export const DialogClose = ({ onClick }: DialogCloseProps) => {
  return (
    <Button variant="ghost" size="icon" onClick={onClick} className="absolute right-4 top-4">
      <X className="h-4 w-4" />
    </Button>
  );
};
