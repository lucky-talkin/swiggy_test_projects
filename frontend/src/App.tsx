import { useMemo, useState } from 'react'
import { skipToken } from '@reduxjs/toolkit/query'
import { useDispatch, useSelector } from 'react-redux'
import { ArrowLeft, ArrowRight, Bike, Clock3, LoaderCircle, MapPin, Minus, Plus, Search, ShoppingBag, Star, Utensils, X } from 'lucide-react'
import type { RootState, AppDispatch } from './app/store'
import { addToCart, changeQuantity, clearCart } from './features/cart/cartSlice'
import { useCancelOrderMutation, useCreateOrderMutation, useGetOrderQuery, useSearchFoodQuery } from './services/foodApi'
import type { FoodItem, Order } from './types'

const money = (value: number) => new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 }).format(value)
const errorMessage = (error: unknown) => {
  if (error && typeof error === 'object' && 'data' in error) {
    const data = (error as { data?: { message?: string; error?: string } }).data
    return data?.message || data?.error || 'The request failed. Please try again.'
  }
  return 'Unable to reach the API. Check that the backend services are running.'
}

export default function App() {
  const dispatch = useDispatch<AppDispatch>()
  const cart = useSelector((state: RootState) => state.cart.items)
  const [city, setCity] = useState('Bengaluru')
  const [search, setSearch] = useState('')
  const [selectedRestaurant, setSelectedRestaurant] = useState<FoodItem | null>(null)
  const [cartOpen, setCartOpen] = useState(false)
  const [checkoutOpen, setCheckoutOpen] = useState(false)
  const [customerId, setCustomerId] = useState('customer-1')
  const [address, setAddress] = useState('MG Road')
  const [orderId, setOrderId] = useState<string | null>(null)
  const [notice, setNotice] = useState('')
  const [noticeError, setNoticeError] = useState(false)

  const searchParams = useMemo(() => ({
    q: search.trim() || undefined,
    city: city.trim() || undefined,
    type: selectedRestaurant ? 'menu_item' as const : 'restaurant' as const,
    restaurant_id: selectedRestaurant?.id,
    page: 1,
    page_size: 50
  }), [search, city, selectedRestaurant])
  const { data, isLoading, isFetching, error, refetch } = useSearchFoodQuery(searchParams)
  const [createOrder, createState] = useCreateOrderMutation()
  const [cancelOrder, cancelState] = useCancelOrderMutation()
  const { data: order, error: orderError, isFetching: orderFetching } = useGetOrderQuery(orderId ?? skipToken, {
    pollingInterval: orderId ? 4000 : 0,
    skipPollingIfUnfocused: true
  })

  const foods = data?.items ?? []
  const count = cart.reduce((sum, item) => sum + item.quantity, 0)
  const total = cart.reduce((sum, item) => sum + item.priceRupees * item.quantity, 0)
  const showNotice = (message: string, isError = false) => { setNotice(message); setNoticeError(isError) }

  function add(item: FoodItem) {
    if (!selectedRestaurant || !item.id || !item.restaurant_id || typeof item.price !== 'number') {
      showNotice('This menu result is missing its item ID, restaurant association or price.', true)
      return
    }
    if (item.restaurant_id !== selectedRestaurant.id) {
      showNotice('This dish belongs to a different restaurant.', true)
      return
    }
    if (cart.length && cart[0].restaurantId !== item.restaurant_id) {
      showNotice('Your cart contains dishes from another restaurant. Clear it before ordering here.', true)
      return
    }
    dispatch(addToCart({ itemId: item.id, restaurantId: item.restaurant_id, name: item.name, priceRupees: item.price }))
    showNotice(item.name + ' added to cart.')
  }

  async function placeOrder() {
    if (!cart.length || !customerId.trim() || !address.trim()) return
    try {
      const result = await createOrder({
        customer_id: customerId.trim(),
        restaurant_id: cart[0].restaurantId,
        items: cart.map(item => ({ item_id: item.itemId, quantity: item.quantity })),
        delivery_address: { line1: address.trim(), city: city.trim() },
        total_amount_minor: Math.round(total * 100),
        currency: 'INR'
      }).unwrap()
      setOrderId(result.order_id)
      dispatch(clearCart())
      setCheckoutOpen(false)
      setCartOpen(false)
      showNotice('Order placed successfully. Tracking status will update automatically.')
    } catch (error) {
      showNotice(errorMessage(error), true)
    }
  }

  async function onCancelOrder() {
    if (!orderId) return
    try {
      await cancelOrder({ id: orderId, reason: 'cancelled_by_customer' }).unwrap()
      showNotice('Cancellation requested. Waiting for workflow confirmation.')
    } catch (error) {
      showNotice(errorMessage(error), true)
    }
  }

  const orderStatus = (order as Order | undefined)?.status
  const terminal = ['DELIVERED', 'REJECTED', 'CANCELLED', 'FAILED', 'COMPENSATION_FAILED'].includes(orderStatus ?? '')

  return <div className="app-shell">
    <header className="topbar">
      <a className="brand" href="#"><span className="brand-mark"><Utensils size={19}/></span><span>crave<span className="brand-dot">.</span></span></a>
      <div className="location"><MapPin size={18}/><div><strong>Delivering to</strong><input aria-label="Delivery city" value={city} onChange={e => setCity(e.target.value)} /></div></div>
      <button className="cart-button" onClick={() => setCartOpen(true)}><ShoppingBag size={18}/><span>Cart</span><b>{count}</b></button>
    </header>

    <main>
      <section className="hero">
        <div className="hero-copy"><span className="eyebrow">GOOD FOOD, GOOD MOOD</span><h1>Your next meal<br/><em>is a few clicks away.</em></h1><p>Discover local favourites, explore their menus and get food delivered.</p>
          <div className="searchbar"><Search size={20}/><input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search restaurants in your city..." /><button onClick={() => refetch()}>Search</button></div>
        </div>
        <div className="hero-art"><div className="art-circle"><span>✦</span><div className="food-emoji">🍜</div><small>made with<br/>love</small></div><div className="floating-note"><Bike size={18}/><span>Fresh food<br/><b>at your door</b></span></div></div>
      </section>

      <section className="browse-section">
        <div className="section-heading"><div>{selectedRestaurant && <button className="back-button" onClick={() => { setSelectedRestaurant(null); setSearch(''); showNotice('') }}><ArrowLeft size={15}/> All restaurants</button>}<span className="eyebrow">{selectedRestaurant ? 'EXPLORE THE MENU' : 'FIND YOUR FAVOURITE'}</span><h2>{selectedRestaurant ? selectedRestaurant.name : 'Restaurants near you'}</h2>{selectedRestaurant && <p className="restaurant-subtitle">{selectedRestaurant.cuisines?.join(' · ')} · {selectedRestaurant.area} · {selectedRestaurant.delivery_time_minutes} min delivery</p>}</div></div>
        {notice && <div className={noticeError ? 'notice notice-error' : 'notice'}><span>{notice}</span><button onClick={() => setNotice('')}><X size={16}/></button></div>}
        {error && <div className="error-panel"><strong>Search API request failed.</strong><p>{errorMessage(error)}</p><button className="retry-button" onClick={() => refetch()}>Retry</button></div>}
        {isLoading && <div className="loading"><LoaderCircle className="spin"/> Loading {selectedRestaurant ? 'menu' : 'restaurants'}...</div>}
        {!isLoading && !error && foods.length === 0 && <div className="empty"><span>🥡</span><h3>{selectedRestaurant ? 'No menu items found' : 'No restaurants found'}</h3><p>{selectedRestaurant ? 'This restaurant has no matching menu items.' : 'Try another search or check your city.'}</p></div>}
        {!isLoading && !error && <div className="food-grid">{foods.map((item, index) => {
          const isRestaurant = item.entity_type === 'restaurant'
          const price = typeof item.price === 'number' ? item.price : undefined
          return <article className="food-card" key={item.id ?? item.name}>
            <div className={`food-image image-${index % 4}`}>{item.image_url ? <img src={item.image_url} alt={item.name}/> : <span>{['🍛','🥘','🍜','🥗'][index % 4]}</span>}<span className="image-tag">{isRestaurant ? 'RESTAURANT' : 'MENU ITEM'}</span></div>
            <div className="food-info"><div className="food-title"><h3>{item.name}</h3>{typeof item.rating === 'number' && <span className="rating"><Star size={13} fill="currentColor"/>{item.rating.toFixed(1)}</span>}</div>
              <p>{item.description || item.cuisines?.join(' · ') || item.category || 'Made fresh, just for you'}</p>
              <div className="food-meta"><span><Clock3 size={14}/>{item.delivery_time_minutes ? `${item.delivery_time_minutes} min` : 'Delivery'}</span><span>{isRestaurant ? (item.price_for_two !== undefined ? `${money(item.price_for_two)} for two` : item.area || '') : price !== undefined ? money(price) : 'Price unavailable'}</span></div>
              {isRestaurant
                ? <button className="add-button" onClick={() => { setSelectedRestaurant(item); setSearch(''); showNotice('') }}>View menu <ArrowRight size={16}/></button>
                : <button className="add-button" disabled={!item.id || !item.restaurant_id || price === undefined} onClick={() => add(item)}><Plus size={16}/> Add to cart</button>}
            </div>
          </article>
        })}</div>}
        {isFetching && !isLoading && <div className="fetching">Updating results…</div>}
      </section>

      {orderId && <section className="order-tracking">
        <div className="tracking-head"><div><span className="eyebrow">YOUR ORDER</span><h2>Order tracking</h2><p>Order ID: <code>{orderId}</code></p></div><span className="tracking-refresh">{orderFetching && <LoaderCircle className="spin" size={15}/>} Live updates</span></div>
        {orderError ? <p className="tracking-error">{errorMessage(orderError)}</p> : order ? <><div className="status-line"><span className={`status-dot ${terminal ? 'terminal' : ''}`}/><strong>{order.status.replaceAll('_', ' ')}</strong>{order.failure_reason && <span>{order.failure_reason}</span>}</div><div className="tracking-details"><span>{order.items.reduce((n, item) => n + item.quantity, 0)} items</span><strong>{money(order.total_amount_minor / 100)}</strong></div>{!terminal && <button className="cancel-order-button" disabled={cancelState.isLoading} onClick={onCancelOrder}>{cancelState.isLoading ? 'Requesting…' : 'Cancel order'}</button>}</> : <div className="loading"><LoaderCircle className="spin"/> Fetching order status…</div>}
      </section>}
    </main>

    <footer><span className="brand small"><Utensils size={16}/> crave<span className="brand-dot">.</span></span><span>Built on your Go food-delivery microservices</span></footer>

    {cartOpen && <div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) setCartOpen(false) }}><aside className="drawer"><div className="drawer-head"><div><span className="eyebrow">YOUR SELECTION</span><h2>Your cart <small>({count})</small></h2></div><button className="icon-button" onClick={() => setCartOpen(false)}><X/></button></div>
      {!cart.length ? <div className="empty"><span>🛍️</span><h3>Your cart is empty</h3><p>Choose a restaurant and add dishes from its menu.</p><button className="add-button" onClick={() => { setCartOpen(false); setSelectedRestaurant(null) }}>Browse restaurants</button></div> : <>
        <div className="cart-list">{cart.map(item => <div className="cart-row" key={item.itemId}><div><strong>{item.name}</strong><small>{money(item.priceRupees)} each</small></div><div className="quantity"><button onClick={() => dispatch(changeQuantity({itemId:item.itemId,delta:-1}))}><Minus size={14}/></button><b>{item.quantity}</b><button onClick={() => dispatch(changeQuantity({itemId:item.itemId,delta:1}))}><Plus size={14}/></button></div></div>)}</div>
        <div className="cart-total"><span>Subtotal</span><strong>{money(total)}</strong></div><button className="primary-button" onClick={() => { setCartOpen(false); setCheckoutOpen(true) }}>Continue to checkout <ArrowRight size={17}/></button><button className="clear-button" onClick={() => dispatch(clearCart())}>Clear cart</button>
      </>}
    </aside></div>}

    {checkoutOpen && <div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) setCheckoutOpen(false) }}><section className="checkout-modal"><div className="drawer-head"><div><span className="eyebrow">ALMOST THERE</span><h2>Checkout</h2></div><button className="icon-button" onClick={() => setCheckoutOpen(false)}><X/></button></div>
      <label>Customer ID<input value={customerId} onChange={e => setCustomerId(e.target.value)} placeholder="customer-1"/></label><label>Delivery address<input value={address} onChange={e => setAddress(e.target.value)} placeholder="Street and area"/></label><label>City<input value={city} onChange={e => setCity(e.target.value)}/></label>
      <div className="cart-total"><span>Total · {count} items</span><strong>{money(total)}</strong></div><button className="primary-button" disabled={createState.isLoading || !customerId.trim() || !address.trim()} onClick={placeOrder}>{createState.isLoading ? <LoaderCircle className="spin"/> : 'Place order'} <ArrowRight size={17}/></button>
      <p className="checkout-note">Payment is not enabled. Placing this order starts the Temporal order workflow.</p>
    </section></div>}
  </div>
}
