// Aggregates every unit's MSW handlers (issue #649). Frozen: units only
// edit their own handlers/<unit>.ts. Unit-specific mocks are imported from
// '@/test/handlers/<unit>' or '@/test/fixtures/contracts', not re-exported
// here, so two units can never collide on an export name.
import { setupServer } from 'msw/node';
import { compareHandlers } from './compare';
import { detailHandlers } from './detail';
import { jobsHandlers } from './jobs';
import { legacyHandlers } from './legacy';
import { newVersionHandlers } from './newversion';
import { projectsHandlers } from './projects';
import { publishHandlers } from './publish';
import { remoteHandlers } from './remote';
import { sharingHandlers } from './sharing';

export * from './legacy';

export const handlers = [
  // Unit handlers first. Within the same method, MSW matches handlers in
  // order, so static paths (e.g. /versions/preview) are listed before
  // parameterised ones by keeping each unit's file self-consistent.
  ...newVersionHandlers,
  ...projectsHandlers,
  ...detailHandlers,
  ...compareHandlers,
  ...remoteHandlers,
  ...sharingHandlers,
  ...publishHandlers,
  ...jobsHandlers,
  ...legacyHandlers,
];

export const server = setupServer(...handlers);
