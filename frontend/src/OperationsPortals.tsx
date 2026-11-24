import { useMemo, useState } from 'react'
import { Activity, ArrowLeft, ArrowRight, Bike, Check, ChefHat, Clock3, MapPin, PackageCheck, Phone, Power, Route, Store, Timer, Truck, Utensils, Wallet } from 'lucide-react'
import './operations-portals.css'

type Stage = 'New' | 'Preparing' | 'Ready'
type KitchenOrder = { id: string; customer: string; placed: string; eta: number; amount: number; address: string; items: { name: string; qty: number }[]; stage: Stage }
type Delivery = { id: string; restaurant: string; customer: string; pickup: string; drop: string; distance: string; payout: number; status: 'Available' | 'To pickup' | 'To customer' | 'Delivered' }

const initialKitchen: KitchenOrder[] = [
  { id: 'ORD-2048', customer: 'Aarav Sharma', placed: '2 min ago', eta: 25, amount: 468, address: 'Indiranagar, Bengaluru', items: [{ name: 'Masala dosa', qty: 2 }, { name: 'Filter coffee', qty: 1 }] },
  { id: 'ORD-2047', customer: 'Diya Nair', placed: '6 min ago', eta: 30, amount: 329, address: 'Domlur, Bengaluru', items: [{ name: 'Paneer biryani', qty: 1 }, { name: 'Lassi', qty: 1 }] },
  { id: 'ORD-2046', customer: 'Kabir Rao', placed: '11 min ago', eta: 20, amount: 285, address: 'Ulsoor, Bengaluru', items: [{ name: 'Idli vada combo', qty: 2 }] }
].map((o, i) => ({ ...o, stage: (i === 0 ? 'New' : i === 1 ? 'Preparing' : 'Ready') as Stage }))

const initialDeliveries: Delivery[] = [
  { id: 'DLV-8102', restaurant: 'MTR 1924', customer: 'Ananya R', pickup: 'Lalbagh Road', drop: 'Jayanagar 4th Block', distance: '3.2 km', payout: 68, status: 'Available' },
  { id: 'DLV-8103', restaurant: 'The Rameshwaram Cafe', customer: 'Rahul K', pickup: 'Indiranagar 100ft Rd', drop: 'Domlur Layout', distance: '2.4 km', payout: 54, status: 'Available' },
  { id: 'DLV-8104', restaurant: 'Truffles', customer: 'Meera S', pickup: 'St Marks Road', drop: 'Richmond Town', distance: '4.1 km', payout: 82, status: 'Available' }
]

const rupees = (n: number) => new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 }).format(n)

function PortalHeader({ role }: { role: 'restaurant' | 'driver' }) {
  return <header className="ops-topbar"><a href="/" className="ops-brand"><span className="ops-logo"><Utensils size={18}/></span> crave<span>.</span></a><nav><a href="/">Customer app</a><a className={role === 'restaurant' ? 'selected' : ''} href="/restaurant">Restaurant</a><a className={role === 'driver' ? 'selected' : ''} href="/driver">Driver</a></nav><div className="ops-user"><span className="ops-avatar">{role === 'restaurant' ? 'M' : 'R'}</span><div><b>{role === 'restaurant' ? 'MTR 1924' : 'Rider account'}</b><small>{role === 'restaurant' ? 'Restaurant manager' : 'Delivery partner'}</small></div></div></header>
}

