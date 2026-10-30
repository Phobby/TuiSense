// End-to-end test of the web demo in a real browser (Chromium via
// Playwright): the website, the browser terminal and the real TuiSense
// running behind tuisense-webd.
//
//   TUISENSE_E2E_URL=http://127.0.0.1:8080 node website/e2e/demo.e2e.cjs [screenshot-dir]
//
// Server mode needs a running tuisense-webd that serves the built frontend
// (TUISENSE_WEB_STATIC_DIR). With TUISENSE_E2E_MODE=wasm it tests the static
// WebAssembly site instead (make web-pages, served at TUISENSE_E2E_URL).
// See docs/WEB_DEMO.md, "Testing".
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const base = (process.env.TUISENSE_E2E_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const wasm = process.env.TUISENSE_E2E_MODE === 'wasm';
const shots = process.argv[2];
let failures = 0;

function check(cond, msg) {
  if (cond) {
    console.log('  ✓', msg);
  } else {
    failures++;
    console.log('  ✗', msg);
  }
}

async function shot(page, name) {
  if (!shots) return;
  fs.mkdirSync(shots, { recursive: true });
  await page.screenshot({ path: path.join(shots, name + '.png') });
}

// Text of the browser terminal, read from xterm.js's buffer.
const screen = (page) => page.evaluate(() => window.__tuisenseDemo.screenText());

async function waitScreen(page, text, timeout = 15000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if ((await screen(page)).includes(text)) return true;
    await page.waitForTimeout(100);
  }
  console.log((await screen(page)).split('\n').slice(0, 6).join('\n'));
  return false;
}

// Pixel centre of the terminal cell holding text (first occurrence).
async function cellOf(page, text) {
  return page.evaluate((needle) => {
    const term = window.__tuisenseDemo.term;
    const b = term.buffer.active;
    for (let y = 0; y < term.rows; y++) {
      const line = b.getLine(b.viewportY + y);
      if (!line) continue;
      let s = '';
      const xs = [];
      for (let x = 0; x < term.cols; x++) {
        const c = line.getCell(x);
        if (!c || c.getWidth() === 0) continue;
        const ch = c.getChars() || ' ';
        for (let i = 0; i < ch.length; i++) xs.push(x);
        s += ch;
      }
      const i = s.indexOf(needle);
      if (i >= 0) {
        const el = term.element.querySelector('.xterm-screen').getBoundingClientRect();
        const w = el.width / term.cols;
        const h = el.height / term.rows;
        return { x: el.left + (xs[i] + 0.5) * w, y: el.top + (y + 0.5) * h, col: xs[i], row: y };
      }
    }
    return null;
  }, text);
}

