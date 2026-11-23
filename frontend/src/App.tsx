import { useMemo, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { Search, ShoppingBag, MapPin, Plus, Minus, Clock3, Star, Utensils, X, Bike, ArrowRight, LoaderCircle } from 'lucide-react'
import type { RootState, AppDispatch } from './app/store'
import { addToCart, changeQuantity, clearCart } from './features/cart/cartSlice'
import { useSearchFoodQuery, useCreateOrderMutation } from './services/foodApi'
import type { FoodItem } from './types'

const money = (value: number) => new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 }).format(value)
const getItems = (data: unknown): FoodItem[] => {
  if (!data || typeof data !== 'object') return []
  const value = data as { results?: unknown; items?: unknown }
  const list = Array.isArray(value.results) ? value.results : Array.isArray(value.items) ? value.items : []
  return list.filter((item): item is FoodItem => !!item && typeof item === 'object' && typeof (item as FoodItem).name === 'string')
}

export default function App() {
  const dispatch = useDispatch<AppDispatch>()
  const cart = useSelector((state: RootState) => state.cart.items)
  const [search, setSearch] = useState('')
  const [city, setCity] = useState('Bengaluru')
  const [kind, setKind] = useState<'restaurant' | 'menu_item'>('restaurant')
  const [cartOpen, setCartOpen] = useState(false)
  const [checkoutOpen, setCheckoutOpen] = useState(false)
  const [customerId, setCustomerId] = useState('customer-1')
  const [address, setAddress] = useState('MG Road')
  const [orderMessage, setOrderMessage] = useState('')
  const params = useMemo(() => ({ q: search.trim() || undefined, city, type: kind, page: 1, page_size: 20 }), [search, city, kind])
  const { data, isLoading, isFetching, error } = useSearchFoodQuery(params)
  const [createOrder, orderState] = useCreateOrderMutation()
  const foods = getItems(data)
  const count = cart.reduce((sum, item) => sum + item.quantity, 0)
  const total = cart.reduce((sum, item) => sum + item.priceRupees * item.quantity, 0)
  const activeRestaurant = cart[0]?.restaurantId

  async function placeOrder() {
    if (!cart.length || !activeRestaurant) return
    try {
      const order = await createOrder({
        customer_id: customerId.trim(), restaurant_id: activeRestaurant,
        items: cart.map(item => ({ item_id: item.itemId, quantity: item.quantity })),
        delivery_address: { line1: address.trim(), city },
        total_amount_minor: Math.round(total * 100), currency: 'INR'
      }).unwrap()
      dispatch(clearCart())
      setCheckoutOpen(false)
      setCartOpen(false)
      setOrderMessage(`Order ${order.order_id} created · ${order.status}`)
    } catch (e) {
      const message = typeof e === 'object' && e && 'data' in e ? JSON.stringify((e as {data: unknown}).data) : 'Could not place order. Check that Order API is running.'
      setOrderMessage(message)
    }
  }

  function add(item: FoodItem) {
    const itemId = item.id
    const restaurantId = item.restaurant_id || (item.entity_type === 'restaurant' ? item.id : undefined)
    const price = typeof item.price === 'number' ? item.price : undefined
    if (!itemId || !restaurantId || price === undefined) {
      setOrderMessage('This result does not include an item ID, restaurant ID and price required for checkout.')
      return
    }
    if (cart.length && cart[0].restaurantId !== restaurantId) {
      setOrderMessage('Your cart contains items from another restaurant. Clear it before ordering here.')
      return
    }
    dispatch(addToCart({ itemId, restaurantId, name: item.name, priceRupees: price }))
    setOrderMessage('')
  }

  return <div className="app-shell">
    <header className="topbar">
      <a className="brand" href="#"><span className="brand-mark"><Utensils size={19}/></span><span>crave<span className="brand-dot">.</span></span></a>
      <div className="location"><MapPin size={18}/><div><strong>Delivering to</strong><input aria-label="Delivery city" value={city} onChange={e => setCity(e.target.value)} /></div></div>
      <button className="cart-button" onClick={() => setCartOpen(true)}><ShoppingBag size={18}/><span>Cart</span><b>{count}</b></button>
    </header>

    <main>
      <section className="hero">
        <div className="hero-copy"><span className="eyebrow">GOOD FOOD, GOOD MOOD</span><h1>Your next meal<br/><em>is a few clicks away.</em></h1><p>Discover local favourites and get the food you love delivered.</p>
          <div className="searchbar"><Search size={20}/><input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search dosa, biryani, restaurants..." /><button onClick={() => setKind('restaurant')}>Search</button></div>
        </div>
        <div className="hero-art"><div className="art-circle"><span>✦</span><div className="food-emoji">🍜</div><small>made with<br/>love</small></div><div className="floating-note"><Bike size={18}/><span>Fresh food<br/><b>at your door</b></span></div></div>
      </section>

      <section className="browse-section">
        <div className="section-heading"><div><span className="eyebrow">FIND YOUR FAVOURITE</span><h2>What are you craving?</h2></div><div className="tabs"><button className={kind === 'restaurant' ? 'active' : ''} onClick={() => setKind('restaurant')}>Restaurants</button><button className={kind === 'menu_item' ? 'active' : ''} onClick={() => setKind('menu_item')}>Dishes</button></div></div>
        {orderMessage && <div className="notice"><span>{orderMessage}</span><button onClick={() => setOrderMessage('')}><X size={16}/></button></div>}
        {error && <div className="error-panel"><strong>Can't reach the Search API.</strong><p>Start the backend with Docker Compose and check http://localhost:8083/healthz.</p></div>}
        {isLoading && <div className="loading"><LoaderCircle className="spin"/> Loading restaurants...</div>}
        {!isLoading && !error && foods.length === 0 && <div className="empty"><span>🥡</span><h3>No results just yet</h3><p>Try another search or check that your Elasticsearch index has documents.</p></div>}
        <div className="food-grid">{foods.map((item, index) => {
          const id = item.id || item.name
          const restaurantId = item.restaurant_id || (item.entity_type === 'restaurant' ? item.id : '')
          const price = typeof item.price === 'number' ? item.price : undefined
          return <article className="food-card" key={id + index}>
            <div className={`food-image image-${index % 4}`}>{item.image_url ? <img src={item.image_url} alt={item.name}/> : <span>{['🍛','🥘','🍜','🥗'][index % 4]}</span>}<span className="image-tag">{item.entity_type === 'menu_item' ? 'DISH' : 'LOCAL FAVOURITE'}</span></div>
            <div className="food-info"><div className="food-title"><h3>{item.name}</h3>{typeof item.rating === 'number' && <span className="rating"><Star size={13} fill="currentColor"/>{item.rating.toFixed(1)}</span>}</div>
              <p>{item.description || item.cuisines?.join(' · ') || item.category || 'Made fresh, just for you'}</p>
              <div className="food-meta"><span><Clock3 size={14}/>{item.delivery_time_minutes ? `${item.delivery_time_minutes} min` : 'Delivery'}</span><span>{price !== undefined ? money(price) : item.price_for_two !== undefined ? `${money(item.price_for_two)} for two` : item.area || item.city || ''}</span></div>
              <button className="add-button" disabled={!item.id || !restaurantId || price === undefined} onClick={() => add(item)}><Plus size={16}/> Add to cart</button>
            </div>
          </article>
        })}</div>
        {isFetching && !isLoading && <div className="fetching">Updating results…</div>}
      </section>
    </main>

    <footer><span className="brand small"><Utensils size={16}/> crave<span className="brand-dot">.</span></span><span>Built on your Go food-delivery microservices</span></footer>

    {cartOpen && <div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) setCartOpen(false) }}><aside className="drawer"><div className="drawer-head"><div><span className="eyebrow">YOUR SELECTION</span><h2>Your cart <small>({count})</small></h2></div><button className="icon-button" onClick={() => setCartOpen(false)}><X/></button></div>
      {!cart.length ? <div className="empty"><span>🛍️</span><h3>Your cart is empty</h3><p>Add something delicious to get started.</p></div> : <>
        <div className="cart-list">{cart.map(item => <div className="cart-row" key={item.itemId}><div><strong>{item.name}</strong><small>{money(item.priceRupees)} each</small></div><div className="quantity"><button onClick={() => dispatch(changeQuantity({itemId:item.itemId,delta:-1}))}><Minus size={14}/></button><b>{item.quantity}</b><button onClick={() => dispatch(changeQuantity({itemId:item.itemId,delta:1}))}><Plus size={14}/></button></div></div>)}</div>
        <div className="cart-total"><span>Subtotal</span><strong>{money(total)}</strong></div><button className="primary-button" onClick={() => { setCartOpen(false); setCheckoutOpen(true) }}>Continue to checkout <ArrowRight size={17}/></button><button className="clear-button" onClick={() => dispatch(clearCart())}>Clear cart</button>
      </>}
    </aside></div>}

    {checkoutOpen && <div className="overlay"><section className="checkout-modal"><div className="drawer-head"><div><span className="eyebrow">ALMOST THERE</span><h2>Checkout</h2></div><button className="icon-button" onClick={() => setCheckoutOpen(false)}><X/></button></div>
      <label>Customer ID<input value={customerId} onChange={e => setCustomerId(e.target.value)} placeholder="customer-1"/></label><label>Delivery address<input value={address} onChange={e => setAddress(e.target.value)} placeholder="Street and area"/></label><label>City<input value={city} onChange={e => setCity(e.target.value)}/></label>
      <div className="cart-total"><span>Total · {count} items</span><strong>{money(total)}</strong></div><button className="primary-button" disabled={orderState.isLoading || !customerId.trim() || !address.trim()} onClick={placeOrder}>{orderState.isLoading ? <LoaderCircle className="spin"/> : 'Place order'} <ArrowRight size={17}/></button>
      <p className="checkout-note">Payment is not enabled in this backend yet. This creates an order and starts the Temporal order workflow.</p>
    </section></div>}
  </div>
}