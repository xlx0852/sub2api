<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200"
      enter-from-class="opacity-0"
      leave-active-class="transition-opacity duration-150"
      leave-to-class="opacity-0"
    >
      <div v-if="show && account" class="pointer-events-none fixed inset-0 z-[90]" role="presentation">
        <Transition
          appear
          enter-active-class="transition-transform duration-200 ease-out"
          enter-from-class="translate-x-full"
          leave-active-class="transition-transform duration-150 ease-in"
          leave-to-class="translate-x-full"
        >
          <aside
            v-if="show && account"
            ref="drawerRef"
            class="pointer-events-auto absolute inset-y-0 right-0 flex min-w-0 w-full flex-col overflow-hidden border-l border-gray-200 bg-white sm:w-[560px] dark:border-dark-600 dark:bg-dark-900"
            role="dialog"
            aria-modal="false"
            :aria-labelledby="drawerTitleId"
          >
            <header class="flex min-w-0 shrink-0 items-start justify-between gap-4 border-b border-gray-200 px-4 py-4 sm:px-5 dark:border-dark-600">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 :id="drawerTitleId" class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ t('admin.accounts.usageDetails.title') }}
                  </h2>
                  <span :class="['inline-flex h-6 items-center rounded px-2 text-xs font-medium', statusClasses]">
                    {{ usageState.presentation.statusLabel }}
                  </span>
                </div>
                <div class="mt-3 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5">
                  <span class="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100" :title="account.name">
                    {{ account.name }}
                  </span>
                  <span class="shrink-0 font-mono text-xs text-gray-400">#{{ account.id }}</span>
                  <div class="min-w-0 overflow-hidden">
                    <PlatformTypeBadge
                      :platform="account.platform"
                      :type="account.type"
                      :plan-type="usageState.presentation.plan || ''"
                      :privacy-mode="accountPrivacyMode"
                      :subscription-expires-at="accountSubscriptionExpiresAt"
                    />
                  </div>
                </div>
              </div>
              <button
                ref="closeButtonRef"
                type="button"
                class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-primary-500/40 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
                :aria-label="t('common.close')"
                @click="emit('close')"
              >
                <Icon name="x" size="sm" />
              </button>
            </header>

            <nav class="grid shrink-0 grid-cols-4 gap-1 border-b border-gray-200 px-4 py-2 dark:border-dark-600" role="tablist">
              <button
                v-for="tab in tabs"
                :key="tab.key"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.key"
                :class="[
                  'inline-flex h-9 min-w-0 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/40',
                  activeTab === tab.key
                    ? 'bg-primary-600 text-white dark:bg-primary-500'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'
                ]"
                @click="activeTab = tab.key"
              >
                <Icon :name="tab.icon" size="xs" />
                <span class="truncate">{{ tab.label }}</span>
              </button>
            </nav>

            <div class="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto overflow-x-hidden px-4 py-5 sm:px-5">
              <section v-show="activeTab === 'quota'" role="tabpanel" class="space-y-5">
                <div>
                  <div class="mb-4 flex items-start justify-between gap-4">
                    <div class="min-w-0">
                      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.usageDetails.officialQuota') }}</h3>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.officialQuotaHint') }}</p>
                    </div>
                    <span v-if="usageState.presentation.updatedAt" class="inline-flex shrink-0 items-center gap-1 text-xs text-gray-400 dark:text-gray-500">
                      <Icon name="clock" size="xs" />
                      {{ t('admin.accounts.usageDetails.updatedAtLabel', { time: formatRelativeTime(usageState.presentation.updatedAt, translateRelativeTime) }) }}
                    </span>
                  </div>
                  <AccountUsageCell
                    :key="`detail-${account.id}`"
                    :account="account"
                    :today-stats="todayStats"
                    :today-stats-loading="todayStatsLoading"
                    :manual-refresh-token="manualRefreshToken"
                    variant="detail"
                    @state-change="handleUsageState"
                  />
                </div>
              </section>

              <section v-if="activeTab === 'statistics'" role="tabpanel">
                <AccountStatsPanel :account="account" />
              </section>

              <section v-show="activeTab === 'performance'" role="tabpanel" class="space-y-5">
                <div>
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.usageDetails.performance24h') }}</h3>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.performanceHint') }}</p>
                </div>
                <div v-if="performanceLoading" class="grid grid-cols-2 gap-3" aria-busy="true">
                  <div v-for="index in 6" :key="index" class="h-16 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
                </div>
                <div v-else-if="performanceStats?.stats?.length" class="space-y-5">
                  <div v-for="stat in performanceStats.stats" :key="stat.request_type">
                    <div class="mb-3 flex items-center justify-between gap-3 border-b border-gray-200 pb-2 dark:border-dark-600">
                      <div>
                        <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ requestTypeLabel(stat.request_type) }}</span>
                        <span class="ml-2 text-xs text-gray-400 dark:text-gray-500">{{ stat.request_count }} {{ t('admin.accounts.usageDetails.requestsUnit') }}</span>
                      </div>
                      <span v-if="(stat.ws_preflight_fail_count || 0) > 0" class="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-950/60 dark:text-amber-300">
                        {{ t('admin.accounts.usageDetails.preflightFailures', { count: stat.ws_preflight_fail_count || 0 }) }}
                      </span>
                    </div>
                    <div class="grid grid-cols-2 gap-x-5 gap-y-4 sm:grid-cols-3">
                      <MetricItem :label="t('admin.accounts.usageDetails.avgLatency')" :value="formatDuration(stat.avg_duration_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.p90Latency')" :value="formatDuration(stat.p90_duration_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.avgFirstToken')" :value="formatDuration(stat.avg_first_token_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.connectionReuse')" :value="formatPercent(stat.ws_conn_reused_rate)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.queueWait')" :value="formatDuration(stat.avg_ws_queue_wait_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.payload')" :value="formatBytes(stat.avg_ws_payload_bytes)" />
                    </div>
                  </div>
                </div>
                <div v-else class="rounded-md border border-dashed border-gray-200 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
                  {{ t('admin.accounts.usageDetails.noPerformanceData') }}
                </div>
              </section>

              <section v-show="activeTab === 'diagnostics'" role="tabpanel" class="flex min-h-0 flex-1 flex-col space-y-3">
                <div>
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.usageDetails.diagnostics') }}</h3>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.diagnosticsHint') }}</p>
                </div>
                <dl class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-600 dark:border-dark-600">
                  <div class="grid grid-cols-[110px_minmax(0,1fr)] gap-3 py-2">
                    <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.accountStatus') }}</dt>
                    <dd class="text-right text-xs font-medium text-gray-800 dark:text-gray-100">{{ account.status }}</dd>
                  </div>
                  <div
                    v-for="item in usageState.presentation.diagnostics"
                    :key="item.label"
                    class="grid grid-cols-[110px_minmax(0,1fr)] gap-3 py-2"
                  >
                    <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
                    <dd :class="['break-words text-right text-xs font-medium', diagnosticValueClass(item.tone)]">
                      {{ formatDiagnosticValue(item.value) }}
                    </dd>
                  </div>
                </dl>
                <div v-if="usageState.error" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
                  {{ usageState.error }}
                </div>
                <div v-if="usageState.presentation.needsReauth" class="flex justify-end">
                  <button
                    type="button"
                    class="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary-600 px-3 text-xs font-medium text-white transition-colors hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600"
                    @click="emit('reauthorize', account)"
                  >
                    <Icon name="link" size="xs" />
                    {{ t('admin.accounts.reAuthorize') }}
                  </button>
                </div>

                <div class="rounded-lg border border-gray-200 bg-gray-50/50 p-2.5 dark:border-dark-600 dark:bg-dark-800/40">
                  <ScheduledTestsPanel
                    :key="`scheduled-panel-${account?.id || 'none'}`"
                    :show="show && activeTab === 'diagnostics' && !!account"
                    :account-id="account?.id ?? null"
                    :plans="scheduledTestPlans"
                    :loading="scheduledTestsLoading"
                    embedded
                    @changed="loadScheduledTests(true)"
                  />
                </div>

                <div class="flex min-h-0 flex-1 flex-col space-y-2 border-t border-gray-200 pt-3 dark:border-dark-600">
                  <div class="flex items-center justify-between gap-2">
                    <div class="min-w-0">
                      <h3 class="text-xs font-semibold text-gray-900 dark:text-white">
                        {{ t('admin.accounts.usageDetails.scheduledTestsHistory') }}
                      </h3>
                    </div>
                    <div class="flex shrink-0 items-center gap-1">
                      <button
                        v-if="scheduledTestEntries.length === 0"
                        type="button"
                        class="inline-flex h-7 items-center gap-1 rounded-md bg-primary-600 px-2 text-[11px] font-medium text-white transition-colors hover:bg-primary-700 disabled:opacity-50 dark:bg-primary-500 dark:hover:bg-primary-600"
                        :disabled="scheduledTestsEnabling || scheduledTestsLoading"
                        @click="enableScheduledDiagnostics"
                      >
                        <Icon name="plus" size="xs" :class="scheduledTestsEnabling ? 'animate-spin' : ''" />
                        {{ scheduledTestsEnabling
                          ? t('admin.accounts.usageDetails.scheduledTestsEnabling')
                          : t('admin.accounts.usageDetails.scheduledTestsEnable') }}
                      </button>
                      <button
                        type="button"
                        class="inline-flex h-7 items-center gap-1 rounded-md px-1.5 text-[11px] font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
                        :disabled="scheduledTestsLoading || scheduledTestsEnabling"
                        @click="loadScheduledTests(true)"
                      >
                        <Icon name="refresh" size="xs" :class="scheduledTestsLoading ? 'animate-spin' : ''" />
                        {{ t('admin.accounts.usageDetails.scheduledTestsRefresh') }}
                      </button>
                    </div>
                  </div>

                  <div v-if="scheduledTestsLoading && scheduledTestEntries.length === 0" class="flex items-center gap-2 py-6 text-xs text-gray-500 dark:text-gray-400">
                    <Icon name="refresh" size="xs" class="animate-spin" />
                    {{ t('admin.accounts.usageDetails.scheduledTestsLoading') }}
                  </div>

                  <div
                    v-else-if="scheduledTestsError"
                    class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300"
                  >
                    {{ scheduledTestsError }}
                  </div>

                  <div
                    v-else-if="scheduledTestEntries.length === 0"
                    class="rounded-md border border-dashed border-gray-200 px-3 py-8 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400"
                  >
                    <p>{{ t('admin.accounts.usageDetails.scheduledTestsEmpty') }}</p>
                    <p class="mt-2 text-[11px] text-gray-400 dark:text-gray-500">
                      {{ t('admin.accounts.usageDetails.scheduledTestsRunHint') }}
                    </p>
                  </div>

                  <div v-else class="flex min-h-0 flex-1 flex-col space-y-2">
                    <div
                      v-for="entry in scheduledTestEntries"
                      :key="entry.plan.id"
                      class="flex min-h-0 flex-1 flex-col rounded-lg border border-gray-200 bg-gray-50/70 p-2.5 dark:border-dark-600 dark:bg-dark-800/60"
                    >
                      <div class="flex flex-wrap items-center justify-between gap-1.5">
                        <div class="min-w-0">
                          <div class="flex min-w-0 flex-wrap items-center gap-1.5">
                            <div class="truncate text-xs font-semibold text-gray-900 dark:text-white">
                              {{ entry.plan.model_id }}
                            </div>
                            <span class="font-mono text-[10px] text-gray-400 dark:text-gray-500">#{{ entry.plan.id }}</span>
                            <span
                              v-if="!entry.plan.enabled"
                              class="inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded px-1.5 text-[11px] font-medium bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300"
                            >
                              {{ t('admin.accounts.usageDetails.scheduledTestsDisabled') }}
                            </span>
                            <span
                              v-else-if="entry.results[0]"
                              :class="['inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded px-1.5 text-[11px] font-medium', scheduledStatusClass(entry.results[0].status)]"
                            >
                              {{ scheduledStatusLabel(entry.results[0].status) }}
                            </span>
                          </div>
                          <div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-gray-500 dark:text-gray-400">
                            <span class="font-mono">{{ entry.plan.cron_expression }}</span>
                            <span>
                              {{ t('admin.accounts.usageDetails.scheduledTestsLastRun') }}
                              <span class="font-medium text-gray-700 dark:text-gray-200">{{ formatScheduledTime(entry.results[0]?.started_at || entry.plan.last_run_at) }}</span>
                            </span>
                            <span>
                              {{ t('admin.accounts.usageDetails.scheduledTestsNextRun') }}
                              <span class="font-medium text-gray-700 dark:text-gray-200">{{ formatScheduledTime(entry.plan.next_run_at) }}</span>
                            </span>
                            <span v-if="entry.results[0]">
                              {{ t('admin.accounts.usageDetails.scheduledTestsLatency') }}
                              <span class="font-medium text-gray-700 dark:text-gray-200">{{ formatDuration(entry.results[0].latency_ms) }}</span>
                            </span>
                          </div>
                        </div>
                      </div>

                      <div class="mt-2 flex min-h-0 flex-1 flex-col border-t border-gray-200 pt-2 dark:border-dark-600">
                        <div class="mb-1.5 flex shrink-0 items-center justify-between gap-2">
                          <div class="text-[11px] font-medium text-gray-600 dark:text-gray-300">
                            {{ t('admin.accounts.usageDetails.scheduledTestsHistory') }}
                          </div>
                          <div class="text-[11px] text-gray-400 dark:text-gray-500">
                            {{ t('admin.accounts.usageDetails.scheduledTestsShowing', {
                              count: entry.results.length,
                              max: entry.plan.max_results || 50
                            }) }}
                          </div>
                        </div>

                        <div v-if="entry.results.length === 0" class="text-[11px] text-gray-500 dark:text-gray-400">
                          {{ t('admin.accounts.usageDetails.scheduledTestsNoResults') }}
                        </div>
                        <div v-else class="min-h-0 flex-1 space-y-2 overflow-y-auto overscroll-contain pr-1">
                          <div
                            v-for="result in entry.results"
                            :key="result.id"
                            class="rounded-md border border-gray-200 bg-white px-2 py-1.5 dark:border-dark-600 dark:bg-dark-900"
                          >
                            <template v-for="detail in [parseScheduledDiagnosticsDetail(result)]" :key="`diag-${result.id}`">
                              <div class="flex flex-wrap items-center justify-between gap-2">
                                <div class="flex min-w-0 flex-wrap items-center gap-1.5">
                                  <span :class="['inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded px-1.5 text-[11px] font-medium', scheduledStatusClass(result.status)]">
                                    {{ scheduledStatusLabel(result.status) }}
                                  </span>
                                  <span
                                    v-if="detail.upgraded"
                                    class="inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded bg-sky-100 px-1.5 text-[11px] font-medium text-sky-700 dark:bg-sky-950/50 dark:text-sky-300"
                                  >
                                    {{ t('admin.accounts.usageDetails.scheduledTestsUpgraded') }}
                                  </span>
                                  <span
                                    v-else-if="detail.allFailed"
                                    class="inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded bg-rose-100 px-1.5 text-[11px] font-medium text-rose-700 dark:bg-rose-950/50 dark:text-rose-300"
                                  >
                                    {{ t('admin.accounts.usageDetails.scheduledTestsAllFailed') }}
                                  </span>
                                  <span
                                    v-if="detail.statusCode"
                                    class="inline-flex h-5 shrink-0 items-center whitespace-nowrap rounded bg-gray-100 px-1.5 font-mono text-[11px] font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-200"
                                  >
                                    {{ detail.statusCode }}
                                  </span>
                                  <span
                                    v-if="detail.errorType"
                                    class="inline-flex h-5 max-w-[10rem] shrink-0 items-center truncate whitespace-nowrap rounded bg-gray-100 px-1.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                                    :title="detail.errorType"
                                  >
                                    {{ detail.errorType }}
                                  </span>
                                  <span class="shrink-0 text-[11px] text-gray-500 dark:text-gray-400">
                                    {{ formatDuration(result.latency_ms) }}
                                  </span>
                                </div>
                                <span class="shrink-0 text-[11px] text-gray-500 dark:text-gray-400">
                                  {{ formatScheduledTime(result.started_at || result.created_at) }}
                                </span>
                              </div>

                              <div
                                v-if="detail.modelUsed || (detail.upgraded && detail.failedCount > 0)"
                                class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]"
                              >
                                <div v-if="detail.modelUsed" class="min-w-0">
                                  <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.scheduledTestsModelUsed') }}</span>
                                  <span class="ml-1 break-all font-mono font-medium text-gray-800 dark:text-gray-100">{{ detail.modelUsed }}</span>
                                </div>
                                <div
                                  v-if="detail.upgraded && detail.failedCount > 0"
                                  class="text-amber-700 dark:text-amber-300"
                                >
                                  {{ t('admin.accounts.usageDetails.scheduledTestsAttemptCount', { count: detail.failedCount }) }}
                                </div>
                              </div>

                              <div
                                v-if="detail.summary && (detail.upgraded || detail.allFailed || detail.failedModels.length > 0 || detail.statusCode || detail.errorType)"
                                class="mt-2 text-[11px] leading-relaxed"
                                :class="result.error_message ? 'text-red-600 dark:text-red-300' : 'text-gray-700 dark:text-gray-300'"
                              >
                                {{ detail.summary }}
                              </div>

                              <div
                                v-if="detail.failedModels.length > 0"
                                class="mt-2 rounded-md border border-amber-200/80 bg-amber-50/80 px-2 py-1.5 dark:border-amber-900/50 dark:bg-amber-950/30"
                              >
                                <div class="mb-1 text-[11px] font-medium text-amber-800 dark:text-amber-200">
                                  {{ detail.allFailed
                                    ? t('admin.accounts.usageDetails.scheduledTestsTriedModels')
                                    : t('admin.accounts.usageDetails.scheduledTestsFailedChain') }}
                                </div>
                                <ul class="space-y-1">
                                  <li
                                    v-for="(item, idx) in detail.failedModels"
                                    :key="`${result.id}-fail-${idx}`"
                                    class="text-[11px] leading-relaxed text-amber-900/90 dark:text-amber-100/90"
                                  >
                                    <div class="flex flex-wrap items-center gap-1.5">
                                      <span class="font-mono font-medium">{{ item.model || '-' }}</span>
                                      <span
                                        v-if="item.statusCode"
                                        class="inline-flex h-4 items-center rounded bg-white/80 px-1 font-mono text-[10px] text-amber-800 dark:bg-dark-900/70 dark:text-amber-200"
                                      >{{ item.statusCode }}</span>
                                      <span
                                        v-if="item.errorType"
                                        class="inline-flex h-4 max-w-[9rem] items-center truncate rounded bg-white/80 px-1 text-[10px] text-amber-800 dark:bg-dark-900/70 dark:text-amber-200"
                                        :title="item.errorType"
                                      >{{ item.errorType }}</span>
                                    </div>
                                    <div v-if="item.reason" class="mt-0.5 text-amber-800/90 dark:text-amber-100/80">
                                      {{ item.reason }}
                                    </div>
                                  </li>
                                </ul>
                              </div>

                              <div
                                v-if="detail.body && !detail.summaryMatchesBody"
                                class="mt-2"
                              >
                                <div class="mb-1 text-[11px] font-medium text-gray-500 dark:text-gray-400">
                                  {{ result.error_message
                                    ? (detail.failedModels.length > 0
                                      ? t('admin.accounts.usageDetails.scheduledTestsRawError')
                                      : t('admin.accounts.usageDetails.scheduledTestsError'))
                                    : t('admin.accounts.usageDetails.scheduledTestsReply') }}
                                </div>
                                <pre
                                  :class="[
                                    'max-h-28 overflow-auto whitespace-pre-wrap break-words font-sans text-[11px] leading-relaxed',
                                    result.error_message
                                      ? 'text-red-600/90 dark:text-red-300/90'
                                      : 'text-gray-700 dark:text-gray-300'
                                  ]"
                                >{{ truncateText(detail.body, 600) }}</pre>
                              </div>

                              <div
                                v-else-if="detail.body && !detail.upgraded && !detail.allFailed && detail.failedModels.length === 0"
                                class="mt-2"
                              >
                                <pre
                                  :class="[
                                    'max-h-28 overflow-auto whitespace-pre-wrap break-words font-sans text-[11px] leading-relaxed',
                                    result.error_message
                                      ? 'text-red-600 dark:text-red-300'
                                      : 'text-gray-700 dark:text-gray-300'
                                  ]"
                                >{{ truncateText(detail.body, 600) }}</pre>
                              </div>
                            </template>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </section>
            </div>

          </aside>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, AccountUsageInfo, ScheduledTestPlan, ScheduledTestResult, WindowStats } from '@/types'
