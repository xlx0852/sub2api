export const SETTINGS_TABS = [
  { key: 'general', icon: 'home' },
  { key: 'agreement', icon: 'document' },
  { key: 'features', icon: 'bolt' },
  { key: 'security', icon: 'shield' },
  { key: 'users', icon: 'user' },
  { key: 'gateway', icon: 'server' },
  { key: 'pricing', icon: 'creditCard' },
  { key: 'payment', icon: 'creditCard' },
  { key: 'email', icon: 'mail' },
  { key: 'backup', icon: 'database' }
] as const

export type SettingsTab = (typeof SETTINGS_TABS)[number]['key']

export function isSettingsTab(value: unknown): value is SettingsTab {
  return SETTINGS_TABS.some((tab) => tab.key === value)
}

export function settingsTabFromParam(value: unknown): SettingsTab {
  const raw = Array.isArray(value) ? value[0] : value
  return isSettingsTab(raw) ? raw : 'general'
}
