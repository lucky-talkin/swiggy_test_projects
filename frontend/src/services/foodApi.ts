import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'
import type { Order, SearchResponse } from '../types'

export interface SearchParams {
  q?: string
  city?: string
  restaurant_id?: string
  type?: 'restaurant' | 'menu_item'
  cuisine?: string
  min_rating?: number
  page?: number
  page_size?: number
}
export interface CreateOrderRequest {
  customer_id: string
  restaurant_id: string
  items: { item_id: string; quantity: number }[]
  delivery_address: { line1: string; city: string }
  total_amount_minor: number
  currency: 'INR'
}

export const foodApi = createApi({
  reducerPath: 'foodApi',
  baseQuery: fetchBaseQuery({ baseUrl: '/' }),
  tagTypes: ['Order'],
  endpoints: build => ({
    searchFood: build.query<SearchResponse, SearchParams>({
      query: params => ({ url: '/search-api/v1/search', params: Object.fromEntries(Object.entries(params).filter(([, value]) => value !== undefined && value !== '')) })
    }),
    createOrder: build.mutation<Order, CreateOrderRequest>({
      query: body => ({
        url: '/order-api/v1/orders', method: 'POST', body,
        headers: { 'Idempotency-Key': crypto.randomUUID() }
      }),
      invalidatesTags: ['Order']
    }),
    getOrder: build.query<Order, string>({
      query: id => `/order-api/v1/orders/${id}`, providesTags: ['Order']
    }),
    cancelOrder: build.mutation<Order, { id: string; reason?: string }>({
      query: ({ id, reason = 'cancelled_by_customer' }) => ({ url: `/order-api/v1/orders/${id}/cancel`, method: 'POST', body: { reason } }),
      invalidatesTags: ['Order']
    })
  })
})
export const { useSearchFoodQuery, useCreateOrderMutation, useGetOrderQuery, useCancelOrderMutation } = foodApi