import type { AccountPerformanceStats } from '@/api/admin/accounts'
import { scheduledTestsAPI } from '@/api/admin/scheduledTests'
import ScheduledTestsPanel from '@/components/admin/account/ScheduledTestsPanel.vue'
import type { AccountUsagePresentation, AccountUsageTone } from '@/utils/accountUsagePresentation'
import { buildAccountUsagePresentation } from '@/utils/accountUsagePresentation'
import { formatRelativeTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import AccountUsageCell from './AccountUsageCell.vue'
import MetricItem from './MetricItem.vue'
import AccountStatsPanel from '@/components/admin/account/AccountStatsModal.vue'

type ScheduledTestEntry = {
  plan: ScheduledTestPlan
  results: ScheduledTestResult[]
}

type UsageTab = 'quota' | 'statistics' | 'performance' | 'diagnostics'
type IconName = InstanceType<typeof Icon>['$props']['name']

const props = withDefaults(defineProps<{
  show: boolean
  account: Account | null
  todayStats?: WindowStats | null
  todayStatsLoading?: boolean
  performanceStats?: AccountPerformanceStats | null
  performanceLoading?: boolean
  manualRefreshToken?: number
  initialTab?: 'quota' | 'statistics' | 'diagnostics' | 'performance'
}>(), {
  todayStats: null,
  todayStatsLoading: false,
  performanceStats: null,
  performanceLoading: false,
  manualRefreshToken: 0,
  initialTab: 'quota'
})

const emit = defineEmits<{
  close: []
  reauthorize: [account: Account]
}>()

const { t } = useI18n()
const translateRelativeTime = (key: string, params?: Record<string, number>) => t(key, params || {})
const drawerRef = ref<HTMLElement | null>(null)
const closeButtonRef = ref<HTMLButtonElement | null>(null)
const activeTab = ref<UsageTab>('quota')
const previousActiveElement = ref<HTMLElement | null>(null)
const drawerTitleId = `account-usage-drawer-title-${Math.random().toString(36).slice(2)}`

const scheduledTestEntries = ref<ScheduledTestEntry[]>([])
const scheduledTestPlans = computed(() => scheduledTestEntries.value.map((entry) => entry.plan))
const scheduledTestsLoading = ref(false)
const scheduledTestsEnabling = ref(false)
const scheduledTestsError = ref<string | null>(null)
const scheduledTestsLoadedFor = ref<number | null>(null)
let scheduledTestsRequestSeq = 0

const emptyPresentation = computed(() => buildAccountUsagePresentation({
  account: props.account || ({ id: 0, platform: 'openai', type: 'apikey' } as Account),
  usageInfo: null,
  todayStats: props.todayStats,
  t
}))

const usageState = ref<{
  usageInfo: AccountUsageInfo | null
  loading: boolean
  error: string | null
  presentation: AccountUsagePresentation
}>({
  usageInfo: null,
  loading: false,
  error: null,
  presentation: emptyPresentation.value
})

const tabs = computed<Array<{ key: UsageTab; label: string; icon: IconName }>>(() => [
  { key: 'quota', label: t('admin.accounts.usageDetails.tabs.quota'), icon: 'chart' },
  { key: 'statistics', label: t('admin.accounts.usageDetails.tabs.statistics'), icon: 'bolt' },
  { key: 'performance', label: t('admin.accounts.usageDetails.tabs.performance'), icon: 'clock' },
  { key: 'diagnostics', label: t('admin.accounts.usageDetails.tabs.diagnostics'), icon: 'beaker' }
])

const statusClasses = computed(() => ({
  neutral: 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300',
  success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300',
  danger: 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300'
}[usageState.value.presentation.statusTone]))

const accountPrivacyMode = computed(() => {
  const value = props.account?.extra?.privacy_mode
  return typeof value === 'string' ? value : props.account?.parent_privacy_mode
})

const accountSubscriptionExpiresAt = computed(() => {
  const value = props.account?.credentials?.subscription_expires_at
  return typeof value === 'string' ? value : props.account?.parent_subscription_expires_at
})

const handleUsageState = (state: typeof usageState.value) => {
  usageState.value = state
}

const handleKeydown = (event: KeyboardEvent) => {
  if (!props.show) return
  if (event.key === 'Escape') emit('close')
}

const syncActiveTabFromProps = () => {
  activeTab.value = (props.initialTab as UsageTab) || 'quota'
}

const maybeLoadDiagnostics = (force = false) => {
  if (!props.show || activeTab.value !== 'diagnostics' || !props.account?.id) return
  void loadScheduledTests(force)
}

watch(
  () => props.show,
  async (show) => {
    if (show) {
      syncActiveTabFromProps()
      previousActiveElement.value = document.activeElement as HTMLElement | null
      await nextTick()
      closeButtonRef.value?.focus()
      window.addEventListener('keydown', handleKeydown)
      maybeLoadDiagnostics(true)
      return
    }
    window.removeEventListener('keydown', handleKeydown)
    previousActiveElement.value?.focus()
  },
  { immediate: true }
)

watch(
  () => props.account?.id,
  () => {
    // Keep current tab when switching accounts only if drawer stays open on diagnostics.
    if (!props.show) {
      syncActiveTabFromProps()
    } else if (props.initialTab) {
      activeTab.value = props.initialTab as UsageTab
    }
    usageState.value = {
      usageInfo: null,
      loading: false,
      error: null,
      presentation: emptyPresentation.value
    }
    resetScheduledTests()
    maybeLoadDiagnostics(true)
  }
)

watch(
  () => activeTab.value,
  (tab) => {
    if (tab === 'diagnostics') maybeLoadDiagnostics(false)
  }
)

watch(
  () => props.initialTab,
  (tab) => {
    if (!props.show || !tab) return
    activeTab.value = tab as UsageTab
    maybeLoadDiagnostics(tab === 'diagnostics')
  }
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})

