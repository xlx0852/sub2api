import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpsProviderStatusCard from '../OpsProviderStatusCard.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const degraded = {
  provider: 'openai',
  freshness: 'fresh' as const,
  snapshot: {
    id: 1,
    provider: 'openai',
    source_url: 'https://status.openai.com/api/v2/summary.json',
    overall_indicator: 'minor',
    overall_description: 'Partial System Degradation',
    source_updated_at: '2026-08-10T15:07:54Z',
    fetched_at: '2026-08-10T15:08:00Z',
    components: [
      { id: 'responses', name: 'Responses', status: 'operational' },
      { id: 'conversations', name: 'Conversations', status: 'degraded_performance' }
    ],
    incidents: [{
      id: 'incident-1', name: 'Increased error rates', status: 'monitoring', impact: 'minor',
      created_at: '2026-08-10T14:04:37Z', updated_at: '2026-08-10T15:07:54Z', updates: []
    }]
  }
}

describe('OpsProviderStatusCard', () => {
  it('shows official degradation separately with source provenance', () => {
    const wrapper = mount(OpsProviderStatusCard, { props: { current: degraded, history: [degraded.snapshot], loading: false } })
    expect(wrapper.text()).toContain('Partial System Degradation')
    expect(wrapper.text()).toContain('Conversations')
    expect(wrapper.text()).toContain('Increased error rates')
    expect(wrapper.get('a').attributes('href')).toBe('https://status.openai.com/')
  })

  it('shows stale and unavailable states without throwing', async () => {
    const wrapper = mount(OpsProviderStatusCard, { props: { current: { ...degraded, freshness: 'stale' }, history: [], loading: false } })
    expect(wrapper.text()).toContain('admin.ops.providerStatus.stale')
    await wrapper.setProps({ current: { provider: 'openai', freshness: 'unavailable' } })
    expect(wrapper.text()).toContain('admin.ops.providerStatus.unavailable')
  })
})
