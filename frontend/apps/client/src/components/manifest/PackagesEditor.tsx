// Owner: U1 (projects). The "Packages" part of the GUI form (prototype):
// an add row (name, version operator, version, "Add package") above a table
// of requested packages whose operator and version can be edited in place.
import { Badge } from '@nebi/ui/components/ui/badge';
import { Button } from '@nebi/ui/components/ui/button';
import { Field, FieldLabel } from '@nebi/ui/components/ui/field';
import { Input } from '@nebi/ui/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@nebi/ui/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@nebi/ui/components/ui/table';
import { toast } from '@nebi/ui/components/ui/toast';
import { Plus, Trash } from 'lucide-react';
import { type KeyboardEvent, useEffect, useRef, useState } from 'react';
import { type RequestedPackage, VERSION_OPERATORS } from '@/types/contracts';
import {
  joinConstraint,
  operatorLabel,
  packageNameError,
  splitConstraint,
} from './rules';

const NAME_COL = 'w-[200px]';
const ACTIONS_COL = 'w-[138px]';

const OperatorItems = () =>
  VERSION_OPERATORS.map(([op, label]) => (
    <SelectItem key={op} value={op}>
      <span className={op === 'none' ? 'shrink-0' : 'w-6 shrink-0 font-mono'}>
        {operatorLabel(op)}
      </span>
      <span className="text-muted-foreground">{label}</span>
    </SelectItem>
  ));

const OperatorSelect = ({
  value,
  onChange,
  label,
  disabled,
}: {
  value: string;
  onChange: (op: string) => void;
  label: string;
  disabled?: boolean;
}) => (
  <Select
    value={value}
    onValueChange={(op) => op && onChange(op as string)}
    disabled={disabled}
  >
    <SelectTrigger className="w-20 shrink-0" aria-label={label}>
      <SelectValue>{(op: string) => operatorLabel(op)}</SelectValue>
    </SelectTrigger>
    <SelectContent align="start" className="min-w-56">
      <OperatorItems />
    </SelectContent>
  </Select>
);

export interface PackagesEditorProps {
  requested: RequestedPackage[];
  onChange: (requested: RequestedPackage[]) => void;
  /** Id prefix for labels. */
  idPrefix?: string;
  emptyText?: string;
  error?: string | null;
  disabled?: boolean;
}

const samePackage = (a: RequestedPackage, b: RequestedPackage) =>
  a.name === b.name && (a.source ?? 'conda') === (b.source ?? 'conda');

export const PackagesEditor = ({
  requested,
  onChange,
  idPrefix = 'packages',
  emptyText = 'No packages yet. You can still create an empty project.',
  error,
  disabled,
}: PackagesEditorProps) => {
  const [name, setName] = useState('');
  const [op, setOp] = useState<string>('>=');
  const [version, setVersion] = useState('');
  const [addError, setAddError] = useState<string | null>(null);
  const nameRef = useRef<HTMLInputElement>(null);

  const add = () => {
    const n = name.trim().toLowerCase();
    const problem = packageNameError(n);
    if (problem) {
      setAddError(problem);
      nameRef.current?.focus();
      return;
    }
    const constraint = joinConstraint(op, version);
    const next: RequestedPackage = { name: n, constraint };
    const exists = requested.some((p) => samePackage(p, next));
    onChange(
      exists
        ? requested.map((p) =>
            samePackage(p, next) ? { ...p, constraint } : p,
          )
        : [...requested, next],
    );
    if (exists) {
      toast.add({
        title: `Updated ${n}`,
        description: `Version constraint is now ${constraint}.`,
      });
    }
    setName('');
    setVersion('');
    setAddError(null);
    nameRef.current?.focus();
  };
  const onEnter = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      add();
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <div className="flex items-end gap-2 max-[640px]:flex-wrap">
          <Field invalid={!!addError} className={`${NAME_COL} shrink-0 gap-1`}>
            <FieldLabel className="text-foreground">Package name</FieldLabel>
            <Input
              ref={nameRef}
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                setAddError(null);
              }}
              onKeyDown={onEnter}
              placeholder="Package name"
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
            />
          </Field>
          <Field className="min-w-0 flex-1 gap-1">
            <FieldLabel id={`${idPrefix}-version`} className="text-foreground">
              Version
            </FieldLabel>
            <div className="flex gap-2">
              <OperatorSelect
                value={op}
                onChange={setOp}
                label="Version operator"
                disabled={disabled}
              />
              <Input
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                onKeyDown={onEnter}
                placeholder="e.g. 1.0"
                aria-labelledby={`${idPrefix}-version`}
                autoComplete="off"
                spellCheck={false}
                className="min-w-0 flex-1"
                disabled={disabled}
              />
            </div>
          </Field>
          <Button
            type="button"
            variant="secondary"
            size="lg"
            className="h-8 w-[130px] shrink-0"
            onClick={add}
            disabled={disabled}
          >
            <Plus />
            Add package
          </Button>
        </div>
        {(addError || error) && (
          <p className="text-destructive-foreground text-xs" role="alert">
            {addError ?? error}
          </p>
        )}
      </div>
      <Table aria-labelledby={`${idPrefix}-heading`}>
        <TableHeader>
          <TableRow>
            <TableHead className={NAME_COL}>Name</TableHead>
            <TableHead>Version constraint</TableHead>
            <TableHead className={ACTIONS_COL}>
              <span className="sr-only">Actions</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {requested.map((pkg) => (
            <PackageRow
              key={`${pkg.source ?? 'conda'}:${pkg.name}`}
              pkg={pkg}
              disabled={disabled}
              onChange={(constraint) =>
                onChange(
                  requested.map((p) =>
                    samePackage(p, pkg) ? { ...p, constraint } : p,
                  ),
                )
              }
              onRemove={() =>
                onChange(requested.filter((p) => !samePackage(p, pkg)))
              }
            />
          ))}
          {requested.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={3}
                className="h-16 text-center text-muted-foreground"
              >
                {emptyText}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
};

const PackageRow = ({
  pkg,
  onChange,
  onRemove,
  disabled,
}: {
  pkg: RequestedPackage;
  onChange: (constraint: string) => void;
  onRemove: () => void;
  disabled?: boolean;
}) => {
  const [op, version] = splitConstraint(pkg.constraint);
  const [draft, setDraft] = useState(version);
  useEffect(() => setDraft(version), [version]);
  const commit = () => {
    const next = joinConstraint(op, draft);
    if (next !== pkg.constraint) onChange(next);
    setDraft(splitConstraint(next)[1]);
  };
  return (
    <TableRow>
      <TableCell className="h-10 py-2">
        <span className="flex items-center gap-1.5">
          {pkg.name}
          {pkg.source === 'pypi' && <Badge variant="outline">PyPI</Badge>}
        </span>
      </TableCell>
      <TableCell className="h-10 py-1 pr-0 pl-2">
        <div className="flex items-center gap-2">
          <OperatorSelect
            value={op}
            onChange={(next) => onChange(joinConstraint(next, draft))}
            label={`Version operator for ${pkg.name}`}
            disabled={disabled}
          />
          <Input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commit();
              }
            }}
            placeholder="any"
            aria-label={`Version for ${pkg.name}`}
            autoComplete="off"
            spellCheck={false}
            className="min-w-0 flex-1"
            disabled={disabled}
          />
        </div>
      </TableCell>
      <TableCell className="h-10 py-1 text-center">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`Remove ${pkg.name}`}
          onClick={onRemove}
          disabled={disabled}
        >
          <Trash />
        </Button>
      </TableCell>
    </TableRow>
  );
};
