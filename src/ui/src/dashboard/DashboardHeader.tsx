import { ArrowLeft, LogOut, Settings, User } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { ModeToggle } from '@/components/theme/mode-toggle'
import TimeRangePicker from './components/TimeRangePicker'
import type { CalendarPreset } from './utils/resolveTimeRange'

export interface DashboardHeaderProps {
  timeFrom: number | string
  timeTo: number | string
  onTimeChange: (
    minutes?: number | null,
    fromMs?: number,
    toMs?: number,
    opts?: { calendar?: CalendarPreset },
  ) => void
  activePreset: number | null
  calendarPreset?: CalendarPreset | null
  timeZone: string
  onTimeZoneChange: (tz: string) => void
  userLabel: string
  onOpenSettings: () => void
  onOpenDashboard: () => void
  onLogout: () => void
  showTimeControls?: boolean
  showSettings?: boolean
  showLogout?: boolean
  onBack?: () => void
}

export default function DashboardHeader({
  timeFrom,
  timeTo,
  onTimeChange,
  activePreset,
  calendarPreset = null,
  timeZone,
  onTimeZoneChange,
  userLabel,
  onOpenSettings,
  onOpenDashboard,
  onLogout,
  showTimeControls = true,
  showSettings = true,
  showLogout = true,
  onBack,
}: DashboardHeaderProps) {
  return (
    <header className="sticky top-0 z-50 flex h-16 items-center justify-between gap-4 border-b border-border bg-card/80 px-6 backdrop-blur-md">
      <div className="flex items-center gap-2">
        {onBack && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={onBack}
                aria-label="Back to dashboard"
              >
                <ArrowLeft className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Back to dashboard</TooltipContent>
          </Tooltip>
        )}
        <button
          type="button"
          onClick={onOpenDashboard}
          aria-label="Homer"
          className="brand-button !m-0 !flex !items-center !border-0 !bg-transparent !p-0 !shadow-none !outline-none focus:!outline-none focus-visible:!outline-none focus-visible:!ring-0"
        >
          <img
            src="/logo.png"
            alt="Homer"
            className="h-9 w-9 rounded-md object-contain"
          />
        </button>
      </div>

      <div className="flex items-center gap-2">
        {showTimeControls && (
          <>
            <TimeRangePicker
              timeFrom={timeFrom}
              timeTo={timeTo}
              onTimeChange={onTimeChange}
              activePreset={activePreset}
              calendarPreset={calendarPreset}
              timeZone={timeZone}
              onTimeZoneChange={onTimeZoneChange}
            />
            <Separator orientation="vertical" className="mx-1 h-6" />
          </>
        )}
        {showSettings && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={onOpenSettings}
                aria-label="Open settings"
              >
                <Settings className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Settings</TooltipContent>
          </Tooltip>
        )}
        <ModeToggle />
        <Separator orientation="vertical" className="mx-1 h-6" />
        <Badge
          variant="secondary"
          className="h-8 gap-1.5 px-2.5 text-xs font-medium"
        >
          <User className="size-3.5 text-muted-foreground" />
          <span className="text-foreground">{userLabel}</span>
        </Badge>
        {showLogout && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                onClick={onLogout}
                aria-label="Log out"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
              >
                <LogOut className="mr-1.5 h-4 w-4" />
                Logout
              </Button>
            </TooltipTrigger>
            <TooltipContent>Sign out of Homer</TooltipContent>
          </Tooltip>
        )}
      </div>
    </header>
  )
}
