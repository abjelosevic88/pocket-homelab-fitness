// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { StartSheet } from './StartSheet'
import { LogDialog } from './LogDialog'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'pfn-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string, section?: string) => void>(() => {})
/** The page's name in the top bar, under a section other than the path's when given ("places"). */
export function useCrumb(title: string | undefined, section?: string) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || '', section); return () => set('') }, [title, section, set])
}

/** Open "Start a workout" from anywhere (a page's button). */
export const openStart = () => window.dispatchEvent(new CustomEvent('pfn:add'))
/** Open "Log a workout" (a past one, or written in a sentence). */
export const openLog = () => window.dispatchEvent(new CustomEvent('pfn:log'))

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const [sp, setSp] = useSearchParams()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const [crumbSection, setCrumbSection] = useState('')
  const setCrumb = useCallback((t: string, section?: string) => { setCrumbRaw(t); setCrumbSection(section || '') }, [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [adding, setAdding] = useState(false)
  const [logging, setLogging] = useState(false)
  const { overview } = useData()
  const active = overview?.active
  const add = () => { closeDrawer(); if (active) navigate(`/workouts/${active.id}`); else setAdding(true) }

  // /?start=1 (the PWA shortcut) opens the dialog
  useEffect(() => { if (sp.get('start')) { setAdding(true); setSp(new URLSearchParams(), { replace: true }) } }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = () => setAdding(true)
    const log = () => setLogging(true)
    window.addEventListener('pfn:shortcuts', open)
    window.addEventListener('pfn:add', make)
    window.addEventListener('pfn:log', log)
    return () => { window.removeEventListener('pfn:shortcuts', open); window.removeEventListener('pfn:add', make); window.removeEventListener('pfn:log', log) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': e.preventDefault(); add(); break
        case 'l': e.preventDefault(); setLogging(true); break
        case 't': go('/'); break
        case 'h': go('/history'); break
        case 'r': go('/routines'); break
        case 'e': go('/exercises'); break
        case 'p': go('/progress'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'start', label: active ? 'Back to your workout' : 'Start a workout', icon: 'play', hint: 'N', keywords: 'new begin gym session train', run: add },
    { id: 'log', label: 'Log a workout', icon: 'pen', hint: 'L', keywords: 'past earlier write sentence add yesterday', run: () => setLogging(true) },
    { id: 'today', label: 'Today', icon: 'today', hint: 'T', keywords: 'week plan home', run: () => navigate('/') },
    { id: 'history', label: 'History', icon: 'journal', hint: 'H', keywords: 'workouts past log sessions', run: () => navigate('/history') },
    { id: 'routines', label: 'Routines', icon: 'list', hint: 'R', keywords: 'templates program plan week', run: () => navigate('/routines') },
    { id: 'exercises', label: 'Exercises', icon: 'dumbbell', hint: 'E', keywords: 'library lifts movements', run: () => navigate('/exercises') },
    { id: 'progress', label: 'Progress', icon: 'chart', hint: 'P', keywords: 'records stats charts volume muscles weight', run: () => navigate('/progress') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'units', label: 'Settings: units and goal', icon: 'scale', keywords: 'kg lb pounds km miles rest timer week goal', run: () => navigate('/settings') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'layers', keywords: 'health habits calendar weather music core', run: () => navigate('/settings/apps') },
    { id: 'import', label: 'Settings: import from Strong or Hevy', icon: 'upload', keywords: 'csv export backup', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onStart={add} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} crumbSection={crumbSection} />
          <main className="content"><div className="content-inner"><Outlet /></div></main>
        </div>
        {mobile && !loc.pathname.startsWith('/workouts/') && <button type="button" className="fab" aria-label={active ? 'Back to your workout' : 'Start a workout'} title={active ? 'Back to your workout' : 'Start a workout'} onClick={add}><Icon name={active ? 'play' : 'plus'} size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {adding && <StartSheet onClose={() => setAdding(false)} onLog={() => { setAdding(false); setLogging(true) }} />}
        {logging && <LogDialog onClose={() => setLogging(false)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Today', '/'], workouts: ['History', '/history'], history: ['History', '/history'], routines: ['Routines', '/routines'], exercises: ['Exercises', '/exercises'], progress: ['Progress', '/progress'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb, crumbSection }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string; crumbSection: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[crumbSection || loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pfn:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
