import { TooltipProvider } from '@nebi/ui/components/ui/tooltip';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { mockCapabilities } from '@/test/fixtures/contracts';
import { server } from '@/test/handlers';
import { NEW_PROJECT_ID } from '@/test/handlers/projects';
import type { CreateProjectRequest } from '@/types/contracts';
import { CreateProjectPage } from './CreateProjectPage';

const C = '/api/v1/client';

// biome-ignore lint/style/useComponentExportOnlyModules: test-only route probe
const Where = () => {
  const loc = useLocation();
  return <div data-testid="location">{loc.pathname + loc.search}</div>;
};

function renderPage(path = '/projects/new') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route path="/projects/new" element={<CreateProjectPage />} />
            <Route path="*" element={<Where />} />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

function captureCreate() {
  const seen: { body?: CreateProjectRequest } = {};
  server.use(
    http.post(`${C}/projects`, async ({ request }) => {
      seen.body = (await request.json()) as CreateProjectRequest;
      return HttpResponse.json(
        {
          project: { id: NEW_PROJECT_ID, name: seen.body.name },
          job: {
            id: 'j1',
            type: 'create',
            type_label: 'Create project',
            status: 'pending',
            project_id: NEW_PROJECT_ID,
            project_name: seen.body.name,
            version_number: null,
            created_at: '2026-09-29T00:00:00Z',
            started_at: null,
            completed_at: null,
            duration_ms: null,
          },
        },
        { status: 202 },
      );
    }),
  );
  return seen;
}

const nameInput = () => screen.getByPlaceholderText('e.g., my-data-project');

