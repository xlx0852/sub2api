<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/ui/Toggle.vue'
import type { AuthSourceType } from '@/api/admin/settings'

defineProps<{
  authSourceDefaultsMeta: { source: AuthSourceType; title: string; description: string }[]
}>()

const authSourceDefaults = defineModel<Record<string, any>>('authSourceDefaults', { required: true })

const form = defineModel<Record<string, any>>('form', { required: true })

const { t } = useI18n()
</script>

<template>
  <div class="space-y-6">
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.defaults.title") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.defaults.description") }}
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.defaults.defaultBalance") }}
                  </label>
                  <input
                    v-model.number="form.default_balance"
                    type="number"
                    step="0.01"
                    min="0"
                    class="input"
                    placeholder="0.00"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.defaults.defaultBalanceHint") }}
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.defaults.defaultConcurrency") }}
                  </label>
                  <input
                    v-model.number="form.default_concurrency"
                    type="number"
                    min="1"
                    class="input"
                    placeholder="1"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.defaults.defaultConcurrencyHint") }}
                  </p>
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                  >
                    {{ t("admin.settings.defaults.defaultUserRpmLimit") }}
                  </label>
                  <input
                    v-model.number="form.default_user_rpm_limit"
                    type="number"
                    min="0"
                    step="1"
                    class="input"
                    placeholder="0"
                  />
                  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.defaults.defaultUserRpmLimitHint") }}
                  </p>
                </div>
              </div>

              <!-- ★ 新增：系统全局默认平台限额矩阵 -->
              <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
                <div class="mb-3">
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t("admin.settings.defaults.defaultPlatformQuotas") }}
                  </label>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.defaults.defaultPlatformQuotasHint") }}
                  </p>
                  <p class="mt-0.5 text-xs text-amber-600 dark:text-amber-400">
                    {{ t("admin.settings.defaults.platformQuotaNotice") }}
                  </p>
                </div>
                <div class="overflow-x-auto">
                  <table class="min-w-full text-sm">
                    <thead>
                      <tr class="text-left text-xs text-gray-500 dark:text-gray-400">
                        <th class="pb-2 pr-4 font-medium">{{ t("admin.settings.platformQuota.platform") }}</th>
                        <th class="pb-2 pr-4 font-medium">{{ t("admin.settings.platformQuota.daily") }}</th>
                        <th class="pb-2 pr-4 font-medium">{{ t("admin.settings.platformQuota.weekly") }}</th>
                        <th class="pb-2 font-medium">{{ t("admin.settings.platformQuota.monthly") }}</th>
                      </tr>
                    </thead>
                    <tbody class="space-y-2">
                      <tr v-for="p in (['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi'] as const)" :key="p" class="align-top">
                        <td class="pr-4 py-1">
                          <span class="font-mono text-xs text-gray-700 dark:text-gray-300">{{ p }}</span>
                        </td>
                        <td class="pr-4 py-1">
                          <input
                            v-model.number="form.default_platform_quotas[p]!.daily"
                            type="number"
                            step="0.01"
                            min="0"
                            class="input h-8 w-28 text-sm"
                            :placeholder="t('admin.settings.platformQuota.placeholder')"
                          />
                        </td>
                        <td class="pr-4 py-1">
                          <input
                            v-model.number="form.default_platform_quotas[p]!.weekly"
                            type="number"
                            step="0.01"
                            min="0"
                            class="input h-8 w-28 text-sm"
                            :placeholder="t('admin.settings.platformQuota.placeholder')"
                          />
                        </td>
                        <td class="py-1">
                          <input
                            v-model.number="form.default_platform_quotas[p]!.monthly"
                            type="number"
                            step="0.01"
                            min="0"
                            class="input h-8 w-28 text-sm"
                            :placeholder="t('admin.settings.platformQuota.placeholder')"
                          />
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
              <!-- /全局平台限额矩阵 -->
            </div>
          </div>

          <!-- 模型级全局并发预算 -->
          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.modelConcurrency.title") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.modelConcurrency.description") }}
              </p>
            </div>
            <div class="space-y-4 p-6">
              <div>
                <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t("admin.settings.modelConcurrency.limitsLabel") }}
                </label>
                <textarea
                  v-model="form.model_concurrency_limits_text"
                  rows="5"
                  class="input w-full resize-y font-mono text-xs"
                  spellcheck="false"
                  :placeholder="t('admin.settings.modelConcurrency.placeholder')"
                ></textarea>
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.modelConcurrency.hint") }}
                </p>
              </div>
              <div class="rounded-lg bg-black/[0.03] px-4 py-3 dark:bg-white/[0.05]">
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.modelConcurrency.help") }}
                </p>
              </div>
            </div>
          </div>

          <div class="card">
            <div
              class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
            >
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t("admin.settings.authSourceDefaults.title") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.authSourceDefaults.description") }}
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div
                class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
              >
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t("admin.settings.authSourceDefaults.requireEmailLabel") }}
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t("admin.settings.authSourceDefaults.requireEmailHint") }}
                  </p>
                </div>
                <Toggle v-model="form.force_email_on_third_party_signup" />
              </div>

              <div class="space-y-4">
                <div
                  v-for="authSource in authSourceDefaultsMeta"
                  :key="authSource.source"
                  class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"
                >
                  <div class="flex items-center justify-between gap-4">
                    <div>
                      <div class="font-medium text-gray-900 dark:text-white">
                        {{ authSource.title }}
                      </div>
                      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                        {{ authSource.description }}
                      </p>
                    </div>
                    <Toggle
                      v-model="
                        authSourceDefaults[authSource.source].grant_on_signup
                      "
                      :data-testid="`auth-source-${authSource.source}-enabled`"
                    />
                  </div>

                  <div
                    v-if="authSourceDefaults[authSource.source].grant_on_signup"
                    :data-testid="`auth-source-${authSource.source}-panel`"
                    class="mt-4 space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
                  >
                    <p class="text-sm text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.authSourceDefaults.enabledHint") }}
                    </p>

                    <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                      <div>
                        <label
                          class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                        >
                          {{ t("admin.settings.defaults.defaultBalance") }}
                        </label>
                        <input
                          v-model.number="
                            authSourceDefaults[authSource.source].balance
                          "
                          type="number"
                          step="0.01"
                          min="0"
                          class="input"
                          placeholder="0.00"
                        />
                      </div>
                      <div>
                        <label
                          class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
                        >
                          {{ t("admin.settings.defaults.defaultConcurrency") }}
                        </label>
                        <input
                          v-model.number="
                            authSourceDefaults[authSource.source].concurrency
                          "
                          type="number"
                          min="1"
                          class="input"
                          placeholder="5"
                        />
                      </div>
                    </div>

                    <div
                      class="flex items-center justify-between rounded border border-gray-200 px-4 py-3 dark:border-dark-700"
                    >
                      <div>
                        <label
                          class="font-medium text-gray-900 dark:text-white"
                        >
                          {{ t("admin.settings.authSourceDefaults.grantOnFirstBindLabel") }}
                        </label>
                        <p
                          class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
                        >
                          {{ t("admin.settings.authSourceDefaults.grantOnFirstBindHint") }}
                        </p>
                      </div>
                      <Toggle
                        v-model="
                          authSourceDefaults[authSource.source]
                            .grant_on_first_bind
                        "
                      />
                    </div>

                    <!-- ★ 新增：auth source 平台限额覆盖区块 -->
                    <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
                      <div class="mb-3">
                        <label class="font-medium text-gray-900 dark:text-white">
                          {{ t("admin.settings.authSourceDefaults.platformQuotasOverride") }}
                        </label>
                        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                          {{ t("admin.settings.authSourceDefaults.platformQuotasOverrideHint") }}
                        </p>
                      </div>
                      <div class="overflow-x-auto">
                        <table class="min-w-full text-sm">
                          <thead>
                            <tr class="text-left text-xs text-gray-500 dark:text-gray-400">
                              <th class="pb-2 pr-4 font-medium">{{ t("admin.settings.platformQuota.platform") }}</th>
                              <th class="pb-2 pr-4 font-medium">{{ t("admin.settings.platformQuota.daily") }}</th>
                              <th class="pb-2 pr-4 font-medium">{{ t("admin.settings.platformQuota.weekly") }}</th>
                              <th class="pb-2 font-medium">{{ t("admin.settings.platformQuota.monthly") }}</th>
                            </tr>
                          </thead>
                          <tbody>
                            <tr v-for="p in (['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi'] as const)" :key="`${authSource.source}-pq-${p}`" class="align-top">
                              <td class="pr-4 py-1">
                                <span class="font-mono text-xs text-gray-700 dark:text-gray-300">{{ p }}</span>
                              </td>
                              <td class="pr-4 py-1">
                                <input
                                  v-model.number="authSourceDefaults[authSource.source].platform_quotas[p]!.daily"
                                  type="number"
                                  step="0.01"
                                  min="0"
                                  class="input h-8 w-28 text-sm"
                                  :placeholder="t('admin.settings.platformQuota.placeholder')"
                                />
                              </td>
                              <td class="pr-4 py-1">
                                <input
                                  v-model.number="authSourceDefaults[authSource.source].platform_quotas[p]!.weekly"
                                  type="number"
                                  step="0.01"
                                  min="0"
                                  class="input h-8 w-28 text-sm"
                                  :placeholder="t('admin.settings.platformQuota.placeholder')"
                                />
                              </td>
                              <td class="py-1">
                                <input
                                  v-model.number="authSourceDefaults[authSource.source].platform_quotas[p]!.monthly"
                                  type="number"
                                  step="0.01"
                                  min="0"
                                  class="input h-8 w-28 text-sm"
                                  :placeholder="t('admin.settings.platformQuota.placeholder')"
                                />
                              </td>
                            </tr>
                          </tbody>
                        </table>
                      </div>
                    </div>
                    <!-- /auth source 平台限额覆盖区块 -->
                  </div>
                </div>
              </div>
            </div>
          </div>
  </div>
</template>
