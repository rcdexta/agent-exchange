import { mkdir, copyFile } from 'node:fs/promises';

const root = new URL('../', import.meta.url);
const output = new URL('./dist/', import.meta.url);
await mkdir(output, { recursive: true });
await copyFile(new URL('AGENTS.md', root), new URL('agents.md', output));
for (const file of ['index.html', '_headers', '_redirects']) {
  await copyFile(new URL(file, import.meta.url), new URL(file, output));
}
