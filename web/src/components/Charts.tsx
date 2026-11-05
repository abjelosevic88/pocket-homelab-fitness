// Two small charts drawn as SVG: a line (best per session, weight) and
// bars (workouts per week). No library; they scale to their box.

export function LineChart({ points, fmt, height = 150, label }: { points: { date: string; v: number }[]; fmt: (v: number) => string; height?: number; label: string }) {
  if (points.length < 2) return <p className="muted small">Not enough sessions for a line yet.</p>
  const W = 600, H = height, P = { l: 8, r: 8, t: 18, b: 4 }
  const vs = points.map(p => p.v)
  let lo = Math.min(...vs), hi = Math.max(...vs)
  if (hi - lo < 1e-6) { lo -= 1; hi += 1 }
  const pad = (hi - lo) * 0.12
  lo -= pad; hi += pad
  const t0 = new Date(points[0].date).getTime(), t1 = new Date(points[points.length - 1].date).getTime() || t0 + 1
  const x = (d: string) => P.l + (W - P.l - P.r) * ((new Date(d).getTime() - t0) / Math.max(1, t1 - t0))
  const y = (v: number) => P.t + (H - P.t - P.b) * (1 - (v - lo) / (hi - lo))
  const path = points.map((p, i) => `${i ? 'L' : 'M'}${x(p.date).toFixed(1)} ${y(p.v).toFixed(1)}`).join(' ')
  const area = `${path} L${x(points[points.length - 1].date).toFixed(1)} ${H - P.b} L${x(points[0].date).toFixed(1)} ${H - P.b} Z`
  const best = points.reduce((m, p) => p.v > m.v ? p : m, points[0])
  const last = points[points.length - 1]
  const month = (d: string) => new Date(d + 'T12:00').toLocaleDateString(undefined, { month: 'short' })
  const ticks = points.filter((p, i) => i === 0 || month(p.date) !== month(points[i - 1].date))
  const dot = (d: string, v: number, cls: string, t: string) => <span className={`ch-dot ${cls}`} title={t} style={{ left: `${x(d) / W * 100}%`, top: `${y(v)}px` }} />
  return (
    <div className="chart-wrap" role="img" aria-label={`${label}: from ${fmt(points[0].v)} to ${fmt(last.v)}, best ${fmt(best.v)}`}>
      <svg className="chart line" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" aria-hidden="true" style={{ height: H }}>
        <path d={area} className="ch-area" />
        <path d={path} className="ch-line" vectorEffect="non-scaling-stroke" />
      </svg>
      {dot(best.date, best.v, 'best', `Best: ${fmt(best.v)}`)}
      {dot(last.date, last.v, 'last', `Latest: ${fmt(last.v)}`)}
      <span className="ch-val" style={{ top: `${y(best.v)}px` }}>{fmt(best.v)}</span>
      {ticks.map(t => <span key={t.date} className="ch-tick" style={{ left: `${x(t.date) / W * 100}%` }}>{month(t.date)}</span>)}
    </div>
  )
}

export function Bars({ rows, goal, label }: { rows: { key: string; label: string; a: number; b?: number; title: string }[]; goal?: number; label: string }) {
  const max = Math.max(goal || 0, ...rows.map(r => r.a + (r.b || 0)), 1)
  return (
    <div className="bars" role="img" aria-label={label}>
      {goal ? <span className="bars-goal" style={{ bottom: `calc(18px + ${goal / max} * (100% - 18px))` }} title={`Goal: ${goal}`} /> : null}
      {rows.map(r => (
        <div key={r.key} className="bar-col" title={r.title}>
          <div className="bar-stack">
            {!!r.b && <span className="bar b" style={{ height: `${r.b / max * 100}%` }} />}
            <span className="bar a" style={{ height: `${r.a / max * 100}%` }} />
          </div>
          <span className="bars-lbl">{r.label}</span>
        </div>
      ))}
    </div>
  )
}
