# Nebi Docs

This website is built with [Astro](https://astro.build) + [Starlight](https://starlight.astro.build) and the shared [`@nebari/starlight`](https://github.com/nebari-dev/starlight) theme.

## Installation

```bash
npm install
```

## Local Development

```bash
npm run dev
```

This starts a local development server with hot reload.

## Build

```bash
npm run build
```

This generates the static site in `dist/`. The build also validates internal docs links through the Starlight links validator plugin.

## Preview

```bash
npm run preview
```

This serves the built `dist/` output locally.

## Deployment

The docs workflow builds `docs/` and deploys `docs/dist` to Cloudflare Pages with `cloudflare/wrangler-action`. Pushes to `main` deploy production, and same-repository pull requests get preview deployments when the Cloudflare secrets are available:

- `CLOUDFLARE_API_TOKEN`
- `CLOUDFLARE_ACCOUNT_ID`
