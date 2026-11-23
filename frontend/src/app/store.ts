import { configureStore } from '@reduxjs/toolkit'
import { setupListeners } from '@reduxjs/toolkit/query'
import { foodApi } from '../services/foodApi'
import cartReducer from '../features/cart/cartSlice'

export const store = configureStore({
  reducer: { [foodApi.reducerPath]: foodApi.reducer, cart: cartReducer },
  middleware: getDefaultMiddleware => getDefaultMiddleware().concat(foodApi.middleware)
})
setupListeners(store.dispatch)
export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch