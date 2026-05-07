import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { SettingsSidebar } from './SettingsSidebar'
import { ROLE } from './permissions'

describe('SettingsSidebar RBAC', () => {
  it('shows full admin menu', () => {
    render(<SettingsSidebar activeSection="about" onSelect={() => {}} role={ROLE.ADMIN} />)
    expect(screen.getByText('Users')).toBeInTheDocument()
    expect(screen.getByText('API Documentation')).toBeInTheDocument()
    expect(screen.getByText('Scripts')).toBeInTheDocument()
  })

  it('hides admin sections for common user', () => {
    render(<SettingsSidebar activeSection="about" onSelect={() => {}} role={ROLE.COMMON} />)
    expect(screen.queryByText('API Documentation')).not.toBeInTheDocument()
    expect(screen.queryByText('Scripts')).not.toBeInTheDocument()
    expect(screen.getByText('Reset')).toBeInTheDocument()
  })

  it('calls onSelect on click', () => {
    let picked = ''
    render(<SettingsSidebar activeSection="profile" onSelect={(key) => { picked = key }} role={ROLE.EXTERNAL} />)
    fireEvent.click(screen.getByText('Reset'))
    expect(picked).toBe('reset')
  })
})
