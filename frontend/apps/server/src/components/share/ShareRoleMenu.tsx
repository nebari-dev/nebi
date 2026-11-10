// Role menu of the share dialog (prototype
// ShareRoleMenu): Can view / Can edit with a check on the current role, and
// "Remove access" for an existing entry. Kept identical in apps/client.
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@nebi/ui/components/ui/dropdown-menu';
import { cn } from '@nebi/ui/lib/utils';
import { ChevronDown } from 'lucide-react';
import { GRANT_ROLES, type GrantRole, ROLE_LABELS } from './shareModel';

export interface ShareRoleMenuProps {
  value: GrantRole;
  onChange: (role: GrantRole) => void;
  /** Adds "Remove access" (existing entries only). */
  onRemove?: () => void;
  variant: 'outline' | 'ghost';
  /** Accessible name prefix, e.g. "Access for Alex Kim". */
  label: string;
  disabled?: boolean;
}

export const ShareRoleMenu = ({
  value,
  onChange,
  onRemove,
  variant,
  label,
  disabled,
}: ShareRoleMenuProps) => (
  <DropdownMenu modal={false}>
    <DropdownMenuTrigger
      variant={variant}
      disabled={disabled}
      aria-label={`${label}: ${ROLE_LABELS[value]}`}
      className={cn(
        'shrink-0 justify-between gap-2',
        variant === 'ghost' && 'h-7 px-2.5 text-xs',
      )}
    >
      {ROLE_LABELS[value]}
      <ChevronDown aria-hidden />
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent align="end" className="min-w-40">
        {GRANT_ROLES.map((role) => (
          <DropdownMenuCheckboxItem
            key={role}
            checked={value === role}
            onCheckedChange={() => {
              if (role !== value) onChange(role);
            }}
          >
            {ROLE_LABELS[role]}
          </DropdownMenuCheckboxItem>
        ))}
        {onRemove && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={onRemove}>
              Remove access
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenu>
);
