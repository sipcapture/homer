import { Plus, RefreshCw } from 'lucide-react'
import { EditIconButton, DeleteIconButton } from '@/components/ui/table-action-buttons'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { BoolIndicator } from '@/components/ui/bool-indicator'
import { DigitsInput } from '@/components/ui/digits-input'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { CrudEditDialog, Field, settingsEditTitle } from './CrudTable'
import { SettingsPageHeader } from './SettingsPageHeader'

interface UserFilters {
  search: string
  username: string
  email: string
  is_admin: string
  enabled: string
  limit: string
}

interface UserForm {
  username: string
  name: string
  email: string
  password: string
  user_group: string
  enabled: boolean
}

interface UserRow {
  guid?: string
  username?: string
  name?: string
  email?: string
  user_group?: string
  enabled?: boolean
}

interface UsersPanelProps {
  filters: UserFilters
  setFilters: (filters: UserFilters) => void
  users: UserRow[]
  loadingUsers: boolean
  onLoadUsers: () => void
  userForm: UserForm
  onUserFormChange: (field: keyof UserForm, value: string | boolean) => void
  onCreate: () => void
  onUpdate: () => void
  onDelete: (user: UserRow) => void
  onStartEdit: (user: UserRow) => void
  onResetForm: () => void
  editingUser: UserRow | null
  savingUser: boolean
  deletingUser: string | null
  createModalOpen: boolean
  onOpenCreateModal: () => void
  onCloseCreateModal: () => void
  canCreate?: boolean
  canEdit?: boolean
  canDelete?: boolean
}

export default function UsersPanel({
  filters,
  setFilters,
  users,
  loadingUsers,
  onLoadUsers,
  userForm,
  onUserFormChange,
  onCreate,
  onUpdate,
  onDelete,
  onStartEdit,
  onResetForm,
  editingUser,
  savingUser,
  deletingUser,
  createModalOpen,
  onOpenCreateModal,
  onCloseCreateModal,
  canCreate = true,
  canEdit = true,
  canDelete = true,
}: UsersPanelProps) {
  const formFields = (isEdit: boolean) => (
    <>
      <Field label="Username">
        <Input
          value={userForm.username}
          onChange={(e) => onUserFormChange('username', e.target.value)}
          disabled={isEdit}
          placeholder="username"
        />
      </Field>
      <Field label="Name">
        <Input
          value={userForm.name}
          onChange={(e) => onUserFormChange('name', e.target.value)}
          placeholder="Display name"
        />
      </Field>
      <Field label="Email">
        <Input
          value={userForm.email}
          onChange={(e) => onUserFormChange('email', e.target.value)}
          placeholder="user@example.com"
        />
      </Field>
      <Field label={isEdit ? 'New password (optional)' : 'Password'}>
        <Input
          type="password"
          value={userForm.password}
          onChange={(e) => onUserFormChange('password', e.target.value)}
          placeholder={isEdit ? 'Leave blank to keep current' : ''}
        />
      </Field>
      <Field label="Group">
        <Select
          value={userForm.user_group}
          onValueChange={(v) => onUserFormChange('user_group', v)}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="user">user</SelectItem>
            <SelectItem value="admin">admin</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <Field label="Enabled">
        <Select
          value={userForm.enabled ? 'true' : 'false'}
          onValueChange={(v) => onUserFormChange('enabled', v === 'true')}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="true">enabled</SelectItem>
            <SelectItem value="false">disabled</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    </>
  )

  return (
    <div>
      <SettingsPageHeader
        title="Users"
        description="Manage Homer user accounts, groups, and access."
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={onLoadUsers}
              disabled={loadingUsers}
            >
              <RefreshCw className={cn('mr-1.5 size-4', loadingUsers && 'animate-spin')} />
              {loadingUsers ? 'Loading...' : 'Refresh'}
            </Button>
            {canCreate && (
              <Button size="sm" onClick={onOpenCreateModal}>
                <Plus className="mr-1.5 size-4" />
                Create user
              </Button>
            )}
          </>
        }
      />

      <Card className="mb-6">
        <CardContent className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Input
            placeholder="Search"
            value={filters.search}
            onChange={(e) => setFilters({ ...filters, search: e.target.value })}
          />
          <Input
            placeholder="Username"
            value={filters.username}
            onChange={(e) => setFilters({ ...filters, username: e.target.value })}
          />
          <Input
            placeholder="Email"
            value={filters.email}
            onChange={(e) => setFilters({ ...filters, email: e.target.value })}
          />
          <Select
            value={filters.is_admin || 'all'}
            onValueChange={(v) =>
              setFilters({ ...filters, is_admin: v === 'all' ? '' : v })
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Admin?" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Admin?</SelectItem>
              <SelectItem value="true">true</SelectItem>
              <SelectItem value="false">false</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filters.enabled || 'all'}
            onValueChange={(v) =>
              setFilters({ ...filters, enabled: v === 'all' ? '' : v })
            }
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Enabled?" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Enabled?</SelectItem>
              <SelectItem value="true">true</SelectItem>
              <SelectItem value="false">false</SelectItem>
            </SelectContent>
          </Select>
          <DigitsInput
            min={1}
            max={1000}
            className="w-full"
            value={filters.limit != null ? String(filters.limit) : ''}
            onValueChange={(limit) => setFilters({ ...filters, limit })}
          />
        </CardContent>
      </Card>

      {canEdit && (
        <CrudEditDialog
          open={!!editingUser}
          title={
            editingUser
              ? settingsEditTitle(
                  editingUser.user_group || 'user',
                  editingUser.username,
                  'Edit user',
                )
              : 'Edit user'
          }
          onCancel={onResetForm}
          onSave={onUpdate}
          saving={savingUser}
          saveLabel="Save changes"
        >
          {formFields(true)}
        </CrudEditDialog>
      )}

      <Card>
        <CardContent className="p-0">
          {users.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">No users loaded.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Username</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Group</TableHead>
                  <TableHead>Enabled</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user) => (
                  <TableRow key={user.guid || user.username}>
                    <TableCell className="font-medium">{user.username || '—'}</TableCell>
                    <TableCell>{user.name || '—'}</TableCell>
                    <TableCell>{user.email || '—'}</TableCell>
                    <TableCell>{user.user_group || '—'}</TableCell>
                    <TableCell>
                      <BoolIndicator value={user.enabled} />
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        {canEdit && (
                          <EditIconButton onClick={() => onStartEdit(user)} />
                        )}
                        {canDelete && (
                          <DeleteIconButton
                            onClick={() => onDelete(user)}
                            loading={deletingUser === user.guid}
                          />
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {canCreate && (
        <Dialog open={createModalOpen} onOpenChange={(o) => !o && onCloseCreateModal()}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>Create user</DialogTitle>
            </DialogHeader>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {formFields(false)}
            </div>
            <div className="flex justify-end">
              <Button onClick={onCreate} disabled={savingUser}>
                {savingUser ? 'Saving...' : 'Create user'}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </div>
  )
}
