import { describe, expect, it, vi } from 'vitest'

const { getModelCatalogMock } = vi.hoisted(() => ({
	getModelCatalogMock: vi.fn()
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('@/api/admin/modelCatalog', () => ({
	getModelCatalog: getModelCatalogMock
}))

import {
  buildModelMappingObject,
	ensureModelCatalogLoaded,
  getModelsByPlatform,
  replaceExactModelsWithUpstream,
  replaceIdentityModelMappingsWithUpstream,
  splitModelMappingObject,
  useModelRestrictionEditor
} from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('共享模型限制编辑器会拒绝重复来源并支持统一重置', () => {
    const editor = useModelRestrictionEditor()

    editor.addModelMapping()
    editor.modelMappings.value[0] = { from: 'gpt-latest', to: 'gpt-5.4' }

    expect(editor.addPresetMapping('gpt-latest', 'gpt-5.5')).toBe(false)
    expect(editor.addPresetMapping('gpt-new', 'gpt-5.5')).toBe(true)
    expect(editor.modelMappings.value).toHaveLength(2)

    editor.modelRestrictionMode.value = 'mapping'
    editor.allowedModels.value = ['gpt-5.4']
    editor.resetModelRestriction()

    expect(editor.modelRestrictionMode.value).toBe('whitelist')
    expect(editor.allowedModels.value).toEqual([])
    expect(editor.modelMappings.value).toEqual([])
  })

  it('openai 模型列表包含 GPT-5.4 官方快照', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-2026-03-05')
    expect(models).toContain('codex-auto-review')
    expect(models).toContain('gpt-5.6-sol')
    expect(models).toContain('gpt-5.6-terra')
    expect(models).toContain('gpt-5.6-luna')
    expect(models).not.toContain('gpt-5.6')
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含当前代 Gemini', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.8-flash')
    expect(models).toContain('gemini-3.8-flash-high')
    expect(models).toContain('gemini-3.7-flash')
    expect(models).toContain('gemini-3.7-flash-high')
    expect(models).toContain('gemini-3.6-flash')
    expect(models).toContain('gemini-3.6-flash-high')
    expect(models).toContain('gemini-3.1-pro')
    expect(models).toContain('gemini-3.1-pro-high')
    expect(models).toContain('gemini-3.1-pro-low')
    expect(models).not.toContain('gemini-3.1-flash-lite')
    expect(models).not.toContain('gemini-3-flash')
    expect(models).not.toContain('gemini-3.1-flash-image')
    expect(models).not.toContain('gemini-2.5-flash')
    expect(models).not.toContain('gemini-2.5-flash-image')
  })

  it('Claude 模型列表包含新发布的 Claude 模型', () => {
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).not.toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).not.toContain('claude-opus-4-8')
  })

  it('gemini 模型列表仅保留当前代', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3.5-flash')
    expect(models).toContain('gemini-3.1-pro-preview')
    expect(models).not.toContain('gemini-2.0-flash')
    expect(models).not.toContain('gemini-2.5-flash')
    expect(models).not.toContain('gemini-2.5-pro')
    expect(models).not.toContain('gemini-3-pro-preview')
  })

  it('xAI 模型列表包含 Grok 4.6 和官方别名', () => {
    const models = getModelsByPlatform('grok')
    expect(models).toContain('grok-4.6')
    expect(models).toContain('grok-4.5')
    expect(models).toContain('grok-4.5-latest')
    expect(models).toContain('grok-build-latest')
  })

  it('DeepSeek 默认列表包含当前官方 V4 模型及视觉实验模型', () => {
    const models = getModelsByPlatform('deepseek')
    expect(models).toEqual(['deepseek-v4-pro', 'deepseek-v4-flash', 'deepseek-v4-flash-vision-exp'])
    expect(models).not.toContain('deepseek-chat')
    expect(models).not.toContain('deepseek-reasoner')
  })

	it('强制刷新会重新请求模型目录并替换旧 Grok 快照', async () => {
		getModelCatalogMock
			.mockResolvedValueOnce({ platforms: { grok: { models: [{ id: 'grok-4.5' }] } } })
			.mockResolvedValueOnce({ platforms: { grok: { models: [{ id: 'grok-4.6' }, { id: 'grok-4.5' }] } } })

		await ensureModelCatalogLoaded({ force: true })
		expect(getModelsByPlatform('grok')).toEqual(['grok-4.5'])
		await ensureModelCatalogLoaded({ force: true })

		expect(getModelCatalogMock).toHaveBeenCalledTimes(2)
		expect(getModelsByPlatform('grok')).toEqual(['grok-4.6', 'grok-4.5'])
	})

  it('antigravity 模型列表不包含图片模型', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.every(model => !model.includes('image') && !model.includes('imagen'))).toBe(true)
  })

  it('antigravity 模型列表包含 Gemini 3.1 Pro 通用别名', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.1-pro')
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.4 mini exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini'
    })
  })

  it('combined 模式会同时保留白名单身份映射和模型映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['gpt-5.4', 'claude-*'],
      [
        { from: 'gpt-latest', to: 'gpt-5.4' },
        { from: 'gpt-5.4', to: 'gpt-5.4-mini' }
      ]
    )

    expect(mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-mini',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('splitModelMappingObject 会把身份映射还原成白名单，其余保留为映射', () => {
    const parsed = splitModelMappingObject({
      'gpt-5.4': 'gpt-5.4',
      'gpt-latest': 'gpt-5.4',
      ' ': 'gpt-empty',
      broken: 123
    })

    expect(parsed).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'gpt-5.4' }]
    })
  })

  it('replaceExactModelsWithUpstream 会用上游精确模型覆盖旧白名单并保留通配符', () => {
    const result = replaceExactModelsWithUpstream(
      ['old-model', 'gpt-*', 'new-model', 'new-model', ' '],
      ['new-model', 'newer-model', 'newer-model', ' ']
    )

    expect(result.models).toEqual(['new-model', 'newer-model', 'gpt-*'])
    expect(result.addedCount).toBe(1)
    expect(result.removedCount).toBe(1)
    expect(result.preservedWildcardCount).toBe(1)
    expect(result.changed).toBe(true)
  })

  it('replaceIdentityModelMappingsWithUpstream 会刷新身份映射并保留手写映射', () => {
    const result = replaceIdentityModelMappingsWithUpstream(
      [
        { from: 'old-model', to: 'old-model' },
        { from: 'custom-model', to: 'target-model' },
        { from: 'new-model', to: 'custom-target' },
        { from: 'new-model', to: 'custom-target' }
      ],
      ['new-model', 'newer-model']
    )

    expect(result.mappings).toEqual([
      { from: 'newer-model', to: 'newer-model' },
      { from: 'custom-model', to: 'target-model' },
      { from: 'new-model', to: 'custom-target' }
    ])
    expect(result.addedCount).toBe(1)
    expect(result.removedCount).toBe(1)
    expect(result.preservedMappingCount).toBe(2)
    expect(result.changed).toBe(true)
  })
})
