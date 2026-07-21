import { createRoot } from 'react-dom/client'
import { App } from './app'
import './style.css'
import { TooltipProvider } from '@/components/ui/tooltip'
import { DEMO_MODE, installDemoApi } from './demo-api'

if (DEMO_MODE) installDemoApi()

createRoot(document.getElementById('root')!).render(<TooltipProvider><App /></TooltipProvider>)
