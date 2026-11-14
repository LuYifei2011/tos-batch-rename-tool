'use strict';
// Run against disposable loopback demo servers, never a NAS installation.
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const { chromium, firefox, webkit } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { expect } = require((process.env.PLAYWRIGHT_MODULE || 'playwright') + '/test');

async function main() {
  const [appUrl, kitUrl] = process.argv.slice(2);
  for (const value of [appUrl, kitUrl]) {
    const url = new URL(value);
    assert.equal(url.protocol, 'http:');
    assert.ok(['localhost', '127.0.0.1'].includes(url.hostname), 'UI tests require disposable loopback servers');
  }
  const engineName = process.env.TEST_BROWSER || 'chromium';
  const engine = { chromium, firefox, webkit }[engineName];
  assert.ok(engine, 'Unknown browser');
  const browser = await engine.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 960, height: 700 }, locale: 'zh-CN' });
  const errors = [];
  context.on('page', page => page.on('pageerror', error => errors.push(error.message)));
  const output = path.resolve('build/kit-ui-validation', engineName);
  fs.mkdirSync(output, { recursive: true });
  const noOverflow = async page => assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
  try {
    const page = await context.newPage();
    const info = await (await page.request.get(new URL('/v2/proxy/batch-rename/api/info', appUrl).href)).json();
    assert.equal(info.mode, 'demo', 'Refuse to modify files outside demo mode');
    await page.goto(appUrl);
    await page.getByRole('button', { name: '演示共享目录', exact: true }).click();
    await page.getByRole('button', { name: '照片整理', exact: true }).click();
    await page.getByLabel('查找', { exact: true }).fill('IMG_');
    await page.getByLabel('替换为', { exact: true }).fill('KIT_');
    await page.getByRole('button', { name: '重命名 18 项', exact: true }).waitFor();
    await page.getByRole('button', { name: '语言与主题', exact: true }).click();
    await page.getByLabel('语言', { exact: true }).selectOption('en-US');
    await page.getByLabel('Theme', { exact: true }).selectOption('dark');
    await expect(page).toHaveTitle('Batch Rename');
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
    await expect(page.getByRole('heading', { name: 'Language and theme' })).toBeVisible();
    await noOverflow(page);
    await page.screenshot({ path: path.join(output, 'desktop-dark-settings.png') });
    await page.keyboard.press('Escape');
    await expect(page.getByLabel('Find', { exact: true })).toHaveValue('IMG_');
    await page.reload();
    await page.getByRole('button', { name: 'Language and theme', exact: true }).click();
    await expect(page.getByLabel('Language', { exact: true })).toHaveValue('en-US');
    await expect(page.getByLabel('Theme', { exact: true })).toHaveValue('dark');
    await page.getByLabel('Language', { exact: true }).selectOption('zh-CN');
    await page.getByLabel('主题', { exact: true }).selectOption('light');
    for (const theme of ['tos', 'browser', 'dark', 'light']) await page.getByLabel('主题', { exact: true }).selectOption(theme);
    for (const language of ['tos', 'browser', 'en-US', 'zh-CN']) await page.locator('.tos-preference-fields select').first().selectOption(language);
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: '使用说明', exact: true }).click();
    await expect(page.getByRole('dialog')).toContainText('隔离目录中的测试文件');
    await page.getByRole('button', { name: '关闭说明', exact: true }).click();
    await page.getByRole('button', { name: '演示共享目录', exact: true }).click();
    await page.getByRole('button', { name: '照片整理', exact: true }).click();
    await page.getByLabel('查找', { exact: true }).fill('IMG_');
    await page.getByLabel('替换为', { exact: true }).fill('KIT_');
    await page.getByRole('button', { name: '重命名 18 项', exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: '确认重命名', exact: true }).click();
    await expect(page.locator('.preview-footer')).toContainText('任务状态：已完成');
    await expect(page.locator('.original-name').filter({ hasText: 'KIT_202611_0001.jpg' })).toHaveCount(1);
    await page.getByRole('button', { name: '撤销', exact: true }).click();
    await page.getByRole('button', { name: '确认撤销', exact: true }).click();
    await expect(page.locator('.preview-footer')).toContainText('撤销状态：已完成');
    await expect(page.locator('.original-name').filter({ hasText: 'IMG_202611_0001.jpg' })).toHaveCount(1);
    await page.screenshot({ path: path.join(output, 'desktop-light-renamed-and-undone.png') });
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      await noOverflow(page);
      await page.getByRole('button', { name: '语言与主题', exact: true }).click();
      await noOverflow(page);
      assert.equal(await page.getByLabel('主题', { exact: true }).evaluate(el => el.getBoundingClientRect().height), 44);
      await page.screenshot({ path: path.join(output, `mobile-${width}-settings.png`), fullPage: true });
      await page.keyboard.press('Escape');
    }
    const denied = await context.newPage();
    await denied.route('**/v2/proxy/batch-rename/api/**', route => route.fulfill({ status: 403, contentType: 'application/json', body: '{"code":"ORIGIN_MISMATCH"}' }));
    await denied.goto(appUrl);
    await denied.getByText('查看完整报告', { exact: true }).click();
    const report = denied.getByLabel('连接诊断报告', { exact: true });
    await denied.getByRole('button', { name: '选中报告', exact: true }).click();
    assert.equal(await report.evaluate(el => document.activeElement === el && el.selectionEnd === el.value.length && el.selectionStart === 0), true);
    assert.doesNotMatch(await report.inputValue(), /TMSESSNAME=|X-Csrf-Token=/);
    await denied.getByLabel('语言', { exact: true }).selectOption('en-US');
    await expect(denied.getByRole('heading', { name: 'Connection diagnostics', exact: true })).toBeVisible();
    await denied.getByLabel('Theme', { exact: true }).selectOption('dark');
    await denied.screenshot({ path: path.join(output, 'access-denied-diagnostic.png') });
    await noOverflow(denied);

    const kit = await context.newPage();
    await kit.goto(kitUrl);
    await kit.getByLabel('语言', { exact: true }).selectOption('en-US');
    await kit.getByLabel('Theme', { exact: true }).selectOption('dark');
    await expect(kit).toHaveTitle('TOS Kit example');
    await kit.getByRole('button', { name: 'Show sample diagnostic', exact: true }).click();
    await expect(kit.getByRole('heading', { name: 'Connection diagnostics', exact: true })).toBeVisible();
    await kit.getByText('View full report', { exact: true }).click();
    assert.match(await kit.getByLabel('Connection diagnostic report').inputValue(), /kit-example/);
    await kit.getByRole('button', { name: 'Select report', exact: true }).click();
    await kit.setViewportSize({ width: 320, height: 844 });
    await noOverflow(kit);
    await kit.screenshot({ path: path.join(output, 'standalone-kit-mobile.png'), fullPage: true });
    await kit.getByRole('button', { name: 'Clear diagnostic', exact: true }).click();
    await expect(kit.getByRole('heading', { name: 'Connection diagnostics', exact: true })).toHaveCount(0);
    assert.deepEqual(errors, []);
    console.log(`PASS ${engineName}: preferences, persistence, languages/themes, keyboard, rename/undo, diagnostics, independent example, 320/390/960px, no page errors`);
  } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