describe('CreateProjectPage', () => {
  it('renders the form with the prototype defaults', async () => {
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    expect(
      screen.getByRole('heading', { level: 1, name: 'New project' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Project' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Environment' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Packages' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'About version 1' }),
    ).toBeInTheDocument();
    expect(screen.getByText('linux-64 · this machine')).toBeInTheDocument();
    expect(screen.getByText('python')).toBeInTheDocument();
    expect(screen.getByLabelText('Version for python')).toHaveValue('3.11');
    expect(
      screen.getByDisplayValue('Initial project creation'),
    ).toBeInTheDocument();
    expect(screen.getByText('latest')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Open in TOML editor/ }),
    ).toBeInTheDocument();
    // nebi-web has no native folder picker.
    expect(
      screen.queryByRole('button', { name: 'Choose a folder' }),
    ).not.toBeInTheDocument();
  });

  it('validates the name, description and duplicates', async () => {
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(
      await screen.findByText('Enter a project name.'),
    ).toBeInTheDocument();

    await userEvent.type(nameInput(), 'bad name');
    expect(
      screen.getByText('Use letters, numbers, hyphens, underscores or dots.'),
    ).toBeInTheDocument();

    await userEvent.clear(nameInput());
    await userEvent.type(nameInput(), 'project_1');
    expect(
      await screen.findByText('A project with that name already exists.'),
    ).toBeInTheDocument();

    await userEvent.clear(nameInput());
    await userEvent.type(nameInput(), 'fresh');
    await userEvent.clear(screen.getByDisplayValue('Initial project creation'));
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(await screen.findByText('Enter a description.')).toBeInTheDocument();
  });

  it('creates from the form and opens the new project', async () => {
    const seen = captureCreate();
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.type(nameInput(), 'my-data');
    await userEvent.type(screen.getByPlaceholderText('Package name'), 'NumPy');
    await userEvent.type(screen.getByPlaceholderText('e.g. 1.0'), '2.0');
    await userEvent.click(screen.getByRole('button', { name: /Add package/ }));
    expect(screen.getByText('numpy')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() =>
      expect(screen.getByTestId('location')).toHaveTextContent(
        `/projects/${NEW_PROJECT_ID}?v=1`,
      ),
    );
    expect(seen.body).toEqual({
      name: 'my-data',
      description: 'Initial project creation',
      tags: ['latest'],
      install: false,
      form: {
        name: 'my-data',
        channels: ['conda-forge'],
        platforms: ['linux-64'],
        requested: [
          { name: 'python', constraint: '>=3.11' },
          { name: 'numpy', constraint: '>=2.0' },
        ],
      },
    });
  });

  it('sends install for Create and install and passes the path', async () => {
    const seen = captureCreate();
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.type(nameInput(), 'envy');
    await userEvent.type(
      screen.getByPlaceholderText('e.g., /home/user/projects/my-data-project'),
      '/srv/envy',
    );
    await userEvent.click(
      screen.getByRole('button', { name: 'Create and install' }),
    );
    await waitFor(() => expect(seen.body?.install).toBe(true));
    expect(seen.body?.path).toBe('/srv/envy');
  });

  it('shows server field errors', async () => {
    server.use(
      http.post(`${C}/projects`, () =>
        HttpResponse.json(
          {
            error:
              'That folder already has a pixi.toml. Choose an empty folder.',
            code: 'invalid',
            field: 'path',
          },
          { status: 422 },
        ),
      ),
    );
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.type(nameInput(), 'clash');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(
      await screen.findByText(
        'That folder already has a pixi.toml. Choose an empty folder.',
      ),
    ).toBeInTheDocument();
  });

  it('creates from the pixi.toml editor, keeping the name in step', async () => {
    const seen = captureCreate();
    renderPage('/projects/new?mode=toml');
    await screen.findByPlaceholderText('e.g., my-data-project');
    expect(
      screen.getByText(
        'You may lose some data that isn’t supported in the GUI editor.',
      ),
    ).toBeInTheDocument();
    const editor = screen.getByRole('textbox', { name: 'pixi.toml' });
    expect((editor as HTMLTextAreaElement).value).toContain(
      'platforms = ["linux-64"]',
    );
    await userEvent.type(nameInput(), 'tomly');
    expect((editor as HTMLTextAreaElement).value).toContain('name = "tomly"');

    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() => expect(seen.body?.pixi_toml).toContain('tomly'));
    expect(seen.body?.form).toBeUndefined();
  });

  it('shows pixi.toml errors from the server under the editor', async () => {
    server.use(
      http.post(`${C}/projects`, () =>
        HttpResponse.json(
          {
            error: 'pixi.toml needs a [workspace] table.',
            code: 'invalid',
            field: 'manifest',
          },
          { status: 422 },
        ),
      ),
    );
    renderPage('/projects/new?mode=toml');
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.type(nameInput(), 'broken');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    expect(
      await screen.findByText('pixi.toml needs a [workspace] table.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'pixi.toml' })).toHaveAttribute(
      'aria-invalid',
      'true',
    );
  });

  it('switches between the GUI form and the pixi.toml editor', async () => {
    let parsed = '';
    server.use(
      http.post(`${C}/manifest/parse`, async ({ request }) => {
        parsed = ((await request.json()) as { pixi_toml: string }).pixi_toml;
        return HttpResponse.json({
          form: {
            name: 'from-toml',
            channels: ['conda-forge'],
            platforms: ['osx-arm64'],
            requested: [],
          },
          unrepresentable: ['[tasks]'],
        });
      }),
    );
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.click(
      screen.getByRole('button', { name: /Open in TOML editor/ }),
    );
    const editor = await screen.findByRole('textbox', { name: 'pixi.toml' });
    expect((editor as HTMLTextAreaElement).value).toContain('[workspace]');

    await userEvent.click(
      screen.getByRole('button', { name: /Open in GUI editor/ }),
    );
    expect(
      await screen.findByText('osx-arm64 · macOS Apple silicon'),
    ).toBeInTheDocument();
    expect(parsed).toContain('[workspace]');
    // The name field picks up the pixi.toml name when it was empty.
    expect(nameInput()).toHaveValue('from-toml');
    expect(
      screen.getByText(
        'No packages yet. You can still create an empty project.',
      ),
    ).toBeInTheDocument();
  });

  it('asks before discarding a filled form', async () => {
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.type(nameInput(), 'half-done');
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(
      within(dialog).getByText('Discard new project?'),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText('Any filled data will be discarded.'),
    ).toBeInTheDocument();
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Go back' }),
    );
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument(),
    );
    expect(nameInput()).toHaveValue('half-done');

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await userEvent.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Discard',
      }),
    );
    expect(screen.getByTestId('location')).toHaveTextContent('/');
  });

  it('leaves at once when nothing was filled', async () => {
    renderPage();
    await screen.findByPlaceholderText('e.g., my-data-project');
    await userEvent.click(screen.getByRole('link', { name: 'Projects' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent('/');
  });

  it('uses the native folder picker in the desktop app', async () => {
    server.use(
      http.get(`${C}/capabilities`, () =>
        HttpResponse.json({
          ...mockCapabilities,
          native_folder_picker: true,
          platform: 'darwin/arm64',
        }),
      ),
      http.post(`${C}/desktop/pick-folder`, () =>
        HttpResponse.json({ path: '/Users/sam/work/proj', cancelled: false }),
      ),
    );
    renderPage();
    await userEvent.click(
      await screen.findByRole('button', { name: 'Choose a folder' }),
    );
    await waitFor(() =>
      expect(
        screen.getByPlaceholderText(
          'e.g., /home/user/projects/my-data-project',
        ),
      ).toHaveValue('/Users/sam/work/proj'),
    );
    expect(screen.getByText('osx-arm64 · this machine')).toBeInTheDocument();
  });
});
