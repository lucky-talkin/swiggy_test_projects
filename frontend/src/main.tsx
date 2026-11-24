import React from 'react'
import ReactDOM from 'react-dom/client'
import { Provider } from 'react-redux'
import { store } from './app/store'
import App from './App'
import OperationsPortals from './OperationsPortals'
import './styles.css'

const path = window.location.pathname
const application = path.startsWith('/restaurant') || path.startsWith('/driver')
  ? <OperationsPortals />
  : <Provider store={store}><App /></Provider>

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>{application}</React.StrictMode>
)