const formatDuration = (value?: number | null) => {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(1)}s`
  return `${Math.round(value)}ms`
}

const formatPercent = (value?: number | null) => {
  if (value == null || !Number.isFinite(value)) return '-'
  return `${Math.round(value * 100)}%`
}

const formatBytes = (value?: number | null) => {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)}MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)}KB`
  return `${Math.round(value)}B`
}

const requestTypeLabel = (requestType: string) => {
  if (requestType === 'ws_v2') return 'WS'
  if (requestType === 'stream') return t('admin.accounts.usageDetails.requestTypeStream')
  if (requestType === 'compact') return 'Compact'
  return requestType || '-'
}

const diagnosticValueClass = (tone?: AccountUsageTone) => ({
  success: 'text-emerald-600 dark:text-emerald-300',
  warning: 'text-amber-600 dark:text-amber-300',
  danger: 'text-red-600 dark:text-red-300',
  neutral: 'text-gray-800 dark:text-gray-100'
}[tone || 'neutral'])

const formatDiagnosticValue = (value: string) => {
  if (!value || value === '-') return '-'
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp) || !value.includes('T')) return value
  return formatRelativeTime(value)
}

const resetScheduledTests = () => {
  scheduledTestEntries.value = []
  scheduledTestsEnabling.value = false
  scheduledTestsError.value = null
  scheduledTestsLoadedFor.value = null
  // Keep loading=false here; maybeLoadDiagnostics/loadScheduledTests will set it true.
  scheduledTestsLoading.value = false
}

