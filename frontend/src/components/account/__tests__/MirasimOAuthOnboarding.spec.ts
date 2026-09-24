/**
 * MirasimOAuthOnboarding 组件测试。
 *
 * 关键约束（改动前请先读）：
 *   1. 组件只“生产”凭据：成功路径必须 emit('apply', credential) 交给父组件回填表单，
 *      不允许自己调账号创建接口。
 *   2. 明文凭据不得出现在 console（任何级别）里；消费后必须立刻清空本地输入。
 *   3. 登录环节不调用 /auth/refresh —— 本组件全链路只有 4 个 admin 端点，测试里断言
 *      调用集合即可（多调一个端点就会失败）。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const {
  listProvidersMock,
  generateAuthUrlMock,
  completeOAuthMock,
  startEmailCodeMock,
  completeEmailCodeMock
} = vi.hoisted(() => ({
  listProvidersMock: vi.fn(),
  generateAuthUrlMock: vi.fn(),
  completeOAuthMock: vi.fn(),
  startEmailCodeMock: vi.fn(),
  completeEmailCodeMock: vi.fn()
}))

vi.mock('@/api/admin/mirasim', () => ({
  default: {
    listProviders: listProvidersMock,
    generateAuthUrl: generateAuthUrlMock,
    completeOAuth: completeOAuthMock,
    startEmailCode: startEmailCodeMock,
    completeEmailCode: completeEmailCodeMock
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${JSON.stringify(params)}` : key
    })
  }
})

import MirasimOAuthOnboarding from '../MirasimOAuthOnboarding.vue'

// 全部为现场构造的假凭据，绝不使用真实 token。
const FAKE_CALLBACK_URL =
  'http://127.0.0.1:9/callback?refresh_token=fake-refresh-token-one&state=fake-state'
const FAKE_REFRESH_TOKEN = 'fake-refresh-token-two'
const FAKE_CREDENTIAL = {
  refresh_token: FAKE_REFRESH_TOKEN,
  identity: {
    provider: 'github',
    email: 'admin@example.com',
    plan: 'pro',
    expires_at: 1893456000
  }
}

const AUTH_URL = 'https://auth.example.test'
const PROXY_ID = 7

function mountPanel(props: Record<string, unknown> = {}) {
  return mount(MirasimOAuthOnboarding, {
    props: { authUrl: AUTH_URL, proxyId: PROXY_ID, defaultExpanded: true, ...props }
  })
}

function find(wrapper: ReturnType<typeof mountPanel>, suffix: string) {
  return wrapper.find(`[data-testid="mirasim-oauth-${suffix}"]`)
}

/** 回调 URL 输入框只在生成授权链接之后才出现（上游 manual 流程）。 */
async function openCallbackForm(wrapper: ReturnType<typeof mountPanel>) {
  await find(wrapper, 'generate').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  listProvidersMock.mockResolvedValue(['github', 'google'])
  generateAuthUrlMock.mockResolvedValue({
    provider: 'github',
    auth_url: 'https://auth.mirasim.ai/auth/oauth/github/login?state=fake-state',
    redirect_uri: 'http://127.0.0.1:9/callback',
    state: 'fake-state',
    expires_at: 1893456000
  })
  completeOAuthMock.mockResolvedValue(FAKE_CREDENTIAL)
  startEmailCodeMock.mockResolvedValue({ email: 'admin@example.com', message: 'sent' })
  completeEmailCodeMock.mockResolvedValue(FAKE_CREDENTIAL)
})

