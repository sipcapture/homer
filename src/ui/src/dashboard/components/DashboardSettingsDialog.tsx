import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { DeleteIconButton } from '@/components/ui/table-action-buttons'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

export default function DashboardSettingsDialog({ dashboard, onSave, onClose, onDelete }) {
  const [name, setName] = useState(dashboard?.name || '')
  const [shared, setShared] = useState(dashboard?.shared || false)

  const handleSave = () => {
    onSave({ ...dashboard, name, shared })
    onClose()
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent className="!max-w-md gap-4">
        <DialogHeader>
          <DialogTitle>Dashboard Settings</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="dash-name">Name</Label>
            <Input
              id="dash-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Dashboard name"
            />
          </div>
          <div className="flex items-center justify-between gap-3">
            <div className="flex flex-col">
              <Label htmlFor="dash-shared" className="cursor-pointer">
                Shared
              </Label>
              <span className="text-[11px] text-muted-foreground">
                Visible to all users
              </span>
            </div>
            <Switch
              id="dash-shared"
              checked={shared}
              onCheckedChange={setShared}
            />
          </div>
        </div>
        <DialogFooter className="items-center sm:justify-between">
          {onDelete ? (
            <DeleteIconButton
              size="icon-sm"
              aria-label="Delete dashboard"
              title="Delete dashboard"
              onClick={() => {
                onDelete()
                onClose()
              }}
            />
          ) : (
            <span />
          )}
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={onClose}>
              Cancel
            </Button>
            <Button size="sm" onClick={handleSave}>
              Save
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
