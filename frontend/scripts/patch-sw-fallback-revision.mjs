// Runs after `vite build` (see package.json's "build" script). The SW's
// precache entry for the SPA-fallback shell is declared in vite.config.ts
// with a placeholder revision (see comment there for why: computing it
// during the Vite build itself races with adapter-static writing the real
// index.html). This patches in a real content hash once build/ is final.
import { createHash } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';

const PLACEHOLDER = '__INDEX_HTML_REVISION__';

const indexHtml = await readFile('build/index.html');
const revision = createHash('md5').update(indexHtml).digest('hex');

const swPath = 'build/sw.js';
const sw = await readFile(swPath, 'utf-8');
if (!sw.includes(PLACEHOLDER)) {
	throw new Error(`${PLACEHOLDER} not found in ${swPath} — did the workbox config change?`);
}
await writeFile(swPath, sw.replaceAll(PLACEHOLDER, revision), 'utf-8');
