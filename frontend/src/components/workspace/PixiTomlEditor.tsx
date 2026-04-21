import { useState, useEffect, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Plus, Trash2, FileCode, Loader2 } from 'lucide-react';

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
    .filter(pkg => pkg.name.trim())
    .map(pkg => {
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

const parsePixiTomlDependencies = (toml: string): Package[] => {
  const lines = toml.split('\n');
  const packages: Package[] = [];
  let inDependencies = false;

  for (const line of lines) {
    const trimmed = line.trim();

    if (trimmed === '[dependencies]') {
      inDependencies = true;
      continue;
    }

    if (inDependencies && trimmed.startsWith('[')) {
      break;
    }

    if (inDependencies && trimmed && !trimmed.startsWith('#')) {
      const match = trimmed.match(/^([^\s=]+)\s*=\s*"([^"]*)"$/);
      if (match) {
        packages.push({ name: match[1], version: match[2] === '*' ? '' : match[2] });
      }
    }
  }

  return packages;
};

export const PixiTomlEditor = ({ tomlValue, onTomlChange, workspaceName, onReloadToml }: PixiTomlEditorProps) => {
  const [mode, setMode] = useState<'ui' | 'toml'>('toml');
  const [packages, setPackages] = useState<Package[]>([{ name: 'python', version: '>=3.11' }]);
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

  const handleAddPackage = () => {
    if (!newPackageName.trim()) return;
    const updated = [...packages, { name: newPackageName.trim(), version: newPackageVersion.trim() }];
    setPackages(updated);
    setDirty(true);
    setNewPackageName('');
    setNewPackageVersion('');
    if (!onReloadToml) {
      onTomlChange(buildPixiToml(updated, workspaceName || 'my-project'));
    }
  };

  const handleRemovePackage = (index: number) => {
    const updated = packages.filter((_, i) => i !== index);
    setPackages(updated);
    setDirty(true);
    if (!onReloadToml) {
      onTomlChange(buildPixiToml(updated, workspaceName || 'my-project'));
    }
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
          {switching ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileCode className="h-4 w-4" />}
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
            <label className="text-sm font-medium block pt-2 pb-0">Packages</label>
            <div className="border rounded-lg overflow-hidden">
              <table className="w-full">
                <thead className="bg-muted/50 border-b">
                  <tr>
                    <th className="text-left p-3 text-sm font-medium">Name</th>
                    <th className="text-left p-3 text-sm font-medium">Version Constraint</th>
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
            Add packages with optional version constraints (e.g., {'>'}=1.24.0, ~=2.0.0, 3.11.*)
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          <label className="text-sm font-medium block pt-2 pb-0">pixi.toml Configuration</label>
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
