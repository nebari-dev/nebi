// Owner: U1 (projects). Section copy of the ManifestEditor.
import type { ReactNode } from 'react';

/** Section copy; defaults are the Create project page's. */
export interface ManifestEditorCopy {
  environmentIntro: ReactNode;
  platformsDescription: ReactNode;
  packagesIntro: ReactNode;
  packagesEmptyText: string;
  tomlIntro: ReactNode;
}

export const CREATE_PROJECT_COPY: ManifestEditorCopy = {
  environmentIntro: 'The platforms pixi solves this project for.',
  platformsDescription:
    'Your machine’s platform is added. For uncommon platforms, type pixi’s name (e.g. linux-ppc64le).',
  packagesIntro:
    'Add packages to install in this project. Leave the version blank to allow any version.',
  packagesEmptyText: 'No packages yet. You can still create an empty project.',
  tomlIntro:
    'Platforms, channels and dependencies. Tags and the description aren’t part of pixi.toml, so they’re set below.',
};
