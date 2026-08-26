import starlight from '@astrojs/starlight';
import { nebari } from '@nebari/starlight';
import mermaid from 'astro-mermaid';
import { defineConfig } from 'astro/config';
import starlightLinksValidator from 'starlight-links-validator';

const site = process.env.SITE ?? 'https://nebi.nebari.dev';
const base = process.env.BASE ?? '/';

export default defineConfig({
  site,
  base,
  integrations: [
    mermaid({
      autoTheme: true,
      enableLog: false,
    }),
    starlight({
      title: 'Nebi',
      description: 'Environment management for teams',
      favicon: '/img/nebi-icon.svg',
      customCss: ['./src/styles/home.css'],
      components: {
        Head: './src/starlight/Head.astro',
      },
      editLink: {
        baseUrl: 'https://github.com/nebari-dev/nebi/edit/main/docs/',
      },
      plugins: [
        nebari({
          githubHref: 'https://github.com/nebari-dev/nebi',
          logo: {
            light: '/nebi-logo.svg',
            dark: '/nebi-logo-dark.svg',
            alt: 'Nebi',
          },
        }),
        starlightLinksValidator(),
      ],
      sidebar: [
        'introduction',
        'installation',
        'quick-start',
        'nebi-components',
        'pixi-essentials',
        {
          label: 'CLI Usage',
          items: ['cli-local', 'cli-team', 'cli-reference'],
        },
        'ui',
        'registry-setup',
        'server-setup',
        {
          label: 'Examples',
          items: [
            'examples/sharing-environments',
            'examples/version-rollback',
          ],
        },
        {
          label: 'Development',
          items: ['maintainers-conda-forge'],
        },
      ],
    }),
  ],
});
