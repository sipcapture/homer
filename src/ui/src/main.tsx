import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Toaster } from '@/components/ui/sonner'
import { ConfirmProvider } from '@/components/ui/confirm-dialog'

const rootEl = document.getElementById('root')
if (!rootEl) throw new Error('Root element #root not found')

createRoot(rootEl).render(
  <StrictMode>
    <TooltipProvider delayDuration={200}>
      <ConfirmProvider>
        <App />
        <Toaster richColors closeButton position="top-right" />
      </ConfirmProvider>
    </TooltipProvider>
  </StrictMode>,
)
