import { FileCode, Loader2, Plus, Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';

interface Package {
  name: string;
  version: string;
}

interface PixiTomlEditorProps {
  tomlValue: string;
  onTomlChange: (toml: string) => void;
  workspaceName?: string;
  onReloadToml?: () => Promise<string>;
}

const buildPixiToml = (packages: Package[], wsName: string): string => {
  const dependenciesLines = packages
    .filter((pkg) => pkg.name.trim())
    .map((pkg) => {
      if (pkg.version.trim()) {
        return `${pkg.name} = "${pkg.version}"`;
      }
      return `${pkg.name} = "*"`;
    })
    .join('\n');

  return `[workspace]
name = "${wsName}"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64", "win-64"]

[dependencies]
${dependenciesLines || 'python = ">=3.11"'}
`;
};

const findSectionRange = (
  lines: string[],
  sectionNames: string[],
): { start: number; end: number } | null => {
  for (let index = 0; index < lines.length; index++) {
    const tableMatch = lines[index].match(/^\s*\[([^[\]]+)\]\s*(?:#.*)?$/);
    if (!tableMatch || !sectionNames.includes(tableMatch[1].trim())) {
      continue;
    }

    let end = lines.length;
    for (let next = index + 1; next < lines.length; next++) {
      if (/^\s*\[+.+\]\s*(?:#.*)?$/.test(lines[next])) {
        end = next;
        break;
      }
    }

    return { start: index, end };
  }

  return null;
};

const parseDependencyLine = (
  line: string,
): {
  indent: string;
  name: string;
  version: string;
  comment: string;
} | null => {
  const match = line.match(/^(\s*)([^\s=#]+)\s*=\s*"([^"]*)"\s*(#.*)?$/);
  if (!match) {
    return null;
  }

  return {
    indent: match[1],
    name: match[2],
    version: match[3],
    comment: match[4] ? ` ${match[4].trimStart()}` : '',
  };
};

const formatDependencyLine = (
  pkg: Package,
  indent = '',
  comment = '',
): string => {
  const name = pkg.name.trim();
  const version = pkg.version.trim() || '*';
  return `${indent}${name} = "${version}"${comment}`;
};

const patchPixiTomlDependencies = (
  toml: string,
  packages: Package[],
): string => {
  const lines = toml.split(/\r?\n/);
  const desiredByName = new Map(packages.map((pkg) => [pkg.name, pkg]));
  const sectionRange = findSectionRange(lines, ['dependencies']);

  if (!sectionRange) {
    if (packages.length === 0) {
      return toml;
    }

    const nextLines = toml ? [...lines] : [];
    if (nextLines.length > 0 && nextLines[nextLines.length - 1] !== '') {
      nextLines.push('');
    }
    nextLines.push(
      '[dependencies]',
      ...packages.map((pkg) => formatDependencyLine(pkg)),
    );
    return nextLines.join('\n');
  }

  const emittedNames = new Set<string>();
  const patchedSectionLines: string[] = [];

  for (const line of lines.slice(sectionRange.start + 1, sectionRange.end)) {
    const dependency = parseDependencyLine(line);
    if (!dependency) {
      patchedSectionLines.push(line);
      continue;
    }

    const desiredPackage = desiredByName.get(dependency.name);
    if (!desiredPackage) {
      continue;
    }

    patchedSectionLines.push(
      formatDependencyLine(
        desiredPackage,
        dependency.indent,
        dependency.comment,
      ),
    );
    emittedNames.add(dependency.name);
  }

  const newDependencyLines = packages
    .filter((pkg) => !emittedNames.has(pkg.name))
    .map((pkg) => formatDependencyLine(pkg));

  if (newDependencyLines.length > 0) {
    let insertAt = patchedSectionLines.length;
    while (insertAt > 0 && patchedSectionLines[insertAt - 1].trim() === '') {
      insertAt--;
    }
    patchedSectionLines.splice(insertAt, 0, ...newDependencyLines);
  }

  return [
    ...lines.slice(0, sectionRange.start + 1),
    ...patchedSectionLines,
    ...lines.slice(sectionRange.end),
  ].join('\n');
};

const patchPixiTomlWorkspaceName = (toml: string, workspaceName: string) => {
  const lines = toml.split(/\r?\n/);
  const sectionRange = findSectionRange(lines, ['workspace', 'project']);
  const nameLine = `name = "${workspaceName}"`;

  if (!sectionRange) {
    return toml;
  }

  const nextLines = [...lines];
  for (let index = sectionRange.start + 1; index < sectionRange.end; index++) {
    const nameMatch = nextLines[index].match(/^(\s*)name\s*=.*?(\s+#.*)?$/);
    if (nameMatch) {
      nextLines[index] = `${nameMatch[1]}${nameLine}${nameMatch[2] ?? ''}`;
      return nextLines.join('\n');
    }
  }

  return toml;
};

const parsePixiTomlDependencies = (toml: string): Package[] => {
  const lines = toml.split(/\r?\n/);
  const packages: Package[] = [];
  const sectionRange = findSectionRange(lines, ['dependencies']);
  if (!sectionRange) {
    return packages;
  }

  for (const line of lines.slice(sectionRange.start + 1, sectionRange.end)) {
    const dependency = parseDependencyLine(line);
    if (dependency) {
      packages.push({
        name: dependency.name,
        version: dependency.version === '*' ? '' : dependency.version,
      });
    }
  }

  return packages;
};

export const PixiTomlEditor = ({
  tomlValue,
  onTomlChange,
  workspaceName,
  onReloadToml,
}: PixiTomlEditorProps) => {
  const [mode, setMode] = useState<'ui' | 'toml'>('toml');
  const [packages, setPackages] = useState<Package[]>([
    { name: 'python', version: '>=3.11' },
  ]);
  const [newPackageName, setNewPackageName] = useState('');
  const [newPackageVersion, setNewPackageVersion] = useState('');
  const [initialized, setInitialized] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [showDiscardPrompt, setShowDiscardPrompt] = useState(false);
  const pendingModeRef = useRef<'ui' | 'toml'>('ui');
  const initialTomlRef = useRef<string>('');

  useEffect(() => {
    if (!initialized && tomlValue) {
      const parsed = parsePixiTomlDependencies(tomlValue);
      if (parsed.length > 0) {
        setPackages(parsed);
      }
      initialTomlRef.current = tomlValue;
      setInitialized(true);
    }
  }, [tomlValue, initialized]);

  const performSwitch = async (newMode: 'ui' | 'toml') => {
    if (newMode === 'toml') {
      if (onReloadToml) {
        setSwitching(true);
        try {
          const freshToml = await onReloadToml();
          onTomlChange(freshToml);
          initialTomlRef.current = freshToml;
        } finally {
          setSwitching(false);
        }
      } else {
        onTomlChange(initialTomlRef.current);
      }
    } else {
      const source = onReloadToml ? tomlValue : initialTomlRef.current;
      const parsed = parsePixiTomlDependencies(source);
      if (parsed.length > 0) {
        setPackages(parsed);
      }
    }
    setDirty(false);
    setMode(newMode);
  };

  const handleModeSwitch = async (newMode: 'ui' | 'toml') => {
    if (newMode === mode) return;

    if (dirty) {
      pendingModeRef.current = newMode;
      setShowDiscardPrompt(true);
      return;
    }

    await performSwitch(newMode);
  };

  const handleConfirmDiscard = async () => {
    setShowDiscardPrompt(false);
    await performSwitch(pendingModeRef.current);
  };

  const getCurrentToml = () => {
    return (
      tomlValue ||
      initialTomlRef.current ||
      buildPixiToml(packages, workspaceName || 'my-project')
    );
  };

  const handleAddPackage = () => {
    if (!newPackageName.trim()) return;
    const updated = [
      ...packages,
      { name: newPackageName.trim(), version: newPackageVersion.trim() },
    ];
    setPackages(updated);
    setDirty(true);
    setNewPackageName('');
    setNewPackageVersion('');
    onTomlChange(patchPixiTomlDependencies(getCurrentToml(), updated));
  };

  const handleRemovePackage = (index: number) => {
    const updated = packages.filter((_, i) => i !== index);
    setPackages(updated);
    setDirty(true);
    onTomlChange(patchPixiTomlDependencies(getCurrentToml(), updated));
  };

  const handleTomlEdit = (value: string) => {
    onTomlChange(value);
    setDirty(value !== initialTomlRef.current);
  };

  return (
    <>
      <ConfirmDialog
        open={showDiscardPrompt}
        onOpenChange={setShowDiscardPrompt}
        title="Unsaved changes"
        description="You have unsaved changes that will be lost if you switch modes. Do you want to continue?"
        confirmText="Discard changes"
        onConfirm={handleConfirmDiscard}
      />

      <div className="flex gap-2 p-1 bg-muted rounded-lg w-fit">
        <Button
          type="button"
          variant={mode === 'toml' ? 'default' : 'ghost'}
          size="sm"
          onClick={() => handleModeSwitch('toml')}
          disabled={switching}
          className="gap-2"
        >
          {switching ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <FileCode className="h-4 w-4" />
          )}
          TOML Mode
        </Button>
        <Button
          type="button"
          variant={mode === 'ui' ? 'default' : 'ghost'}
          size="sm"
          onClick={() => handleModeSwitch('ui')}
          disabled={switching}
          className="gap-2"
        >
          <Plus className="h-4 w-4" />
          UI Mode
        </Button>
      </div>

      {mode === 'ui' ? (
        <div className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium block pt-2 pb-0">
              Workspace Name
            </label>
            <Input
              value={workspaceName || 'my-project'}
              onChange={(e) => {
                const newName = e.target.value;
                setDirty(true);
                onTomlChange(
                  patchPixiTomlWorkspaceName(getCurrentToml(), newName),
                );
              }}
              placeholder="Workspace name"
              className="font-mono"
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium block pt-2 pb-0">
              Packages
            </label>
            <div className="border rounded-lg overflow-hidden">
              <table className="w-full">
                <thead className="bg-muted/50 border-b">
                  <tr>
                    <th className="text-left p-3 text-sm font-medium">Name</th>
                    <th className="text-left p-3 text-sm font-medium">
                      Version Constraint
                    </th>
                    <th className="w-16"></th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {packages.map((pkg, index) => (
                    <tr key={index} className="hover:bg-muted/30">
                      <td className="p-3">
                        <span className="font-mono text-sm">{pkg.name}</span>
                      </td>
                      <td className="p-3">
                        <span className="font-mono text-sm text-muted-foreground">
                          {pkg.version || '-'}
                        </span>
                      </td>
                      <td className="p-3">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => handleRemovePackage(index)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className="flex gap-2">
            <Input
              placeholder="Package name (e.g., numpy)"
              value={newPackageName}
              onChange={(e) => setNewPackageName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleAddPackage();
                }
              }}
            />
            <Input
              placeholder="Version (e.g., >=1.24.0)"
              value={newPackageVersion}
              onChange={(e) => setNewPackageVersion(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleAddPackage();
                }
              }}
              className="w-64"
            />
            <Button
              type="button"
              onClick={handleAddPackage}
              disabled={!newPackageName.trim()}
            >
              <Plus className="h-4 w-4 mr-2" />
              Add Package
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Add packages with optional version constraints (e.g., {'>'}=1.24.0,
            ~=2.0.0, 3.11.*)
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          <label className="text-sm font-medium block pt-2 pb-0">
            pixi.toml Configuration
          </label>
          <Textarea
            placeholder="Enter your pixi.toml content"
            value={tomlValue}
            onChange={(e) => handleTomlEdit(e.target.value)}
            rows={12}
            required
            className="font-mono text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Define your project dependencies and configuration in TOML format
          </p>
        </div>
      )}
    </>
  );
};
