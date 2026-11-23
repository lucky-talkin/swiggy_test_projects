import { createSlice, type PayloadAction } from '@reduxjs/toolkit'

export interface CartItem {
  itemId: string
  restaurantId: string
  name: string
  priceRupees: number
  quantity: number
}
interface CartState { items: CartItem[] }
const initialState: CartState = { items: [] }

const cartSlice = createSlice({
  name: 'cart', initialState,
  reducers: {
    addToCart(state, action: PayloadAction<Omit<CartItem, 'quantity'>>) {
      const incoming = action.payload
      if (state.items.some(item => item.restaurantId !== incoming.restaurantId)) return
      const existing = state.items.find(item => item.itemId === incoming.itemId)
      if (existing) existing.quantity += 1
      else state.items.push({ ...incoming, quantity: 1 })
    },
    changeQuantity(state, action: PayloadAction<{ itemId: string; delta: number }>) {
      const item = state.items.find(entry => entry.itemId === action.payload.itemId)
      if (!item) return
      item.quantity += action.payload.delta
      if (item.quantity <= 0) state.items = state.items.filter(entry => entry.itemId !== item.itemId)
    },
    clearCart(state) { state.items = [] }
  }
})
export const { addToCart, changeQuantity, clearCart } = cartSlice.actions
export default cartSlice.reducer