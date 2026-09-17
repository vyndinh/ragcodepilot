const { test, expect } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const baseline = JSON.parse(fs.readFileSync(path.join(root, 'docs/eval/baseline_v8.json')));

test.beforeEach(async ({ page }) => { await page.goto('/'); });

test('answer toggling cannot resurrect stale or unsupported answers', async ({ page }) => {
  const toggle = page.locator('#simAnswerToggleBtn');
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await toggle.click();
  await expect(page.locator('#simAnswerText')).toContainText('two-tier');
  await toggle.click();
  await page.locator('.preset-btn').nth(1).click();
  await toggle.click();
  await expect(page.locator('#simAnswerText')).toContainText('ChunkFile is defined');
  for (const query of ['unknown query', '']) {
    await page.locator('#simQueryInput').fill(query);
    await page.locator('#simRunBtn').click();
    await toggle.click();
    await toggle.click();
    await expect(page.locator('#simAnswerBox')).toBeHidden();
    await expect(page.locator('#simAnswerText')).toBeEmpty();
    await expect(page.locator('#simCitationsList')).toBeEmpty();
  }
});

test('all presets respect mode paths and citation destinations', async ({ page }) => {
  for (let preset = 0; preset < 5; preset++) {
    await page.locator('.preset-btn').nth(preset).click();
    for (const mode of ['hybrid', 'dense', 'sparse']) {
      await page.locator(`[data-mode="${mode}"]`).click();
      await expect(page.locator('#simDenseCard')).toHaveClass(mode === 'sparse' ? /is-skipped/ : /^(?!.*is-skipped)/);
      await expect(page.locator('#simSparseCard')).toHaveClass(mode === 'dense' ? /is-skipped/ : /^(?!.*is-skipped)/);
      for (const enabled of [true, false]) {
        await page.locator('#simAnswerToggleBtn').click();
        await expect(page.locator('#simAnswerBox'))[enabled ? 'toBeVisible' : 'toBeHidden']();
        if (enabled) {
          const links = await page.locator('#simCitationsList a').evaluateAll(nodes => nodes.map(a => ({
            valid: a.target === '_blank' ? a.href.includes('/blob/') : Boolean(document.querySelector(a.hash))
          })));
          expect(links.length).toBeGreaterThan(0);
          expect(links.every(link => link.valid)).toBeTruthy();
        }
      }
    }
  }
  await page.locator('.preset-btn').first().click();
  await page.locator('#simAnswerToggleBtn').click();
  await page.locator('#simCitationsList a').first().click();
  await expect(page.locator('#sim-source-0')).toBeFocused();
});

for (const theme of ['light', 'dark']) {
  test(`${theme}: narrow screens and deep dives stay within viewport`, async ({ page }) => {
    if (theme === 'dark') await page.locator('#themeToggleBtn').click();
    for (const width of [320, 390, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 900 });
      for (const panel of ['ast', 'enrichment', 'rrf', 'incremental']) {
        await page.locator(`[data-tab="${panel}"]`).click();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
      }
      for (const id of ['setupClone', 'setupServices', 'setupIndex', 'setupSearch', 'setupAnswer']) {
        const bounds = await page.locator(`#${id}`).evaluate(e => {
          const r = e.parentElement.getBoundingClientRect(); return { right: r.right, width: innerWidth };
        });
        expect(bounds.right).toBeLessThanOrEqual(bounds.width);
      }
    }
  });
}

test('keyboard, failure path, RRF math, and mobile navigation', async ({ page }) => {
  await expect(page.locator('#playPauseText')).toHaveText('Play Animation');
  await page.locator('#cnode-ingest-walker').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('#inspectTitle')).toContainText('File Walker');
  await page.locator('#walkFailure').click();
  await expect(page.locator('#walkOutputStage')).toBeHidden();
  await page.locator('#walkSuccess').click();
  await expect(page.locator('#walkOutputStage')).toBeVisible();
  await page.locator('[data-tab="rrf"]').click();
  await expect(page.locator('#rrfExampleBody tr').first()).toContainText('0.03306');
  await page.locator('[data-rrf-mode="dense"]').click();
  await expect(page.locator('#rrfExampleBody tr').first()).toContainText('Conceptual overview');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('#navToggleBtn').click();
  await page.locator('#siteNav a[href="#getting-started"]').click();
  await expect(page.locator('#navToggleBtn')).toHaveAttribute('aria-expanded', 'false');
});

test('scoreboard and full query table match saved evidence', async ({ page }) => {
  await expect(page.locator('[data-evidence="hit5"]').first()).toHaveText(`${(baseline.aggregate.hit_at_5 * 100).toFixed(1)}%`);
  await expect(page.locator('#evalTableBody tr')).toHaveCount(baseline.queries.length);
  await expect(page.locator('[data-evidence="retrievalDate"]')).toHaveText(baseline.run_id.slice(0, 10));
  await expect(page.locator('#implementationRevision')).toHaveAttribute('href', /\/tree\/[a-f0-9]{40}$/);
  await expect(page.locator('#evalTableBody')).toContainText(baseline.queries[0].query);
});

test('setup commands copy successfully and clipboard rejection is handled', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  for (const id of ['setupClone', 'setupServices', 'setupIndex', 'setupSearch', 'setupAnswer']) {
    await page.locator(`[data-copy-command="${id}"]`).click();
    const copied = await page.evaluate(() => navigator.clipboard.readText());
    expect(copied).toBe(await page.locator(`#${id}`).textContent());
  }
  await page.evaluate(() => { Object.defineProperty(navigator.clipboard, 'writeText', { value: () => Promise.reject(new Error('denied')) }); });
  await page.locator('#copyCliQuickstartBtn').click();
  await expect(page.locator('#copyStatus')).toContainText('Clipboard unavailable');
});

test('loads local assets without browser errors or broken anchors', async ({ page }) => {
  const errors = [], external = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => { if (!request.url().startsWith('http://127.0.0.1:8770/')) external.push(request.url()); });
  await page.reload();
  await expect(page.locator('#evalTableBody tr')).toHaveCount(baseline.queries.length);
  const missing = await page.locator('a[href^="#"]').evaluateAll(links => links.filter(a => a.hash && !document.querySelector(a.hash)).map(a => a.hash));
  expect(missing).toEqual([]);
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
});
