const { test } = require('@playwright/test');

test('debug game page', async ({ page }) => {
  page.on('console', msg => console.log('console:', msg.type(), msg.text()));
  page.on('pageerror', err => console.log('pageerror:', err.stack || err.message));
  page.on('requestfailed', req => console.log('requestfailed:', req.url(), req.failure()?.errorText));
  await page.goto('http://127.0.0.1:8900/game/e99e9e46-74e0-4933-88de-f4bc762104e8', { waitUntil: 'load' });
  await page.waitForTimeout(5000);
});
