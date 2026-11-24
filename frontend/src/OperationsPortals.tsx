import { useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowRight, Bike, Check, ChefHat, Clock3, LoaderCircle, MapPin, PackageCheck, Power, Route, Store, Timer, Utensils, Wallet } from 'lucide-react'
import './operations-portals.css'

const viteEnv = (import.meta as ImportMeta & { env: Record<string, string | undefined> }).env
const RESTAURANT_ID = viteEnv.VITE_RESTAURANT_ID || 'restaurant-1'
const PORTAL_TOKEN = viteEnv.VITE_PORTAL_ACCESS_TOKEN || 'dev-portal-token'

type OrderItem = { item_id: string; quantity: number }
type Address = { line1?: string; area?: string; city?: string; landmark?: string }
type RestaurantOrder = {
  order_id: string
  restaurant_id: string
  customer_id: string
  items: OrderItem[]
  delivery_address: Address
  total_amount_minor: number
  status: string
  created_at: string
  updated_at: string
}
type DriverAssignment = RestaurantOrder & {
  driver_id: string
  assignment_status: string
}
type PortalProfile = { restaurant_id?: string; accepting_orders?: boolean; driver_id?: string; status?: string }
type DriverDirectoryItem = { driver_id: string; status: string }

const rupees = (minor: number) => new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 }).format(minor / 100)
const pretty = (value: string) => value.replaceAll('_', ' ').toLowerCase().replace(/\b\w/g, c => c.toUpperCase())
const timeAgo = (value: string) => {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000))
  if (seconds < 60) return 'Just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} min ago`
  return `${Math.floor(seconds / 3600)} hr ago`
}
const addressText = (address?: Address) => [address?.line1, address?.area, address?.landmark, address?.city].filter(Boolean).join(', ') || 'Address not supplied'
const activeStatuses = new Set(['ASSIGNED', 'PICKED_UP'])
const finishedStatuses = new Set(['DELIVERED', 'RELEASED', 'DELIVERY_EXCEPTION'])

async function api<T>(url: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  headers.set('X-Portal-Token', PORTAL_TOKEN)
  const response = await fetch(url, { ...init, headers })
  const raw = await response.text()
  const payload = raw ? JSON.parse(raw) : {}
  if (!response.ok) throw new Error(payload.message || payload.error || `Request failed (${response.status})`)
  return payload as T
}

function liveStream(path: string, refresh: () => void) {
  const source = new EventSource(`${path}?token=${encodeURIComponent(PORTAL_TOKEN)}`)
  source.addEventListener('refresh', refresh)
  source.onerror = () => { /* EventSource automatically reconnects. */ }
  return () => source.close()
}

function PortalHeader({ role, driverId }: { role: 'restaurant' | 'driver'; driverId?: string }) {
  return <header className="ops-topbar"><a href="/" className="ops-brand"><span className="ops-logo"><Utensils size={18}/></span> crave<span>.</span></a><nav><a href="/">Customer app</a><a className={role === 'restaurant' ? 'selected' : ''} href="/restaurant">Restaurant</a><a className={role === 'driver' ? 'selected' : ''} href="/driver">Driver</a></nav><div className="ops-user"><span className="ops-avatar">{role === 'restaurant' ? 'R' : 'D'}</span><div><b>{role === 'restaurant' ? RESTAURANT_ID : driverId || 'Select a driver'}</b><small>{role === 'restaurant' ? 'Restaurant console' : 'Delivery partner'}</small></div></div></header>
}

function OrderBucket({ status }: { status: string }) {
  if (['PENDING', 'AWAITING_RESTAURANT_ACCEPTANCE'].includes(status)) return <span className="stage-pill new">{pretty(status)}</span>
  if (['RESTAURANT_ACCEPTED', 'DRIVER_ASSIGNED', 'PREPARING'].includes(status)) return <span className="stage-pill preparing">{pretty(status)}</span>
  if (['READY_FOR_PICKUP', 'OUT_FOR_DELIVERY'].includes(status)) return <span className="stage-pill ready">{pretty(status)}</span>
  return <span className="stage-pill">{pretty(status)}</span>
}