function RestaurantPortal() {
  const [orders, setOrders] = useState(initialKitchen)
  const [tab, setTab] = useState<'All' | Stage>('All')
  const [online, setOnline] = useState(true)
  const [toast, setToast] = useState('')
  const filtered = useMemo(() => tab === 'All' ? orders : orders.filter(o => o.stage === tab), [orders, tab])
  const update = (id: string, stage: Stage) => { setOrders(old => old.map(o => o.id === id ? { ...o, stage } : o)); setToast(`${id} moved to ${stage.toLowerCase()}.`) }
  const count = (stage: Stage) => orders.filter(o => o.stage === stage).length
  return <div className="ops-shell"><PortalHeader role="restaurant"/><main className="ops-main">
    <div className="ops-welcome"><div><div className="ops-kicker">RESTAURANT CONSOLE / BENGALURU</div><h1>Good evening, MTR team <span>✦</span></h1><p>Here’s what’s happening in your kitchen today.</p></div><button className={online ? 'online-toggle on' : 'online-toggle'} onClick={() => setOnline(!online)}><span/>{online ? 'Accepting orders' : 'Paused'} <Power size={15}/></button></div>
    <section className="ops-stats"><article><span className="stat-icon orange"><PackageCheck/></span><div><small>Orders today</small><strong>38</strong><em>↑ 12% vs yesterday</em></div></article><article><span className="stat-icon purple"><Timer/></span><div><small>Awaiting action</small><strong>{count('New')}</strong><em>Needs your attention</em></div></article><article><span className="stat-icon blue"><ChefHat/></span><div><small>In preparation</small><strong>{count('Preparing')}</strong><em>Kitchen queue</em></div></article><article><span className="stat-icon green"><Wallet/></span><div><small>Today's sales</small><strong>₹12,840</strong><em>Across 38 orders</em></div></article></section>
    <div className="ops-section-title"><div><h2>Order management</h2><p>Review incoming orders and keep customers updated.</p></div><span className="live-label"><i/> LIVE QUEUE</span></div>
    {toast && <div className="ops-toast"><Check size={16}/>{toast}<button onClick={() => setToast('')}>Dismiss</button></div>}
    <div className="ops-tabs">{(['All', 'New', 'Preparing', 'Ready'] as const).map(t => <button className={tab === t ? 'active' : ''} onClick={() => setTab(t)} key={t}>{t}<b>{t === 'All' ? orders.length : count(t as Stage)}</b></button>)}</div>
    <div className="kitchen-list">{filtered.map(order => <article className="kitchen-card" key={order.id}><div className="kitchen-card-top"><div><span className="order-id">{order.id}</span><span className={`stage-pill ${order.stage.toLowerCase()}`}>{order.stage}</span></div><span className="order-time"><Clock3 size={14}/>{order.placed}</span></div><div className="kitchen-content"><div className="kitchen-items"><h3>{order.customer}</h3>{order.items.map(item => <div className="kitchen-item" key={item.name}><b>{item.qty}×</b>{item.name}</div>)}<div className="kitchen-address"><MapPin size={14}/>{order.address}</div></div><div className="kitchen-summary"><small>ORDER TOTAL</small><strong>{rupees(order.amount)}</strong><small>Suggested prep time</small><b>{order.eta} min</b></div></div><div className="kitchen-actions">{order.stage === 'New' && <><button className="decline-btn" onClick={() => { setOrders(old => old.filter(o => o.id !== order.id)); setToast(`${order.id} rejected in this UI demo.`) }}>Reject</button><button className="ops-primary" disabled={!online} onClick={() => update(order.id, 'Preparing')}>Accept order <ArrowRight size={15}/></button></>}{order.stage === 'Preparing' && <button className="ops-primary" onClick={() => update(order.id, 'Ready')}>Mark ready for pickup <Check size={15}/></button>}{order.stage === 'Ready' && <span className="ready-note"><Check size={15}/> Waiting for delivery partner pickup</span>}</div></article>)}{!filtered.length && <div className="ops-empty">No {tab.toLowerCase()} orders right now.</div>}</div>
    <div className="ops-footnote">Demo UI · Actions update local sample data only. Connect these controls to the Order API to persist real orders.</div>
  </main></div>
}

