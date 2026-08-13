import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function capitalize(str: string): string {
  if (!str) return str;
  return str.charAt(0).toUpperCase() + str.slice(1);
}

const workspaceStatusColors: Record<string, string> = {
  pending: 'bg-yellow-100 text-yellow-800 border-yellow-300',
  creating: 'bg-blue-100 text-blue-800 border-blue-300',
  running: 'bg-blue-100 text-blue-800 border-blue-300',
  ready: 'bg-green-100 text-green-800 border-green-300',
  failed: 'bg-red-100 text-red-800 border-red-300',
  deleting: 'bg-orange-100 text-orange-800 border-orange-300',
};

export function getWorkspaceStatusColor(status: string): string {
  return (
    workspaceStatusColors[status] || 'bg-zinc-100 text-zinc-800 border-zinc-300'
  );
}

const installStatusColors: Record<string, string> = {
  not_installed: 'bg-zinc-100 text-zinc-800 border-zinc-300',
  installing: 'bg-blue-100 text-blue-800 border-blue-300',
  installed: 'bg-green-100 text-green-800 border-green-300',
  uninstalling: 'bg-orange-100 text-orange-800 border-orange-300',
  install_failed: 'bg-red-100 text-red-800 border-red-300',
};

export function getInstallStatusColor(status: string): string {
  return (
    installStatusColors[status] || 'bg-zinc-100 text-zinc-800 border-zinc-300'
  );
}