const enableScheduledDiagnostics = async () => {
  const accountId = props.account?.id
  if (!accountId || scheduledTestsEnabling.value) return
  scheduledTestsEnabling.value = true
  scheduledTestsError.value = null
  try {
    await scheduledTestsAPI.ensureDefault(accountId)
    await loadScheduledTests(true)
  } catch (error) {
    scheduledTestsError.value = error instanceof Error && error.message
      ? error.message
      : t('admin.accounts.usageDetails.scheduledTestsLoadFailed')
  } finally {
    scheduledTestsEnabling.value = false
  }
}

const loadScheduledTests = async (force = false) => {
  const accountId = props.account?.id
  if (!accountId) return
  if (!force && scheduledTestsLoading.value) return
  if (
    !force &&
    scheduledTestsLoadedFor.value === accountId &&
    !scheduledTestsError.value
  ) {
    return
  }

  const seq = ++scheduledTestsRequestSeq
  scheduledTestsLoading.value = true
  scheduledTestsError.value = null

  try {
    let plans = await scheduledTestsAPI.listByAccount(accountId)
    if (plans.length === 0) {
      try {
        const ensured = await scheduledTestsAPI.ensureDefault(accountId)
        if (ensured?.plan) {
          plans = [ensured.plan]
        } else {
          plans = await scheduledTestsAPI.listByAccount(accountId)
        }
      } catch {
        // Keep empty state; user can click enable manually.
      }
    }
    const sorted = [...plans].sort((a, b) => {
      if (a.enabled !== b.enabled) return a.enabled ? -1 : 1
      return b.id - a.id
    })

    const entries = await Promise.all(
      sorted.map(async (plan) => {
        const limit = Math.min(Math.max(plan.max_results || 50, 1), 200)
        try {
          const results = await scheduledTestsAPI.listResults(plan.id, limit)
          return { plan, results } satisfies ScheduledTestEntry
        } catch {
          return { plan, results: [] } satisfies ScheduledTestEntry
        }
      })
    )

    if (seq !== scheduledTestsRequestSeq) return
    scheduledTestEntries.value = entries
    scheduledTestsLoadedFor.value = accountId
  } catch (error) {
    if (seq !== scheduledTestsRequestSeq) return
    scheduledTestEntries.value = []
    scheduledTestsLoadedFor.value = null
    scheduledTestsError.value = error instanceof Error && error.message
      ? error.message
      : t('admin.accounts.usageDetails.scheduledTestsLoadFailed')
  } finally {
    if (seq === scheduledTestsRequestSeq) {
      scheduledTestsLoading.value = false
    }
  }
}