function DriverPortal() {
  const [deliveries, setDeliveries] = useState(initialDeliveries)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [online, setOnline] = useState(true)
  const [toast, setToast] = useState('')
  const active = deliveries.find(d => d.id === activeId)
  const available = deliveries.filter(d => d.status === 'Available')
  const complete = deliveries.filter(d => d.status === 'Delivered').length
  const advance = () => {
    if (!active) return
    const next = active.status === 'To pickup' ? 'To customer' : 'Delivered'
    setDeliveries(old => old.map(d => d.id === active.id ? { ...d, status: next } : d))
    if (next === 'Delivered') { setToast(`${active.id} delivered. Nice work!`); setActiveId(null) }
    else setToast('Pickup confirmed. Head to the customer.')
  }
  return <div className="ops-shell"><PortalHeader role="driver"/><main className="ops-main">
    <div className="ops-welcome"><div><div className="ops-kicker">DELIVERY PARTNER / BENGALURU</div><h1>Ride smart, deliver happy <span>✦</span></h1><p>Your shift at a glance. Stay safe out there.</p></div><button className={online ? 'online-toggle on' : 'online-toggle'} onClick={() => setOnline(!online)}><span/>{online ? 'You’re online' : 'You’re offline'} <Power size={15}/></button></div>
    <section className="ops-stats"><article><span className="stat-icon orange"><Bike/></span><div><small>Deliveries today</small><strong>{6 + complete}</strong><em>Keep it going</em></div></article><article><span className="stat-icon purple"><Wallet/></span><div><small>Today's earnings</small><strong>₹486</strong><em>+ ₹{complete * 54} in this demo</em></div></article><article><span className="stat-icon blue"><Route/></span><div><small>Distance covered</small><strong>18.4 km</strong><em>Across completed trips</em></div></article><article><span className="stat-icon green"><Activity/></span><div><small>Acceptance rate</small><strong>94%</strong><em>Great performance</em></div></article></section>
    {toast && <div className="ops-toast"><Check size={16}/>{toast}<button onClick={() => setToast('')}>Dismiss</button></div>}
    {active ? <section className="active-delivery"><div className="active-heading"><div><div className="ops-kicker">YOUR ACTIVE DELIVERY</div><h2>{active.id}</h2></div><span className="stage-pill preparing">{active.status === 'To pickup' ? 'Go to restaurant' : 'On the way'}</span></div><div className="route-visual"><div className="route-stop"><span className="route-dot pickup"/><div><small>PICK UP FROM</small><b>{active.restaurant}</b><p>{active.pickup}</p></div></div><div className="route-connector"/><div className="route-stop"><span className="route-dot drop"/><div><small>DELIVER TO</small><b>{active.customer}</b><p>{active.drop}</p></div></div></div><div className="delivery-meta"><span><Route size={15}/>{active.distance}</span><span><Wallet size={15}/>{rupees(active.payout)} payout</span><span><Phone size={15}/>Customer contact</span></div><button className="ops-primary full" onClick={advance}>{active.status === 'To pickup' ? 'Confirm pickup' : 'Mark as delivered'} <ArrowRight size={16}/></button></section> : <section className="driver-map"><div className="map-art"><div className="map-roads"/><div className="map-pin pin-a"><Store size={17}/></div><div className="map-pin pin-b"><Bike size={17}/></div><div className="map-pin pin-c"><MapPin size={17}/></div><div className="map-route-line"/></div><div className="map-caption"><div><span className="live-label"><i/> YOUR DELIVERY ZONE</span><h3>Central Bengaluru</h3><p>Nearby orders will appear here when you’re online.</p></div><div className="zone-availability"><b>{available.length}</b><small>available<br/>orders</small></div></div></section>}
    <div className="ops-section-title"><div><h2>{active ? 'Next available orders' : 'Available deliveries'}</h2><p>Choose a delivery that works for your route.</p></div><span className="live-label"><i/> {online ? 'MATCHING LIVE' : 'OFFLINE'}</span></div>
    <div className="delivery-list">{deliveries.filter(d => d.status === 'Available').map(d => <article className="delivery-card" key={d.id}><div className="delivery-card-main"><div className="delivery-symbol"><Bike/></div><div><span className="order-id">{d.id}</span><h3>{d.restaurant}</h3><p><MapPin size={13}/>{d.pickup} <ArrowRight size={13}/> {d.drop}</p><small>{d.customer} · {d.distance}</small></div></div><div className="delivery-offer"><strong>{rupees(d.payout)}</strong><small>estimated payout</small><button disabled={!online || !!activeId} onClick={() => { setActiveId(d.id); setDeliveries(old => old.map(x => x.id === d.id ? { ...x, status: 'To pickup' } : x)); setToast(`You accepted ${d.id}. Head to pickup.`) }}>Accept <ArrowRight size={14}/></button></div></article>)}{!available.length && <div className="ops-empty">No available deliveries at the moment.</div>}</div>
    <div className="ops-footnote">Demo UI · Delivery offers and status updates use local sample data. Driver assignment and GPS tracking need backend integration.</div>
  </main></div>
}

export default function OperationsPortals() {
  return window.location.pathname.startsWith('/driver') ? <DriverPortal/> : <RestaurantPortal/>
}
