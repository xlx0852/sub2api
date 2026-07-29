import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GrokProductUsageBreakdown from '../GrokProductUsageBreakdown.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => params?.value ? `${key} ${params.value}` : key
  })
}))

describe('GrokProductUsageBreakdown', () => {
  it('在独立区块展示完整产品渠道明细', () => {
    const wrapper = mount(GrokProductUsageBreakdown, {
      props: {
        products: [
          { product: 'Api', usage_percent: 96 },
          { product: 'GrokBuild', usage_percent: 4 },
          { product: 'GrokChat' },
          { product: 'PartnerRelay', usage_percent: 12.4 }
        ]
      }
    })

    expect(wrapper.get('[data-product="api:0"]').text()).toContain('96%')
    expect(wrapper.get('[data-product="grokbuild:1"]').text()).toContain('4%')
    expect(wrapper.get('[data-product="grokchat:2"]').text()).toContain('--')
    expect(wrapper.get('[data-product="partnerrelay:3"]').text()).toContain('PartnerRelay')
    expect(wrapper.get('[data-product="partnerrelay:3"]').text()).toContain('12%')
  })

  it('没有产品数据时不占据抽屉空间', () => {
    const wrapper = mount(GrokProductUsageBreakdown, { props: { products: [] } })
    expect(wrapper.find('[data-testid="grok-product-usage-breakdown"]').exists()).toBe(false)
  })
})