const scheduledStatusLabel = (status?: string | null) => {
  const normalized = (status || '').toLowerCase()
  if (normalized === 'success') return t('admin.accounts.usageDetails.scheduledTestsSuccess')
  if (normalized === 'failed' || normalized === 'error') return t('admin.accounts.usageDetails.scheduledTestsFailed')
  if (normalized === 'running') return t('admin.accounts.usageDetails.scheduledTestsRunning')
  return status || '-'
}

const scheduledStatusClass = (status?: string | null) => {
  const normalized = (status || '').toLowerCase()
  if (normalized === 'success') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'
  if (normalized === 'failed' || normalized === 'error') return 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300'
  if (normalized === 'running') return 'bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300'
  return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300'
}

const formatScheduledTime = (value?: string | null) => {
  if (!value) return '-'
  return formatRelativeTime(value, translateRelativeTime)
}

const truncateText = (value: string, max = 400) => {
  const textValue = (value || '').trim()
  if (textValue.length <= max) return textValue
  return `${textValue.slice(0, max)}…`
}

type ScheduledDiagFailedModel = {
  model: string
  reason: string
  statusCode: string
  errorType: string
}
type ScheduledDiagDetail = {
  upgraded: boolean
  allFailed: boolean
  modelUsed: string
  failedCount: number
  failedModels: ScheduledDiagFailedModel[]
  summary: string
  statusCode: string
  errorType: string
  body: string
  summaryMatchesBody: boolean
}

