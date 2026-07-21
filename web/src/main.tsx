import { createRoot } from 'react-dom/client'
import { App } from './app'
import './style.css'
import { TooltipProvider } from '@/components/ui/tooltip'

createRoot(document.getElementById('root')!).render(<TooltipProvider><App /></TooltipProvider>)
