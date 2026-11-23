export interface FoodItem {
  id?: string
  restaurant_id?: string
  entity_type?: 'restaurant' | 'menu_item'
  name: string
  description?: string
  cuisines?: string[]
  category?: string
  price?: number
  price_for_two?: number
  rating?: number
  delivery_time_minutes?: number
  is_open?: boolean
  city?: string
  area?: string
  image_url?: string
  location?: { lat: number; lon: number }
}
export interface SearchResponse {
  results?: FoodItem[]
  items?: FoodItem[]
  total?: number
  page?: number
  page_size?: number
  [key: string]: unknown
}
export interface Order {
  order_id: string
  customer_id: string
  restaurant_id: string
  items: { item_id: string; quantity: number }[]
  total_amount_minor: number
  currency: string
  status: string
  failure_reason?: string
  delivery_address?: { line1?: string; city?: string }
  updated_at?: string
  created_at?: string
}