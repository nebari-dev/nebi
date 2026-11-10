// Owner: U4 (compare). "Base" / "Compare to" version select.
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@nebi/ui/components/ui/select';

export const VersionPicker = ({
  label,
  value,
  versions,
  format,
  onChange,
}: {
  label: string;
  value: number;
  versions: number[];
  format: (n: number) => string;
  onChange: (n: number) => void;
}) => (
  // biome-ignore lint/a11y/noLabelWithoutControl: the Base UI trigger inside is the labelled control (it also carries aria-label).
  <label className="flex items-center gap-3">
    <span className="text-muted-foreground text-xs leading-4">{label}</span>
    <Select<number>
      value={value}
      onValueChange={(n) => {
        if (n != null) onChange(n);
      }}
    >
      <SelectTrigger
        aria-label={label}
        className="min-h-7 w-auto bg-background py-1 font-medium text-xs"
      >
        <SelectValue>{(n: number) => format(n)}</SelectValue>
      </SelectTrigger>
      <SelectContent align="start" className="min-w-48">
        {versions.map((n) => (
          <SelectItem key={n} value={n}>
            {format(n)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  </label>
);
