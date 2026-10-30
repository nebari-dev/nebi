import { cpSync, rmSync } from 'node:fs';

// Go embeds a separate copy of both bundles. Replace it after every successful
// build so direct Wails builds cannot package assets from an earlier build.
const source = new URL('../dist/', import.meta.url);
const destination = new URL('../../internal/frontend/dist/', import.meta.url);
rmSync(destination, { recursive: true, force: true });
cpSync(source, destination, { recursive: true });
