import { Toaster } from '@nebi/ui/components/ui/toast';
import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Link, Route, Routes, useLocation } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { ManifestEditorProps } from '@/components/manifest/ManifestEditor';
import { mockJobDetail, PROJECT_ID } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import {
  mockChanges,
  mockDraftToml,
  mockVersionDraft,
} from '@/test/handlers/newversion';
import { renderWithProviders, screen, waitFor, within } from '@/test/utils';
import { NewVersionPage } from './NewVersionPage';

// The shared ManifestEditor belongs to the projects unit; these tests pin
// the page's behaviour through its props contract, not its rendering.
vi.mock('@/components/manifest/ManifestEditor', () => ({
  ManifestEditor: ({ value, onChange, errors }: ManifestEditorProps) => (
    <div>
      <span>Editor mode: {value.mode}</span>
      <textarea
        aria-label="pixi.toml"
        value={value.pixiToml}
        onChange={(e) =>
          onChange({ ...value, mode: 'toml', pixiToml: e.target.value })
        }
      />
      <button
        type="button"
        onClick={() =>
          onChange({
            ...value,
            mode: 'form',
            form: {
              ...value.form,
              requested: [
                ...value.form.requested,
                { name: 'pandas', constraint: '>=2.2' },
              ],
            },
          })
        }
      >
        Add pandas
      </button>
      <button
        type="button"
        onClick={() =>
          onChange({
            ...value,
            mode: 'form',
            form: { ...value.form, platforms: [] },
          })
        }
      >
        Clear platforms
      </button>
      {errors?.manifest && <p>Editor error: {errors.manifest}</p>}
      {errors?.platforms && <p>Editor error: {errors.platforms}</p>}
    </div>
  ),
}));

const C = '/api/v1/client';
const PAGE = `/projects/${PROJECT_ID}/new-version?from=7`;

export function LocationProbe() {
  const loc = useLocation();
  return <p data-testid="location">{`${loc.pathname}${loc.search}`}</p>;
}

function renderPage(path = PAGE) {
  return renderWithProviders(
    <TooltipProvider>
      <Link to="/jobs">Header jobs link</Link>
      <Routes>
        <Route path="/projects/:id/new-version" element={<NewVersionPage />} />
        <Route path="/projects/:id" element={<p>Detail page</p>} />
        <Route path="/jobs" element={<p>Jobs page</p>} />
        <Route path="/" element={<p>Projects page</p>} />
      </Routes>
      <LocationProbe />
      <Toaster />
    </TooltipProvider>,
    { initialEntries: [path] },
  );
}

const location = () => screen.getByTestId('location').textContent;