function RestaurantPortal() {
  const [orders, setOrders] = useState<RestaurantOrder[]>([])
  const [accepting, setAccepting] = useState(true)
  const [tab, setTab] = useState('All')
  const [busy, setBusy] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const load = useCallback(async () => {
    try {
      const [queue, profile] = await Promise.all([
        api<{ orders: RestaurantOrder[] }>(`/restaurant-api/v1/portal/restaurants/${RESTAURANT_ID}/orders`),
        api<PortalProfile>(`/restaurant-api/v1/portal/restaurants/${RESTAURANT_ID}`)
      ])
      setOrders(queue.orders || [])
      setAccepting(profile.accepting_orders !== false)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to load restaurant orders')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
    return liveStream(`/restaurant-api/v1/portal/restaurants/${RESTAURANT_ID}/events`, () => { void load() })
  }, [load])

  const setAvailability = async () => {
    try {
      const next = !accepting
      await api(`/restaurant-api/v1/portal/restaurants/${RESTAURANT_ID}/availability`, { method: 'POST', body: JSON.stringify({ accepting_orders: next }) })
      setAccepting(next)
      setNotice(next ? 'Your restaurant is accepting new orders.' : 'New orders are paused.')
    } catch (e) { setNotice(e instanceof Error ? e.message : 'Could not update availability') }
  }

  const transition = async (orderId: string, action: 'accept' | 'reject' | 'ready') => {
    setBusy(orderId)
    try {
      await api(`/restaurant-api/v1/portal/restaurants/${RESTAURANT_ID}/orders/${orderId}/${action}`, { method: 'POST', body: '{}' })
      setNotice(action === 'ready' ? `${orderId} marked ready. Driver will be notified by the order workflow.` : `${orderId} ${action}ed. Temporal workflow notified.`)
      await load()
    } catch (e) { setNotice(e instanceof Error ? e.message : `Unable to ${action} order`) }
    finally { setBusy(null) }
  }

  const bucket = (status: string) => ['PENDING', 'AWAITING_RESTAURANT_ACCEPTANCE'].includes(status) ? 'New'
    : ['RESTAURANT_ACCEPTED', 'DRIVER_ASSIGNED', 'PREPARING'].includes(status) ? 'Preparing'
    : ['READY_FOR_PICKUP', 'OUT_FOR_DELIVERY'].includes(status) ? 'Ready' : 'Completed'
  const filtered = useMemo(() => tab === 'All' ? orders : orders.filter(order => bucket(order.status) === tab), [orders, tab])
  const newCount = orders.filter(o => bucket(o.status) === 'New').length
  const preparingCount = orders.filter(o => bucket(o.status) === 'Preparing').length
  const todayTotal = orders.reduce((n, o) => n + o.total_amount_minor, 0)

  return <div className="ops-shell"><PortalHeader role="restaurant"/><main className="ops-main">
    <div className="ops-welcome"><div><div className="ops-kicker">RESTAURANT CONSOLE / BENGALURU</div><h1>Order desk <span>✦</span></h1><p>Orders and status are read from the live Order API.</p></div><button className={accepting ? 'online-toggle on' : 'online-toggle'} onClick={setAvailability}><span/>{accepting ? 'Accepting orders' : 'Paused'} <Power size={15}/></button></div>
    <section className="ops-stats"><article><span className="stat-icon orange"><PackageCheck/></span><div><small>Orders in queue</small><strong>{orders.length}</strong><em>Last 100 orders</em></div></article><article><span className="stat-icon purple"><Timer/></span><div><small>Awaiting action</small><strong>{newCount}</strong><em>Needs your attention</em></div></article><article><span className="stat-icon blue"><ChefHat/></span><div><small>In preparation</small><strong>{preparingCount}</strong><em>Kitchen queue</em></div></article><article><span className="stat-icon green"><Wallet/></span><div><small>Order value in queue</small><strong>{rupees(todayTotal)}</strong><em>Not a settlement statement</em></div></article></section>
    <div className="ops-section-title"><div><h2>Order management</h2><p>Accept or reject orders, then mark them ready for pickup.</p></div><span className="live-label"><i/> SSE CONNECTED / AUTO REFRESH</span></div>
    {notice && <div className="ops-toast"><Check size={16}/>{notice}<button onClick={() => setNotice('')}>Dismiss</button></div>}
    {error && <div className="ops-error">{error}<button onClick={() => void load()}>Retry</button></div>}
    <div className="ops-tabs">{['All', 'New', 'Preparing', 'Ready', 'Completed'].map(t => <button className={tab === t ? 'active' : ''} onClick={() => setTab(t)} key={t}>{t}<b>{t === 'All' ? orders.length : orders.filter(o => bucket(o.status) === t).length}</b></button>)}</div>
    {loading ? <div className="ops-empty"><LoaderCircle className="spin"/> Loading orders…</div> : <div className="kitchen-list">{filtered.map(order => <article className="kitchen-card" key={order.order_id}>
      <div className="kitchen-card-top"><div><span className="order-id">{order.order_id}</span><OrderBucket status={order.status}/></div><span className="order-time"><Clock3 size={14}/>{timeAgo(order.created_at)}</span></div>
      <div className="kitchen-content"><div className="kitchen-items"><h3>Customer · {order.customer_id}</h3>{order.items.map(item => <div className="kitchen-item" key={item.item_id}><b>{item.quantity}×</b>{item.item_id}</div>)}<div className="kitchen-address"><MapPin size={14}/>{addressText(order.delivery_address)}</div></div><div className="kitchen-summary"><small>ORDER TOTAL</small><strong>{rupees(order.total_amount_minor)}</strong><small>Last updated</small><b>{timeAgo(order.updated_at)}</b></div></div>
      <div className="kitchen-actions">{bucket(order.status) === 'New' && <><button className="decline-btn" disabled={busy === order.order_id} onClick={() => void transition(order.order_id, 'reject')}>Reject</button><button className="ops-primary" disabled={!accepting || busy === order.order_id} onClick={() => void transition(order.order_id, 'accept')}>{busy === order.order_id ? 'Updating…' : 'Accept order'} <ArrowRight size={15}/></button></>}{order.status === 'PREPARING' && <button className="ops-primary" disabled={busy === order.order_id} onClick={() => void transition(order.order_id, 'ready')}>Mark ready for pickup <Check size={15}/></button>}{bucket(order.status) === 'Ready' && <span className="ready-note"><Check size={15}/> Driver pickup is managed by the workflow</span>}{bucket(order.status) === 'Completed' && <span className="ready-note">{pretty(order.status)}</span>}</div>
    </article>)}{!filtered.length && <div className="ops-empty">No {tab.toLowerCase()} orders for {RESTAURANT_ID}.</div>}</div>}
    <div className="ops-footnote">Local demo portal identity: {RESTAURANT_ID}. Replace the shared development token with authenticated restaurant sessions before deployment.</div>
  </main></div>
}

function DriverPortal() {
  const [drivers, setDrivers] = useState<DriverDirectoryItem[]>([])
  const [driverId, setDriverId] = useState('')
  const [assignments, setAssignments] = useState<DriverAssignment[]>([])
  const [online, setOnline] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [directoryLoading, setDirectoryLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const loadDrivers = useCallback(async (preferredId?: string) => {
    const result = await api<{ drivers: DriverDirectoryItem[] }>('/driver-api/v1/portal/drivers')
    const nextDrivers = result.drivers || []
    setDrivers(nextDrivers)
    setDriverId(current => {
      if (preferredId && nextDrivers.some(d => d.driver_id === preferredId)) return preferredId
      if (current && nextDrivers.some(d => d.driver_id === current)) return current
      return nextDrivers.find(d => d.status === 'AVAILABLE')?.driver_id || nextDrivers[0]?.driver_id || ''
    })
  }, [])

  useEffect(() => {
    void loadDrivers().catch(e => setError(e instanceof Error ? e.message : 'Unable to load registered drivers'))
      .finally(() => setDirectoryLoading(false))
  }, [loadDrivers])

  const load = useCallback(async () => {
    if (!driverId) {
      setAssignments([])
      setOnline(false)
      setLoading(false)
      return
    }
    setLoading(true)
    try {
      const [result, profile] = await Promise.all([
        api<{ assignments: DriverAssignment[] }>(`/driver-api/v1/portal/drivers/${encodeURIComponent(driverId)}/assignments`),
        api<PortalProfile>(`/driver-api/v1/portal/drivers/${encodeURIComponent(driverId)}`)
      ])
      setAssignments(result.assignments || [])
      setOnline(profile.status === 'AVAILABLE')
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to load driver assignments')
    } finally { setLoading(false) }
  }, [driverId])

  useEffect(() => {
    if (!driverId) return
    void load()
    return liveStream(`/driver-api/v1/portal/drivers/${encodeURIComponent(driverId)}/events`, () => { void load() })
  }, [driverId, load])

  const selectDriver = (nextId: string) => {
    setDriverId(nextId)
    setNotice('')
    setError('')
    setAssignments([])
    setLoading(Boolean(nextId))
  }

  const setAvailability = async () => {
    if (!driverId) return
    try {
      const next = !online
      await api(`/driver-api/v1/portal/drivers/${encodeURIComponent(driverId)}/availability`, { method: 'POST', body: JSON.stringify({ available: next }) })
      setOnline(next)
      setNotice(next ? 'You are available for automatic assignment.' : 'You are now offline.')
      await loadDrivers(driverId)
    } catch (e) { setNotice(e instanceof Error ? e.message : 'Could not update availability') }
  }

  const action = async (orderId: string, next: 'picked-up' | 'delivered') => {
    if (!driverId) return
    setBusy(orderId)
    try {
      await api(`/driver-api/v1/portal/drivers/${encodeURIComponent(driverId)}/assignments/${orderId}/${next}`, { method: 'POST', body: '{}' })
      setNotice(next === 'picked-up' ? 'Pickup confirmed. Order workflow updated.' : 'Delivery confirmed. Order workflow completed.')
      await Promise.all([load(), loadDrivers(driverId)])
    } catch (e) { setNotice(e instanceof Error ? e.message : 'Unable to update assignment') }
    finally { setBusy(null) }
  }

  const active = assignments.filter(a => activeStatuses.has(a.assignment_status))
  const completed = assignments.filter(a => finishedStatuses.has(a.assignment_status))
  const totalMinor = assignments.reduce((n, a) => n + a.total_amount_minor, 0)

  return <div className="ops-shell"><PortalHeader role="driver" driverId={driverId}/><main className="ops-main">
    <div className="ops-welcome"><div><div className="ops-kicker">DELIVERY PARTNER / BENGALURU</div><h1>Your delivery desk <span>✦</span></h1><p>Choose a registered driver. Assignments are allocated by the Temporal order workflow.</p></div><button className={online ? 'online-toggle on' : 'online-toggle'} onClick={setAvailability} disabled={!driverId || loading}><span/>{online ? 'You’re available' : 'You’re offline'} <Power size={15}/></button></div>
    <section className="driver-picker"><label htmlFor="driver-select">Working as</label><select id="driver-select" value={driverId} onChange={e => selectDriver(e.target.value)} disabled={directoryLoading || drivers.length === 0}><option value="">{directoryLoading ? 'Loading drivers…' : 'No registered drivers'}</option>{drivers.map(driver => <option key={driver.driver_id} value={driver.driver_id}>{driver.driver_id} · {pretty(driver.status)}</option>)}</select><button type="button" onClick={() => { setDirectoryLoading(true); void loadDrivers(driverId).catch(e => setError(e instanceof Error ? e.message : 'Unable to refresh drivers')).finally(() => setDirectoryLoading(false)) }}>Refresh list</button></section>
    <section className="ops-stats"><article><span className="stat-icon orange"><Bike/></span><div><small>Assigned orders</small><strong>{assignments.length}</strong><em>For this driver</em></div></article><article><span className="stat-icon purple"><Route/></span><div><small>Active deliveries</small><strong>{active.length}</strong><em>Current workload</em></div></article><article><span className="stat-icon blue"><Check/></span><div><small>Completed</small><strong>{completed.length}</strong><em>Confirmed deliveries</em></div></article><article><span className="stat-icon green"><Wallet/></span><div><small>Order value handled</small><strong>{rupees(totalMinor)}</strong><em>Not driver earnings</em></div></article></section>
    {notice && <div className="ops-toast"><Check size={16}/>{notice}<button onClick={() => setNotice('')}>Dismiss</button></div>}
    {error && <div className="ops-error">{error}<button onClick={() => void loadDrivers(driverId).then(() => void load()).catch(e => setError(e instanceof Error ? e.message : 'Unable to reload'))}>Retry</button></div>}
    <div className="ops-section-title"><div><h2>Your assignments</h2><p>Confirm pickup only after the restaurant marks the order ready.</p></div>{driverId && <span className="live-label"><i/> SSE CONNECTED / AUTO REFRESH</span>}</div>
    {directoryLoading || loading ? <div className="ops-empty"><LoaderCircle className="spin"/> Loading driver data…</div> : !drivers.length ? <div className="ops-empty">No drivers are registered yet. Add drivers through onboarding or run the optional local seed script described in the README.</div> : <div className="delivery-list">{assignments.map(item => {
      const canPickup = item.assignment_status === 'ASSIGNED' && item.status === 'READY_FOR_PICKUP'
      const canDeliver = item.assignment_status === 'PICKED_UP' && item.status === 'OUT_FOR_DELIVERY'
      return <article className="delivery-card" key={item.order_id}><div className="delivery-card-main"><div className="delivery-symbol"><Bike/></div><div><span className="order-id">{item.order_id}</span><h3>Restaurant · {item.restaurant_id}</h3><p><Store size={13}/> {pretty(item.assignment_status)} <ArrowRight size={13}/> {pretty(item.status)}</p><small>Deliver to {item.customer_id} · {addressText(item.delivery_address)}</small><div className="driver-order-total">{item.items.reduce((n, i) => n + i.quantity, 0)} items · {rupees(item.total_amount_minor)}</div></div></div><div className="delivery-offer"><OrderBucket status={item.status}/>{canPickup && <button disabled={busy === item.order_id} onClick={() => void action(item.order_id, 'picked-up')}>Confirm pickup <ArrowRight size={14}/></button>}{canDeliver && <button disabled={busy === item.order_id} onClick={() => void action(item.order_id, 'delivered')}>Mark delivered <Check size={14}/></button>}{!canPickup && !canDeliver && <small>{finishedStatuses.has(item.assignment_status) ? 'Trip completed' : item.status === 'PREPARING' ? 'Waiting for kitchen' : 'Waiting for workflow'}</small>}</div></article>
    })}{!assignments.length && <div className="ops-empty">{online ? 'You are available. New assignments appear automatically after restaurant acceptance.' : 'No assignments for this driver yet. Go online to receive delivery assignments.'}</div>}</div>}
    <div className="ops-footnote">Driver identity is selected from registered database records. Production deployment must bind this portal to the authenticated driver account.</div>
  </main></div>
}

export default function OperationsPortals() {
  return window.location.pathname.startsWith('/driver') ? <DriverPortal/> : <RestaurantPortal/>
}
