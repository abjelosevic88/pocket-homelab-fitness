// Left column: start (or go back to) a workout, the sections, and your
// routines — the Pocket Calendar sidebar.
import { Link, NavLink } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { Icon } from './icons'

export function Sidebar({ onStart, onNavigate }: { onStart: () => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { overview } = useData()
  const title = settings?.title || status?.title || 'Pocket Fitness'
  const active = overview?.active
  const week = overview?.week
  const nav: { to: string; label: string; icon: string; end?: boolean; count?: string }[] = [
    { to: '/', label: 'Today', icon: 'today', end: true, count: week && week.goal > 0 ? `${week.done}/${week.goal}` : undefined },
    { to: '/history', label: 'History', icon: 'journal' },
    { to: '/routines', label: 'Routines', icon: 'list' },
    { to: '/exercises', label: 'Exercises', icon: 'dumbbell' },
    { to: '/progress', label: 'Progress', icon: 'chart' },
  ]
  const routines = overview?.routines || []
  return (
    <aside className="sidebar" aria-label="Pocket Fitness">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Fitness' ? title : <>Pocket <b>Fitness</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      {active
        ? <Link to={`/workouts/${active.id}`} className="btn primary create-btn" onClick={onNavigate}><Icon name="play" size={15} strokeWidth={2.2} />Back to your workout</Link>
        : <button type="button" className="btn primary create-btn" onClick={onStart}><Icon name="plus" size={15} strokeWidth={2.2} />Start a workout</button>}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${week && week.done >= week.goal ? 'done' : ''}`} title="Workouts this week, of your goal">{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {routines.length > 0 && <>
        <div className="sb-section">Your routines</div>
        <nav className="sb-nav sb-routines" aria-label="Your routines">
          {routines.map(r => (
            <NavLink key={r.id} to={`/routines/${r.id}`} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
              <Icon name={r.outdoors ? 'sun' : 'dumbbell'} size={15} className="sb-list-ico" />
              <span className="ellipsis grow">{r.name}</span>
              {!!r.weekdays?.length && <span className="sb-days muted small">{r.weekdays.map(d => 'MTWTFSS'[d - 1]).join('')}</span>}
            </NavLink>
          ))}
        </nav>
      </>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pfn:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
