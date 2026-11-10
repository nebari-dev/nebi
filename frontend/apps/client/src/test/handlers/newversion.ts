// Owner: U3 (newversion).
import { HttpResponse, http } from 'msw';
import { mockJobSummary, PROJECT_ID } from '@/test/fixtures/contracts';
import type { VersionDraft } from '@/types/client/newversion';
import type { VersionDiff } from '@/types/contracts';

const C = '/api/v1/client';

export const mockDraftToml = `[workspace]
name = "Project_1"
channels = ["conda-forge"]
platforms = ["linux-64", "linux-aarch64"]

[dependencies]
python = "3.12.*"
numpy = ">=2.0"

[tasks]
train = "python train.py"
`;

export const mockVersionDraft: VersionDraft = {
  project_id: PROJECT_ID,
  project_name: 'Project_1',
  base_version: 7,
  latest_version: 7,
  newer_versions: [],
  pixi_toml: mockDraftToml,
  form: {
    name: 'Project_1',
    channels: ['conda-forge'],
    platforms: ['linux-64', 'linux-aarch64'],
    requested: [
      { name: 'python', constraint: '3.12.*' },
      { name: 'numpy', constraint: '>=2.0' },
    ],
  },
  unrepresentable: ['[tasks]'],
  existing_tags: ['latest', 'stable'],
};

export const mockNoChanges: VersionDiff = {
  platform: 'linux-64',
  requested: [
    {
      name: 'numpy',
      source: 'conda',
      change: 'same',
      base: { constraint: '>=2.0', version: '>=2.0' },
      compare: { constraint: '>=2.0', version: '>=2.0' },
    },
  ],
  requested_counts: { added: 0, removed: 0, changed: 0, same: 1 },
  resolved: [],
  resolved_counts: { added: 0, removed: 0, changed: 0, same: 0 },
  channels: { added: [], removed: [] },
  platforms: { added: [], removed: [] },
};

export const mockChanges: VersionDiff = {
  ...mockNoChanges,
  requested: [
    {
      name: 'numpy',
      source: 'conda',
      change: 'removed',
      base: { constraint: '>=2.0', version: '>=2.0' },
      compare: null,
    },
    {
      name: 'pandas',
      source: 'conda',
      change: 'added',
      base: null,
      compare: { constraint: '>=2.2', version: '>=2.2' },
    },
    {
      name: 'python',
      source: 'conda',
      change: 'changed',
      base: { constraint: '3.12.*', version: '3.12.*' },
      compare: { constraint: '3.13.*', version: '3.13.*' },
    },
  ],
  requested_counts: { added: 1, removed: 1, changed: 1, same: 0 },
  channels: { added: ['pytorch'], removed: [] },
};

export const newVersionHandlers = [
  http.post(`${C}/projects/:id/versions/preview`, () =>
    HttpResponse.json(mockNoChanges),
  ),
  http.get(`${C}/projects/:id/versions/:version/draft`, ({ params }) =>
    HttpResponse.json({ ...mockVersionDraft, project_id: params.id as string }),
  ),
  http.post(`${C}/projects/:id/versions`, () =>
    HttpResponse.json(
      {
        ...mockJobSummary,
        type: 'create_version',
        type_label: 'Create version',
        status: 'pending',
        version_number: null,
      },
      { status: 202 },
    ),
  ),
];
