import { RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { runClientResetWithConfirm, type ClientResetAction } from '../settings/clientResetActions'

export interface ClientResetDropdownProps {
  /** Opens Settings on the full Reset page (server + client actions). */
  onOpenResetSettings?: () => void
}

export function ClientResetDropdown({ onOpenResetSettings }: ClientResetDropdownProps) {
  const confirm = useConfirm()

  const run = (action: ClientResetAction) => {
    void runClientResetWithConfirm(confirm, action)
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="icon"
          aria-label="Browser reset: cache, dashboard, local storage, cookies"
          title="Browser reset (cache, dashboard, storage, cookies)"
          type="button"
        >
          <RotateCcw className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-[14rem]">
        <DropdownMenuLabel>Browser reset</DropdownMenuLabel>
        <DropdownMenuItem onSelect={() => run('ui_cache')}>UI cache reset</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => run('dashboard_local')}>Dashboard reset (local)</DropdownMenuItem>
        <DropdownMenuItem
          onSelect={() => run('local_storage')}
          className="text-destructive focus:text-destructive"
        >
          Local storage reset
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => run('cookies')}>Cookie reset</DropdownMenuItem>
        {onOpenResetSettings ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={() => {
                onOpenResetSettings()
              }}
            >
              More reset options…
            </DropdownMenuItem>
          </>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
