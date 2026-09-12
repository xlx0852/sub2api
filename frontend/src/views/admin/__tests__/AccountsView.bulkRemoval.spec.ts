import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const currentDir = dirname(fileURLToPath(import.meta.url))

import AccountsView from '@/features/admin/views/AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  bulkUpdate,
  batchClearError,
  batchRefresh,
  deleteAccount
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  bulkUpdate: vi.fn(),
  batchClearError: vi.fn(),
  batchRefresh: vi.fn(),
  deleteAccount: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      delete: deleteAccount,
      batchClearError,
      batchRefresh,
      bulkUpdate,
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <span v-for="column in columns" :key="column.key" data-test="column-key">{{ column.key }}</span>
      <div v-for="row in data" :key="row.id" data-test="account-row">
        <span data-test="account-name">{{ row.name }}</span>
        <slot name="cell-created_at" :value="row.created_at" :row="row" />
      </div>
    </div>
  `
}

const baseStubs = {
  AppLayout: { template: '<div data-test="accounts-view-root"><slot /></div>' },
  TablePageLayout: {
    template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
  },
  DataTable: DataTableStub,
  Pagination: true,
  ConfirmDialog: true,
  AccountTableActions: { template: '<div data-test="account-table-actions"><slot name="beforeCreate" /><slot name="after" /></div>' },
  AccountTableFilters: { template: '<div data-test="account-table-filters"></div>' },
  AccountActionMenu: true,
  ImportDataModal: true,
  ReAuthAccountModal: true,
  AccountTestModal: true,
  AccountStatsModal: true,
  ScheduledTestsPanel: true,
  SyncFromCrsModal: true,
  TempUnschedStatusModal: true,
  ErrorPassthroughRulesModal: true,
  TLSFingerprintProfilesModal: true,
  CreateAccountModal: true,
  EditAccountModal: true,
  PlatformTypeBadge: true,
  AccountCapacityCell: true,
  AccountStatusIndicator: true,
  AccountTodayStatsCell: true,
  AccountGroupsCell: true,
  AccountUsageCell: true,
  Icon: true
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: baseStubs
    }
  })
}

describe('admin AccountsView bulk action removal', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    bulkUpdate.mockReset()
    batchClearError.mockReset()
    batchRefresh.mockReset()
    deleteAccount.mockReset()

    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'solo-account',
          platform: 'anthropic',
          type: 'oauth',
          status: 'active',
          schedulable: true,
          created_at: '2026-03-07T10:00:00Z',
          updated_at: '2026-03-07T10:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('mounts the account pool without bulk action UI or bulk mutation calls', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="accounts-view-root"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="account-table-actions"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="data-table"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="account-name"]').text()).toBe('solo-account')

    // Bulk surface must not be present
    expect(wrapper.find('[data-test="bulk-edit-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="edit-filtered"]').exists()).toBe(false)
    expect(wrapper.html()).not.toMatch(/bulk-edit|BulkEdit|bulk actions|AccountBulkActionsBar/i)

    // Bulk mutation APIs must not be invoked merely by mounting/rendering the page
    expect(bulkUpdate).not.toHaveBeenCalled()
    expect(batchClearError).not.toHaveBeenCalled()
    expect(batchRefresh).not.toHaveBeenCalled()
    expect(deleteAccount).not.toHaveBeenCalled()

    // Single-account list columns still render
    const columnKeys = wrapper.findAll('[data-test="column-key"]').map(node => node.text())
    expect(columnKeys).toContain('created_at')
    expect(columnKeys).toContain('name')
  })

  it('does not wire bulk components or bulk handlers in the shipped AccountsView source', () => {
    const sourcePath = resolve(currentDir, '../../../features/admin/views/AccountsView.vue')
    const source = readFileSync(sourcePath, 'utf8')

    expect(source).not.toContain('AccountBulkActionsBar')
    expect(source).not.toContain('BulkEditAccountModal')
    expect(source).not.toContain('handleBulkDelete')
    expect(source).not.toContain('handleBulkResetStatus')
    expect(source).not.toContain('handleBulkRefreshToken')
    expect(source).not.toContain('handleBulkToggleSchedulable')
    expect(source).not.toContain('openBulkEditSelected')
    expect(source).not.toContain('openBulkEditFiltered')
    expect(source).not.toContain('showBulkEdit')
    expect(source).not.toMatch(/adminAPI\.accounts\.(bulkUpdate|batchClearError|batchRefresh)\b/)
  })
})