describe('NewVersionPage', () => {
  it('starts from the base version with the prototype copy', async () => {
    renderPage();
    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'New version of Project_1',
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Starts from version 7’s environment. Version 7 stays as it is.',
      ),
    ).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'breadcrumb' });
    expect(within(nav).getByRole('link', { name: 'Projects' })).toHaveAttribute(
      'href',
      '/',
    );
    expect(
      within(nav).getByRole('link', { name: 'Project_1' }),
    ).toHaveAttribute('href', `/projects/${PROJECT_ID}?v=7`);
    expect(screen.getByLabelText('pixi.toml')).toHaveValue(mockDraftToml);
    expect(screen.getByText('Editor mode: form')).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'About this version' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'A tag stays with this version until a newer version uses the same tag. latest does this automatically.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('latest')).toBeInTheDocument();
    expect(
      await screen.findByText('Nothing changed in Project_1.'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Create version' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Create and install' }),
    ).toBeInTheDocument();
  });

  it('opens in the pixi.toml editor with ?mode=toml', async () => {
    renderPage(`${PAGE}&mode=toml`);
    expect(await screen.findByText('Editor mode: toml')).toBeInTheDocument();
  });

  it('shows the summary of changes and suggests a description', async () => {
    const bodies: unknown[] = [];
    server.use(
      http.post(`${C}/projects/:id/versions/preview`, async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json(mockChanges);
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Add pandas' }));

    expect(
      await screen.findByRole('img', {
        name: 'Summary of changes: 1 added, 1 removed, 1 changed',
      }),
    ).toBeInTheDocument();
    const rows = screen.getByRole('list', { name: 'Package changes' });
    expect(within(rows).getByText('pandas')).toBeInTheDocument();
    expect(within(rows).getByText('3.12.* → 3.13.*')).toBeInTheDocument();
    expect(screen.getByText('added pytorch')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Description' })).toHaveValue(
      'Added pandas, removed numpy, changed python',
    );
    expect(
      screen.getByText('Suggested from your changes.'),
    ).toBeInTheDocument();
    // Form mode previews the form (applied onto the base pixi.toml).
    await waitFor(() =>
      expect(bodies.at(-1)).toMatchObject({
        base_version: 7,
        form: {
          requested: expect.arrayContaining([
            { name: 'pandas', constraint: '>=2.2' },
          ]),
        },
      }),
    );
  });

  it('requires a description before creating', async () => {
    const create = vi.fn();
    server.use(
      http.post(`${C}/projects/:id/versions`, () => {
        create();
        return HttpResponse.json({}, { status: 500 });
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByText('Nothing changed in Project_1.');
    await user.click(screen.getByRole('button', { name: 'Create version' }));
    expect(await screen.findByText('Enter a description.')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Description' })).toHaveFocus();
    expect(create).not.toHaveBeenCalled();
  });

  it('asks for a platform in the form', async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(
      await screen.findByRole('button', { name: 'Clear platforms' }),
    );
    await user.type(
      screen.getByRole('textbox', { name: 'Description' }),
      'No platforms',
    );
    await user.click(screen.getByRole('button', { name: 'Create version' }));
    expect(
      await screen.findByText('Editor error: Add at least one platform.'),
    ).toBeInTheDocument();
  });

  it('says when nothing changed', async () => {
    server.use(
      http.post(`${C}/projects/:id/versions`, () =>
        HttpResponse.json(
          { error: 'Nothing changed in Project_1.', code: 'no_changes' },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderPage();
    await user.type(
      await screen.findByRole('textbox', { name: 'Description' }),
      'Same as before',
    );
    await user.click(screen.getByRole('button', { name: 'Create version' }));
    expect(
      await screen.findByText(
        'Change a package, channel or platform to create a new version.',
      ),
    ).toBeInTheDocument();
    expect(location()).toBe(PAGE);
  });

  it('shows server validation errors on the pixi.toml editor', async () => {
    server.use(
      http.post(`${C}/projects/:id/versions`, () =>
        HttpResponse.json(
          {
            error: 'List at least one platform in platforms = [ ].',
            code: 'invalid',
            field: 'manifest',
          },
          { status: 422 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderPage();
    const toml = await screen.findByLabelText('pixi.toml');
    await user.clear(toml);
    await user.type(toml, '[workspace]');
    await user.type(
      screen.getByRole('textbox', { name: 'Description' }),
      'Broken',
    );
    await user.click(screen.getByRole('button', { name: 'Create version' }));
    expect(
      await screen.findByText(
        'Editor error: List at least one platform in platforms = [ ].',
      ),
    ).toBeInTheDocument();
  });

  it('creates the version, follows the job and installs it', async () => {
    let body: Record<string, unknown> | undefined;
    let installBody: unknown;
    server.use(
      http.post(`${C}/projects/:id/versions/preview`, () =>
        HttpResponse.json(mockChanges),
      ),
      http.post(`${C}/projects/:id/versions`, async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(
          {
            ...mockJobDetail,
            type: 'create_version',
            type_label: 'Create version',
            status: 'running',
            version_number: null,
          },
          { status: 202 },
        );
      }),
      http.get(`${C}/jobs/:jobId`, () =>
        HttpResponse.json({
          ...mockJobDetail,
          type: 'create_version',
          type_label: 'Create version',
          status: 'completed',
          version_number: 8,
        }),
      ),
      http.post(`${C}/projects/:id/install`, async ({ request }) => {
        installBody = await request.json();
        return HttpResponse.json(
          { ...mockJobDetail, status: 'running', version_number: 8 },
          { status: 202 },
        );
      }),
    );
    const user = userEvent.setup();
    renderPage(`${PAGE}&mode=toml`);
    const toml = await screen.findByLabelText('pixi.toml');
    await user.type(toml, '\n# edited');
    await screen.findByText('Suggested from your changes.');

    const tags = screen.getByRole('combobox', { name: 'Tags (optional)' });
    await user.type(tags, 'stable{Enter}');
    await user.click(
      screen.getByRole('button', { name: 'Create and install' }),
    );

    await waitFor(() => expect(location()).toBe(`/projects/${PROJECT_ID}`));
    expect(body).toMatchObject({
      base_version: 7,
      pixi_toml: `${mockDraftToml}\n# edited`,
      description: 'Added pandas, removed numpy, changed python',
      tags: ['stable'],
    });
    expect(body).not.toHaveProperty('form');
    expect(
      await screen.findByText('Version 8 created', {}, { timeout: 4000 }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Started from version 7 · added pandas · removed numpy · changed python',
      ),
    ).toBeInTheDocument();
    await waitFor(() => expect(installBody).toEqual({ version: 8 }));
  });

  it('asks before discarding changes and stays on Go back', async () => {
    const user = userEvent.setup();
    renderPage();
    await user.type(
      await screen.findByRole('textbox', { name: 'Description' }),
      'Draft',
    );
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(
      within(dialog).getByText('Discard new version?'),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        'Your changes are discarded. Version 7 stays as it is.',
      ),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Go back' }));
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
    );
    expect(location()).toBe(PAGE);

    // Links elsewhere in the app are guarded too.
    await user.click(screen.getByRole('link', { name: 'Header jobs link' }));
    const again = await screen.findByRole('alertdialog');
    await user.click(within(again).getByRole('button', { name: 'Discard' }));
    await waitFor(() => expect(location()).toBe('/jobs'));
  });

  it('leaves without asking when nothing was edited', async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(location()).toBe(`/projects/${PROJECT_ID}?v=7`));
  });

  it('starts from the latest version without ?from', async () => {
    let requested = '';
    server.use(
      http.get(`${C}/projects/:id/versions/:version/draft`, ({ params }) => {
        requested = params.version as string;
        return HttpResponse.json(mockVersionDraft);
      }),
    );
    renderPage(`/projects/${PROJECT_ID}/new-version`);
    await screen.findByRole('heading', { name: 'New version of Project_1' });
    expect(requested).toBe('latest');
  });

  it('shows not found for an unknown project or version', async () => {
    server.use(
      http.get(`${C}/projects/:id/versions/:version/draft`, () =>
        HttpResponse.json(
          { error: 'not found', code: 'not_found' },
          { status: 404 },
        ),
      ),
    );
    renderPage();
    expect(
      await screen.findByRole('heading', { name: 'Project not found' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Back to projects' }),
    ).toBeInTheDocument();
  });
});
