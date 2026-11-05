// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true: four routines on
// a weekly plan, six months of workouts with the weights going up, and made-up linked apps: runs from
// "Pocket Health", the weather, habits and playlists):
//
//   DATA_DIR=/tmp/pfndemo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Amar PORT=18310 ./pocket-fitness
//   CDP=http://127.0.0.1:9337 BASE=http://127.0.0.1:18310 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18310'
const shots = process.env.SHOTS !== '0'
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1000) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name, full = false) { if (shots) await page.screenshot({ path: dir + name + '.png', fullPage: full }) }
const api = (page, method, path, body) => page.evaluate(([m, p, b]) => fetch('/api/v1' + p, { method: m, headers: { 'Content-Type': 'application/json' }, body: b ? JSON.stringify(b) : undefined }).then(r => r.status === 204 ? null : r.json()), [method, path, body])

// ---- desktop ----
let benchId, workoutId
{
  const { ctx, page } = await session()
  await go(page, '/', 1500)
  const html = await page.content()
  assert.ok(html.includes('This week') && html.includes('Recent workouts') && html.includes('New records'), 'today shows the week, workouts and records')
  await shot(page, 'desktop-today')
  // a running workout: the leg day, two exercises ticked
  const routines = await api(page, 'GET', '/routines')
  const legs = routines.find(r => r.name === 'Leg day')
  const started = await api(page, 'POST', '/workouts', { routine_id: legs.id })
  workoutId = started.workout.id
  const sets = started.workout.sets.map((s, i) => ({ ...s, done: i < 6 }))
  await api(page, 'PUT', `/workouts/${workoutId}`, { sets })
  await go(page, `/workouts/${workoutId}`, 1200)
  assert.ok((await page.content()).includes('Finish'), 'a running workout has Finish')
  await page.locator('.set-check:not(.on)').first().click()
  await page.waitForTimeout(400)
  await shot(page, 'desktop-workout')
  await go(page, '/history')
  await shot(page, 'desktop-history')
  await go(page, '/routines')
  assert.ok((await page.content()).includes('Your week'), 'routines show the week')
  await shot(page, 'desktop-routines')
  const ex = await api(page, 'GET', '/exercises')
  benchId = ex.find(e => e.name === 'Bench Press (Barbell)').id
  await go(page, `/exercises/${benchId}`)
  assert.ok((await page.content()).includes('Heaviest'), 'an exercise shows its records')
  await shot(page, 'desktop-exercise')
  await go(page, '/progress', 1200)
  assert.ok((await page.content()).includes('Sets per muscle'), 'progress shows the muscles')
  await shot(page, 'desktop-progress')
  await go(page, '/settings/apps')
  await shot(page, 'desktop-settings-apps')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1500)
  await shot(page, 'phone-today')
  await go(page, `/workouts/${workoutId}`, 1200)
  await shot(page, 'phone-workout')
  await go(page, '/progress', 1200)
  await shot(page, 'phone-progress')
  // nothing is wider than the phone
  for (const p of ['/', `/workouts/${workoutId}`, '/history', '/routines', `/exercises/${benchId}`, '/progress', '/settings/apps']) {
    await go(page, p, 600)
    const w = await page.evaluate(() => document.documentElement.scrollWidth)
    assert.ok(w <= 391, `${p} is ${w}px wide on a phone`)
  }
  await ctx.close()
}
{
  const { ctx, page } = await session({ phone: true, dark: true })
  await go(page, `/exercises/${benchId}`, 1200)
  await shot(page, 'phone-exercise-dark')
  await go(page, '/routines', 1000)
  await shot(page, 'phone-routines-dark')
  // leave the demo as it was: no workout running
  await api(page, 'DELETE', `/workouts/${workoutId}`)
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