const scheduledDiagDetailCache = new WeakMap<object, ScheduledDiagDetail>()

const parseFailedModelLine = (rawLine: string): ScheduledDiagFailedModel => {
  const raw = rawLine.trim()
  const sep = raw.indexOf(':')
  if (sep > 0) {
    const model = raw.slice(0, sep).trim()
    const reasonRaw = raw.slice(sep + 1).trim()
    const parsed = parseDiagErrorText(reasonRaw)
    return {
      model,
      reason: parsed.summary || reasonRaw,
      statusCode: parsed.statusCode,
      errorType: parsed.errorType
    }
  }
  const parsed = parseDiagErrorText(raw)
  return {
    model: '',
    reason: parsed.summary || raw,
    statusCode: parsed.statusCode,
    errorType: parsed.errorType
  }
}

const parseScheduledDiagnosticsDetail = (result: {
  response_text?: string | null
  error_message?: string | null
}): ScheduledDiagDetail => {
  const cached = scheduledDiagDetailCache.get(result as object)
  if (cached) return cached

  const source = String(result.error_message || result.response_text || '').trim()
  const empty: ScheduledDiagDetail = {
    upgraded: false,
    allFailed: false,
    modelUsed: '',
    failedCount: 0,
    failedModels: [],
    summary: '',
    statusCode: '',
    errorType: '',
    body: source,
    summaryMatchesBody: false
  }
  if (!source) {
    scheduledDiagDetailCache.set(result as object, empty)
    return empty
  }

  let rest = source
  let upgraded = false
  let allFailed = false
  let modelUsed = ''
  let failedCount = 0
  const failedModels: ScheduledDiagFailedModel[] = []

  const upgradedMatch = rest.match(
    /^\[diagnostics upgraded model=([^\]]+?) after (\d+) failed attempt\(s\)\]\s*/i
  )
  const plainMatch = !upgradedMatch
    ? rest.match(/^\[diagnostics model=([^\]]+?)\]\s*/i)
    : null
  const allFailedMatch = !upgradedMatch && !plainMatch
    ? rest.match(/^all diagnostic models failed\s*/i)
    : null

  if (upgradedMatch) {
    upgraded = true
    modelUsed = upgradedMatch[1].trim()
    failedCount = Number(upgradedMatch[2] || 0)
    rest = rest.slice(upgradedMatch[0].length)
  } else if (plainMatch) {
    modelUsed = plainMatch[1].trim()
    rest = rest.slice(plainMatch[0].length)
  } else if (allFailedMatch) {
    allFailed = true
    rest = rest.slice(allFailedMatch[0].length).replace(/^:\s*/, '')
  }

  // failed_models: header (upgrade success path)
  const failedHeader = rest.match(/^failed_models:\s*\n?/i)
  if (failedHeader) {
    rest = rest.slice(failedHeader[0].length)
  }

  // bullet list of attempts (upgrade failed_models or all-failed chain)
  if (rest.includes('\n- ') || rest.startsWith('- ')) {
    const lines = rest.split('\n')
    const kept: string[] = []
    let inList = true
    for (const line of lines) {
      const trimmed = line.trim()
      if (inList && trimmed.startsWith('- ')) {
        failedModels.push(parseFailedModelLine(trimmed.slice(2)))
        continue
      }
      // allow blank lines inside list header area
      if (inList && trimmed === '') continue
      inList = false
      kept.push(line)
    }
    rest = kept.join('\n').trim()
    if (!failedCount) failedCount = failedModels.length
    if (!allFailed && !upgraded && failedModels.length > 1) {
      allFailed = true
    }
  }

  rest = rest.replace(/^\[diagnostics[^\]]*\]\s*/i, '').trim()

  // If still no structured failures, try single "model: error" line
  if (failedModels.length === 0) {
    const singleModel = rest.match(/^([a-zA-Z0-9._:-]{3,80}):\s+(.+)$/s)
    if (singleModel && /model|gpt|claude|gemini|grok|kimi|codex|haiku|sonnet/i.test(singleModel[1])) {
      failedModels.push(parseFailedModelLine(`${singleModel[1]}: ${singleModel[2]}`))
      rest = ''
    }
  }

  const topParsed = parseDiagErrorText(rest || source)
  let summary = ''
  if (upgraded) {
    summary = topParsed.summary || rest
  } else if (allFailed) {
    summary =
      failedModels[0]?.reason ||
      topParsed.summary ||
      'all diagnostic models failed'
  } else if (result.error_message) {
    summary = topParsed.summary || rest
  } else {
    summary = ''
  }

  // Prefer first failed item status/type when top-level lacks them
  const statusCode = topParsed.statusCode || failedModels[0]?.statusCode || ''
  const errorType = topParsed.errorType || failedModels[0]?.errorType || ''

  // body: keep reply text for success; for errors prefer hiding raw JSON if summary is cleaner
  let body = rest
  if (result.error_message) {
    const looksJson = /\{.*"message".*\}/i.test(rest) || /\{.*"error".*\}/i.test(rest)
    if (looksJson && summary) {
      body = ''
    } else if (summary && summary === rest) {
      body = rest
    } else if (!summary) {
      body = rest
    } else if (failedModels.length > 0) {
      body = ''
    }
  }

  const summaryMatchesBody = Boolean(summary && body && summary.trim() === body.trim())

  const detail: ScheduledDiagDetail = {
    upgraded,
    allFailed,
    modelUsed,
    failedCount: failedCount || failedModels.length,
    failedModels,
    summary: summary && !summaryMatchesBody ? summary : summary,
    statusCode,
    errorType,
    body: body || ((upgraded || modelUsed || allFailed || failedModels.length > 0) ? '' : source),
    summaryMatchesBody
  }

  // if only summary and no body, ensure summary shows via body fallback path for plain errors
  if (!detail.upgraded && !detail.allFailed && detail.failedModels.length === 0) {
    if (detail.summary && (detail.statusCode || detail.errorType)) {
      // keep summary + optional empty body
      if (/\{.*"message".*\}/i.test(detail.body) || /\{.*"error".*\}/i.test(detail.body)) {
        detail.body = ''
      }
    } else if (detail.summary) {
      detail.body = detail.summary
      detail.summary = ''
    }
  }

  scheduledDiagDetailCache.set(result as object, detail)
  return detail
}