describe('MirasimOAuthOnboarding', () => {
  it('折叠时不请求上游，展开后只请求一次登录方式', async () => {
    const wrapper = mountPanel({ defaultExpanded: false })
    expect(listProvidersMock).not.toHaveBeenCalled()

    await find(wrapper, 'toggle').trigger('click')
    await flushPromises()
    expect(listProvidersMock).toHaveBeenCalledTimes(1)
    expect(listProvidersMock).toHaveBeenCalledWith({ auth_url: AUTH_URL, proxy_id: PROXY_ID })

    const options = find(wrapper, 'provider').findAll('option')
    expect(options.map((o) => o.element.textContent?.trim())).toEqual([
      'admin.accounts.mirasimOAuth.providerGithub',
      'admin.accounts.mirasimOAuth.providerGoogle'
    ])

    // 再次折叠/展开不应重复拉取。
    await find(wrapper, 'toggle').trigger('click')
    await find(wrapper, 'toggle').trigger('click')
    await flushPromises()
    expect(listProvidersMock).toHaveBeenCalledTimes(1)
  })

  it('上游登录方式拉取失败时回退内置 github/google 并提示错误', async () => {
    listProvidersMock.mockRejectedValueOnce(new Error('upstream down'))
    const wrapper = mountPanel()
    await flushPromises()

    expect(find(wrapper, 'error').text()).toBe('upstream down')
    expect(find(wrapper, 'provider').findAll('option').length).toBe(2)
  })

  it('生成授权链接：只带 provider/auth_url/proxy_id，并把链接渲染到 <a href>', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await find(wrapper, 'provider').setValue('google')
    await find(wrapper, 'generate').trigger('click')
    await flushPromises()

    expect(generateAuthUrlMock).toHaveBeenCalledWith({
      provider: 'google',
      auth_url: AUTH_URL,
      proxy_id: PROXY_ID
    })
    expect(find(wrapper, 'auth-link').attributes('href')).toBe(
      'https://auth.mirasim.ai/auth/oauth/github/login?state=fake-state'
    )
  })

  it('回调 URL 提交：回填凭据、清空本地明文、不发多余请求', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await openCallbackForm(wrapper)
    // 清掉“打开表单”阶段的调用，下面只验证提交回调这一步的真实请求集合。
    generateAuthUrlMock.mockClear()
    listProvidersMock.mockClear()

    await find(wrapper, 'callback-url').setValue(FAKE_CALLBACK_URL)
    await find(wrapper, 'submit-callback').trigger('click')
    await flushPromises()

    expect(completeOAuthMock).toHaveBeenCalledWith({
      callback_url: FAKE_CALLBACK_URL,
      provider: 'github'
    })
    // 只调了 complete 这一个端点：登录链路不碰 /auth/refresh，也不会重复生成链接。
    expect(generateAuthUrlMock).not.toHaveBeenCalled()
    expect(listProvidersMock).not.toHaveBeenCalled()
    expect(startEmailCodeMock).not.toHaveBeenCalled()
    expect(completeEmailCodeMock).not.toHaveBeenCalled()

    const applied = wrapper.emitted('apply')
    expect(applied).toHaveLength(1)
    expect(applied![0][0]).toEqual(FAKE_CREDENTIAL)

    // 明文回调链接必须立刻从输入框消失。
    expect((find(wrapper, 'callback-url').element as HTMLTextAreaElement).value).toBe('')
    const result = find(wrapper, 'result').text()
    expect(result).toContain('admin@example.com')
    expect(result).toContain('pro')
    expect(result).not.toContain(FAKE_REFRESH_TOKEN)
  })

  it('回调 URL 为空时本地报错且不发请求', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await openCallbackForm(wrapper)

    await find(wrapper, 'submit-callback').trigger('click')
    await flushPromises()

    expect(completeOAuthMock).not.toHaveBeenCalled()
    expect(find(wrapper, 'error').text()).toBe('admin.accounts.mirasimOAuth.missingCallback')
    expect(wrapper.emitted('apply')).toBeUndefined()
  })

  it('粘贴 token 模式：只发 refresh_token，提交后清空输入', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    // 空 token 先本地拦截。
    await find(wrapper, 'tab-token').trigger('click')
    await find(wrapper, 'submit-token').trigger('click')
    await flushPromises()
    expect(find(wrapper, 'error').text()).toBe('admin.accounts.mirasimOAuth.missingToken')
    expect(completeOAuthMock).not.toHaveBeenCalled()

    await find(wrapper, 'token').setValue(FAKE_REFRESH_TOKEN)
    await find(wrapper, 'submit-token').trigger('click')
    await flushPromises()

    expect(completeOAuthMock).toHaveBeenCalledWith({
      refresh_token: FAKE_REFRESH_TOKEN,
      provider: 'github'
    })
    expect((find(wrapper, 'token').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.emitted('apply')).toHaveLength(1)
  })

  it('邮箱验证码模式：先发码再校验，两步各自带 email', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await find(wrapper, 'tab-email').trigger('click')

    // 没有邮箱时两步都应在本地拦截。
    await find(wrapper, 'send-code').trigger('click')
    await find(wrapper, 'submit-code').trigger('click')
    await flushPromises()
    expect(startEmailCodeMock).not.toHaveBeenCalled()
    expect(completeEmailCodeMock).not.toHaveBeenCalled()
    expect(find(wrapper, 'error').text()).toBe('admin.accounts.mirasimOAuth.missingEmail')

    await find(wrapper, 'email').setValue('admin@example.com')
    await find(wrapper, 'send-code').trigger('click')
    await flushPromises()
    expect(startEmailCodeMock).toHaveBeenCalledWith({
      email: 'admin@example.com',
      auth_url: AUTH_URL,
      proxy_id: PROXY_ID
    })

    // 缺验证码时本地拦截。
    await find(wrapper, 'submit-code').trigger('click')
    await flushPromises()
    expect(completeEmailCodeMock).not.toHaveBeenCalled()
    expect(find(wrapper, 'error').text()).toBe('admin.accounts.mirasimOAuth.missingCode')

    await find(wrapper, 'code').setValue('123456')
    await find(wrapper, 'submit-code').trigger('click')
    await flushPromises()

    expect(completeEmailCodeMock).toHaveBeenCalledWith({
      email: 'admin@example.com',
      code: '123456',
      auth_url: AUTH_URL,
      proxy_id: PROXY_ID
    })
    expect(wrapper.emitted('apply')).toHaveLength(1)
    expect((find(wrapper, 'code').element as HTMLInputElement).value).toBe('')
  })

  it('上游失败只在面板内提示，不 emit 也不清空已填内容', async () => {
    completeOAuthMock.mockRejectedValueOnce({
      response: { data: { message: 'MIRASIM_OAUTH_VERIFY_NO_TOKEN' } }
    })
    const wrapper = mountPanel()
    await flushPromises()
    await openCallbackForm(wrapper)

    await find(wrapper, 'callback-url').setValue(FAKE_CALLBACK_URL)
    await find(wrapper, 'submit-callback').trigger('click')
    await flushPromises()

    expect(find(wrapper, 'error').text()).toBe('MIRASIM_OAUTH_VERIFY_NO_TOKEN')
    expect(wrapper.emitted('apply')).toBeUndefined()
  })

  it('任何分支都不把明文凭据写进 console', async () => {
    const spies = [
      vi.spyOn(console, 'log').mockImplementation(() => {}),
      vi.spyOn(console, 'info').mockImplementation(() => {}),
      vi.spyOn(console, 'warn').mockImplementation(() => {}),
      vi.spyOn(console, 'error').mockImplementation(() => {})
    ]

    const wrapper = mountPanel()
    await flushPromises()
    await openCallbackForm(wrapper)

    await find(wrapper, 'callback-url').setValue(FAKE_CALLBACK_URL)
    await find(wrapper, 'submit-callback').trigger('click')
    await flushPromises()

    await find(wrapper, 'tab-token').trigger('click')
    await find(wrapper, 'token').setValue(FAKE_REFRESH_TOKEN)
    await find(wrapper, 'submit-token').trigger('click')
    await flushPromises()

    await find(wrapper, 'tab-email').trigger('click')
    await find(wrapper, 'email').setValue('admin@example.com')
    await find(wrapper, 'send-code').trigger('click')
    await find(wrapper, 'code').setValue('123456')
    await find(wrapper, 'submit-code').trigger('click')
    await flushPromises()

    const secrets = [FAKE_REFRESH_TOKEN, FAKE_CALLBACK_URL, 'fake-state']
    for (const spy of spies) {
      for (const call of spy.mock.calls) {
        const rendered = call
          .map((arg) => {
            if (typeof arg === 'string') return arg
            try {
              return JSON.stringify(arg)
            } catch {
              return String(arg)
            }
          })
          .join(' ')
        for (const secret of secrets) {
          expect(rendered).not.toContain(secret)
        }
      }
    }
    spies.forEach((spy) => spy.mockRestore())
    expect(wrapper.emitted('apply')?.length).toBe(3)
  })
})
