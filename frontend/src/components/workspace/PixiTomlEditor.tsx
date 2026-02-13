import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Plus, Trash2, FileCode } from 'lucide-react';

interface Package {
  name: string;
  version: string;
}

interface PixiTomlEditorProps {
  tomlValue: string;
  onTomlChange: (toml: string) => void;
  workspaceName: string;
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

export const PixiTomlEditor = ({ tomlValue, onTomlChange, workspaceName }: PixiTomlEditorProps) => {
  const [mode, setMode] = useState<'ui' | 'toml'>('ui');
  const [packages, setPackages] = useState<Package[]>([{ name: 'python', version: '>=3.11' }]);
  const [newPackageName, setNewPackageName] = useState('');
  const [newPackageVersion, setNewPackageVersion] = useState('');
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (!initialized && tomlValue) {
      const parsed = parsePixiTomlDependencies(tomlValue);
      if (parsed.length > 0) {
        setPackages(parsed);
      }
      setInitialized(true);
    }
  }, [tomlValue, initialized]);

  const handleModeSwitch = (newMode: 'ui' | 'toml') => {
    if (newMode === 'toml' && mode === 'ui') {
      onTomlChange(buildPixiToml(packages, workspaceName));
    } else if (newMode === 'ui' && mode === 'toml') {
      const parsed = parsePixiTomlDependencies(tomlValue);
      if (parsed.length > 0) {
        setPackages(parsed);
      }
    }
    setMode(newMode);
  };

  const handleAddPackage = () => {
    if (!newPackageName.trim()) return;
    const updated = [...packages, { name: newPackageName, version: newPackageVersion }];
    setPackages(updated);
    setNewPackageName('');
    setNewPackageVersion('');
    onTomlChange(buildPixiToml(updated, workspaceName));
  };

  const handleRemovePackage = (index: number) => {
    const updated = packages.filter((_, i) => i !== index);
    setPackages(updated);
    onTomlChange(buildPixiToml(updated, workspaceName));
  };

  return (
    <>
      <div className="flex gap-2 p-1 bg-muted rounded-lg w-fit">
        <Button
          type="button"
          variant={mode === 'ui' ? 'default' : 'ghost'}
          size="sm"
          onClick={() => handleModeSwitch('ui')}
          className="gap-2"
        >
          <Plus className="h-4 w-4" />
          UI Mode
        </Button>
        <Button
          type="button"
          variant={mode === 'toml' ? 'default' : 'ghost'}
          size="sm"
          onClick={() => handleModeSwitch('toml')}
          className="gap-2"
        >
          <FileCode className="h-4 w-4" />
          TOML Mode
        </Button>
      </div>

      {mode === 'ui' ? (
        <div className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Packages</label>
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
          <label className="text-sm font-medium">pixi.toml Configuration</label>
          <Textarea
            placeholder="Enter your pixi.toml content"
            value={tomlValue}
            onChange={(e) => onTomlChange(e.target.value)}
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