(async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1600, height: 960 }, deviceScaleFactor: 1 });
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: base });
  const page = await context.newPage();
  const consoleErrors = [];
  page.on('console', (m) => m.type() === 'error' && consoleErrors.push(m.text()));
  page.on('pageerror', (e) => consoleErrors.push(String(e)));

  console.log('Landing page');
  await page.goto(base + '/');
  check((await page.textContent('h1')) === 'TuiSense', 'shows the TuiSense title');
  check(await page.isVisible('text=Your Controller. Your Terminal.'), 'shows the tagline');
  check(await page.isVisible('text=No installation required'), 'says no installation is required');
  for (const label of ['Launch Interactive Demo', 'View on GitHub', 'Download TuiSense']) {
    check(await page.isVisible(`text=${label}`), `has "${label}"`);
  }
  await page.waitForSelector(wasm ? 'text=runs the real app in your browser' : 'text=Live demo online');
  check(true, 'reports the live demo as available');
  await page.waitForTimeout(1200); // entrance animation and the recording
  await shot(page, '1-landing');

  console.log('Launch');
  await page.click('text=Launch Interactive Demo');
  await page.waitForURL('**/demo');
  // Reload with the test hook (the session resumes, as after any reload).
  await page.goto(base + '/demo?e2e');
  await page.waitForFunction(() => window.__tuisenseDemo && window.__tuisenseDemo.term.element);
  check(await waitScreen(page, 'WEB DEMO — SIMULATED CONTROLLERS', wasm ? 40000 : 15000), 'the real TuiSense draws its web demo indicator');
  const info = wasm
    ? { app: await (await page.request.get(base + '/wasm/build.json')).json(), limits: { minCols: 40, minRows: 12 } }
    : await (await page.request.get(base + '/api/info')).json();
  check(await waitScreen(page, 'v' + info.app.version), `the terminal shows TuiSense v${info.app.version}`);
  check(await page.isVisible(`text=TuiSense v${info.app.version}`), 'the demo bar shows the deployed version and commit');
  const text = await screen(page);
  check(/[⣿⣶⣤⣀]/.test(text), 'the 3D controller is rendered (braille)');
  check(text.includes('DualSense'), 'opens on the simulated DualSense');
  await page.waitForTimeout(800);
  await shot(page, '2-demo-home');

  console.log('Keyboard');
  await page.keyboard.press('?');
  check(await waitScreen(page, 'General navigation'), '? opens the native Help');
  await shot(page, '3-help');
  await page.keyboard.press('?');
  await page.waitForTimeout(300);
  await page.keyboard.press('2');
  check(await waitScreen(page, 'Stick'), '2 opens Diagnostics');
  await page.waitForTimeout(500);
  await shot(page, '4-diagnostics');
  await page.keyboard.press('1');

  console.log('Mouse');
  const pill = await cellOf(page, 'DualSense');
  await page.mouse.click(pill.x, pill.y);
  check(await waitScreen(page, 'Choose a controller'), 'clicking the controller pill opens the selector');
  const msi = await cellOf(page, 'GC20 V2 · simulated');
  await page.mouse.click(msi.x, msi.y);
  check(await waitScreen(page, 'GC20 V2'), 'clicking a model switches the controller');
  await page.waitForTimeout(400);
  check(!(await screen(page)).includes('Battery'), 'the MSI Force GC20 V2 shows no battery');
  await shot(page, '5-msi');

  const tabs = (await screen(page)).split('\n').slice(0, 4).join('\n');
  check(!tabs.includes('Customize') && tabs.includes('Diagnostics'), 'capability-aware tabs: nothing to customize on the GC20 V2');

  // The DualSense has lighting, vibration and adaptive triggers to customize.
  await page.keyboard.press('c');
  await waitScreen(page, 'Choose a controller');
  for (let i = 0; i < 10; i++) await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(400);
  await page.keyboard.press('3');
  check(await waitScreen(page, '[ Lights ]'), 'the DualSense offers Customize (EZ: Lights, Vibration, Triggers)');
  check(!(await screen(page)).includes('[ Sticks ]'), 'EZ mode hides the advanced pages');

  // EZ ↔ Advanced with the native mode switch (mouse) and E (keyboard).
  const adv = await cellOf(page, 'ADVANCED');
  await page.mouse.click(adv.x + 10, adv.y);
  check(await waitScreen(page, '[ Sticks ]', 3000), 'clicking ADVANCED shows the Advanced pages (Sticks, Motion, Audio)');
  await page.waitForTimeout(500);
  await shot(page, '6-dualsense-customize-advanced');
  await page.keyboard.press('e');
  check(await waitScreen(page, '[ Lights ]', 3000), 'E switches back to EZ mode');
  await page.keyboard.press('1');

  // Mouse wheel and drag reach the 3D stage.
  await page.keyboard.press('1');
  await page.waitForTimeout(300);
  const box = await page.locator('.term-frame').boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, -300);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 120, box.y + box.height / 2 + 30, { steps: 8 });
  await page.mouse.up();
  check(true, 'wheel and drag on the 3D stage are delivered');

  console.log('Model switching with the keyboard');
  await page.keyboard.press('c');
  await waitScreen(page, 'Choose a controller');
  for (let i = 0; i < 8; i++) await page.keyboard.press('ArrowDown'); // DualSense → Wolverine V3 Pro
  await page.keyboard.press('Enter');
  check(await waitScreen(page, 'Wolverine V3 Pro'), 'selector works with arrows and Enter');

  console.log('Resize');
  await page.setViewportSize({ width: 1100, height: 700 });
  await page.waitForTimeout(700);
  const cols = await page.evaluate(() => window.__tuisenseDemo.term.cols);
  check(cols < 160, `the terminal reflowed to ${cols} columns`);
  check(await waitScreen(page, 'WEB DEMO'), 'TuiSense redraws at the new size');
  const pill2 = await cellOf(page, 'Wolverine V3 Pro');
  await page.mouse.click(pill2.x, pill2.y);
  check(await waitScreen(page, 'Choose a controller'), 'mouse coordinates stay correct after resizing');
  await page.keyboard.press('Escape');
  await shot(page, '7-resized');

  console.log('Small screen');
  await page.setViewportSize({ width: 390, height: 700 });
  await page.waitForTimeout(700);
  const small = await page.evaluate(() => ({ cols: window.__tuisenseDemo.term.cols, rows: window.__tuisenseDemo.term.rows }));
  check(small.cols >= info.limits.minCols && small.rows >= info.limits.minRows, `never below the minimum (${small.cols}×${small.rows})`);
  await shot(page, '8-phone');
  await page.setViewportSize({ width: 1600, height: 960 });
  await page.waitForTimeout(700);

  console.log('Clipboard');
  await page.keyboard.press('y');
  await page.waitForTimeout(800);
  const clip = await page.evaluate(() => navigator.clipboard.readText().catch((e) => 'ERR ' + e));
  check(clip.includes('TuiSense') || clip.includes('Wolverine'), 'y copies the device report to the browser clipboard (OSC 52)');

  console.log('Restart / reset / exit');
  await page.click('text=Restart Demo');
  check(await waitScreen(page, 'WEB DEMO'), 'Restart Demo starts TuiSense again');
  await page.waitForTimeout(2200);
  page.once('dialog', (d) => d.accept());
  await page.click('text=Reset Session');
  check(await waitScreen(page, 'DualSense'), 'Reset Session starts fresh on the DualSense');
  await page.waitForTimeout(500);
  await page.locator('.term-frame').click();
  await page.keyboard.press('Escape');
  await waitScreen(page, 'Quit TuiSense?');
  await page.keyboard.press('Enter');
  await page.waitForSelector('text=TuiSense has exited');
  check(true, 'quitting shows the exit dialog (no shell)');
  check((await screen(page)).trim() === '', 'nothing of the exited process is left on screen');
  await shot(page, '9-exited');
  await page.waitForTimeout(2100);
  await page.click('text=Restart TuiSense');
  check(await waitScreen(page, 'WEB DEMO'), 'TuiSense restarts from the exit dialog');

  console.log('Fullscreen and back');
  await page.click('button:has-text("Fullscreen")');
  await page.waitForTimeout(400);
  const fsOK = await page.evaluate(() => !!document.fullscreenElement);
  console.log('    fullscreen element:', fsOK);
  if (fsOK) {
    check(await page.isVisible('text=Exit fullscreen'), 'fullscreen offers a way out (Esc stays with TuiSense)');
    await page.click('text=Exit fullscreen');
    await page.waitForTimeout(400);
    check(!(await page.evaluate(() => !!document.fullscreenElement)), 'leaves fullscreen');
  }
  if (wasm) {
    await page.click('text=Back to Website');
    await page.waitForURL(base + '/');
    check(await page.isVisible('text=Your Controller. Your Terminal.'), 'Back to Website returns to the landing page');
  } else {
    const before = (await (await page.request.get(base + '/api/info')).json()).sessions.active;
    await page.click('text=Back to Website');
    await page.waitForURL(base + '/');
    await page.waitForTimeout(500);
    const after = (await (await page.request.get(base + '/api/info')).json()).sessions.active;
    check(after === before - 1, `leaving ends the session on the server (${before} → ${after})`);
  }

  check(consoleErrors.length === 0, 'no browser console errors' + (consoleErrors.length ? ': ' + consoleErrors.join(' | ') : ''));
  await browser.close();
  console.log(failures ? `${failures} check(s) failed` : 'All end-to-end checks passed');
  process.exit(failures ? 1 : 0);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
