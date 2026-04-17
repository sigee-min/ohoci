import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

async function readSource(relativePath) {
  return readFile(path.join(root, relativePath), 'utf8');
}

test('warm degradation data is consumed by operator surfaces', async () => {
  const hookSource = await readSource('src/hooks/use-workspace-app.js');
  const overviewSource = await readSource('src/views/overview-view.jsx');
  const runnersSource = await readSource('src/views/runners-view.jsx');

  assert.match(hookSource, /degradedTargets/, 'workspace hook should compute degradedTargets');
  assert.match(overviewSource, /warmPoolStatus\?\.degradedTargets/, 'Overview should consume degradedTargets directly');
  assert.match(overviewSource, /overview\.warmTargets\.title/, 'Overview should render a warm target detail surface');
  assert.match(runnersSource, /warmState/, 'Runners view should render warm state');
  assert.match(runnersSource, /warmRepoOwner/, 'Runners view should render warm target owner');
  assert.match(runnersSource, /warmRepoName/, 'Runners view should render warm target repository');
});
