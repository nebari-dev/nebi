// Owner: U2 (detail). Steps of the project page tour (prototype copy).
// Each step highlights the first element found with one of its
// data-tour targets; steps whose targets are not on the page are skipped.

export type TourPlacement = 'right' | 'bottom' | 'bottom-end';

export interface TourStep {
  id: string;
  targets: string[];
  title: string;
  body: string;
  placement: TourPlacement;
  /** Extra space around the highlighted element (px). */
  pad?: number;
  /** The closing step: no counter, "Done" instead of "Next". */
  finish?: boolean;
}

export const PROJECT_TOUR_STEPS: TourStep[] = [
  {
    id: 'versions',
    targets: ['rail'],
    title: 'Every change is a new version',
    body: 'A version is a saved snapshot of this project’s packages. Versions never change, so you can always go back to one.',
    placement: 'right',
    pad: 8,
  },
  {
    id: 'installed',
    targets: ['installed', 'install'],
    title: 'One version is installed at a time',
    body: 'The installed version is the one on your disk. Installing another version replaces it.',
    placement: 'bottom',
  },
  {
    id: 'create',
    targets: ['create-version'],
    title: 'Change packages with a new version',
    body: 'Start from any version, edit its packages, and save it as the next version.',
    placement: 'bottom',
  },
  {
    id: 'push',
    targets: ['push'],
    title: 'Push sends your versions to the server',
    body: 'Versions you create stay on this machine until you push them. Teammates only see pushed versions.',
    placement: 'bottom',
  },
  {
    id: 'share',
    targets: ['share'],
    title: 'Share gives teammates access',
    body: 'Add people or teams to this project on your Nebi server. They can see and pull its versions.',
    placement: 'bottom-end',
  },
  {
    id: 'publish',
    targets: ['publish'],
    title: 'Publish sends a version to a registry',
    body: 'Use it to make a version available outside Nebi, like GHCR or Quay. To work with teammates, use Share.',
    placement: 'bottom',
  },
  {
    id: 'finish',
    targets: ['help'],
    title: 'Come back anytime',
    body: 'Restart this tour from Help whenever you need it.',
    placement: 'bottom-end',
    finish: true,
  },
];

/** The element a step points at, or null when it is not on the page. */
export function tourTarget(step: TourStep): HTMLElement | null {
  for (const t of step.targets) {
    const el = document.querySelector<HTMLElement>(`[data-tour="${t}"]`);
    if (el) return el;
  }
  return null;
}
