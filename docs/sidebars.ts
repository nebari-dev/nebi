import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'why-nebi',
    'getting-started',
    {
      type: 'category',
      label: 'Tutorial',
      link: {type: 'doc', id: 'tutorial/index'},
      items: [
        'tutorial/first-steps',
        'tutorial/working-with-workspaces',
        'tutorial/sharing-with-a-server',
        'tutorial/versions-and-tags',
        'tutorial/status-and-diffing',
        'tutorial/publishing-to-oci',
        'tutorial/working-as-a-team',
      ],
    },
    {
      type: 'category',
      label: 'Concepts',
      link: {type: 'doc', id: 'concepts/index'},
      items: [
        'concepts/workspaces',
        'concepts/lifecycle',
        'concepts/versions-and-tags',
        'concepts/origin-tracking',
        'concepts/pixi-prerequisites',
      ],
    },
    {
      type: 'category',
      label: 'How-To Guides',
      link: {type: 'doc', id: 'how-to/index'},
      items: [
        'how-to/use-someone-elses-environment',
        'how-to/start-from-a-template',
        'how-to/share-with-a-teammate',
        'how-to/compare-environments',
        'how-to/set-up-a-server',
        'how-to/publish-to-oci',
        'how-to/use-without-a-server',
      ],
    },
    'cli-guide',
    'cli-reference',
    'architecture',
    'server-setup',
    {
      type: 'category',
      label: 'Maintainers',
      items: [
        'maintainers-conda-forge',
      ],
    },
  ],
};

export default sidebars;