const parseDiagErrorText = (input: string) => {
  const text = (input || '').trim()
  if (!text) return { summary: '', statusCode: '', errorType: '' }

  let statusCode = ''
  let errorType = ''
  let summary = ''

  const statusMatch =
    text.match(/\bAPI returned\s+([45]\d{2})\b/i) ||
    text.match(/\b([45]\d{2})\b/)
  if (statusMatch) statusCode = statusMatch[1]

  const msgMatch = text.match(/"message"\s*:\s*"((?:\\.|[^"\\])*)"/i)
  if (msgMatch?.[1]) {
    try {
      summary = JSON.parse(`"${msgMatch[1]}"`)
    } catch {
      summary = msgMatch[1]
    }
  }

  const typeMatch = text.match(/"type"\s*:\s*"([^"]+)"/i)
  if (typeMatch) errorType = typeMatch[1]

  if (!summary) {
    // strip common wrappers
    summary = text
      .replace(/^error\(/i, '')
      .replace(/\)$/, '')
      .replace(/\bAPI returned\s+[45]\d{2}:\s*/i, '')
      .replace(/\{[^{}]*"error"[^{}]*\{[\s\S]*\}\s*\}/g, '')
      .replace(/\{[\s\S]*\}/g, '')
      .replace(/\s+/g, ' ')
      .trim()
  }

  if (!summary) {
    if (statusCode && errorType) summary = `${statusCode} ${errorType}`
    else if (statusCode) summary = `HTTP ${statusCode}`
    else summary = text
  }

  if (summary.length > 180) summary = `${summary.slice(0, 180)}…`

  return { summary, statusCode, errorType }
}

</script>
