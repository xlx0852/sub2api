<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.editAccount')"
    variant="drawer"
    width="wide"
    :close-on-click-outside="true"
    @close="handleClose"
  >
    <form
      v-if="account"
      id="edit-account-form"
      novalidate
      @submit.prevent="handleSubmit"
      class="space-y-5"
    >
      <div class="sticky top-0 z-10 -mx-4 -mt-4 mb-1 bg-white px-4 pb-3 pt-3 dark:bg-dark-800 sm:-mx-6 sm:px-6 sm:-mt-4 sm:pt-4" data-testid="edit-account-tabs">
        <div class="modal-tabs-scroll flex gap-2 overflow-x-auto overscroll-x-contain pb-1">
          <button
            v-for="tab in editTabs"
            :key="tab.id"
            type="button"
            class="shrink-0 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
            :class="activeEditTab === tab.id
              ? 'bg-primary-600 text-white shadow-sm'
              : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-200 dark:hover:bg-dark-600'"
            :data-testid="`edit-account-tab-${tab.id}`"
            @click="activeEditTab = tab.id"
          >
            {{ tab.label }}
          </button>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ activeEditTabHint }}</p>
      </div>

      <div v-show="activeEditTab === 'basic'" class="space-y-4" data-testid="edit-tab-basic">
        <div class="grid grid-cols-1 gap-4">
          <div>
            <label class="input-label">{{ t('common.name') }}</label>
            <input v-model="form.name" type="text" class="input" data-tour="edit-account-form-name" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.notes') }}</label>
            <textarea
              v-model="form.notes"
              rows="2"
              class="input min-h-[2.5rem]"
              :placeholder="t('admin.accounts.notesPlaceholder')"
            ></textarea>
            <p class="input-hint">{{ t('admin.accounts.notesHint') }}</p>
          </div>
        </div>
      </div><!-- /edit-tab-basic-name-notes -->

      <!-- API Key fields (only for apikey type) -->
      <div v-if="account.type === 'apikey'" class="space-y-4">
        <div v-show="activeEditTab === 'basic'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
          <input
            v-model="editBaseUrl"
            type="text"
            class="input"
            :placeholder="platformAPIKeyConfig(account.platform).baseUrl"
          />
          <p class="input-hint">{{ baseUrlHint }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
          <input
            v-model="editApiKey"
            type="password"
            class="input font-mono"
            autocomplete="new-password"
            data-1p-ignore
            data-lpignore="true"
            data-bwignore="true"
            :placeholder="platformAPIKeyConfig(account.platform).placeholder"
          />
          <p v-if="mirasimRelayActive" class="input-hint text-amber-600 dark:text-amber-400">
            {{ t('admin.accounts.mirasimRelay.apiKeyOptionalHint') }}
          </p>
          <p v-else class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
        </div>

        </div><!-- /apikey basic fields -->
        <!-- Model Restriction Section (不适用于 Antigravity) -->
        <div v-show="activeEditTab === 'models'" v-if="isFieldVisible('oauthModelRestriction', { platform: account.platform })" class="pt-4">
          <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

          <div
            v-if="isOpenAIModelRestrictionDisabled"
            class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
          >
            <p class="text-xs text-amber-700 dark:text-amber-400">
              {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') }}
            </p>
          </div>

          <template v-else>
            <!-- Mode Toggle -->
            <div class="mb-4 flex gap-2">
              <button
                type="button"
                @click="modelRestrictionMode = 'whitelist'"
                :class="[
                  'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                  modelRestrictionMode === 'whitelist'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                <svg
                  class="mr-1.5 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                {{ t('admin.accounts.modelWhitelist') }}
              </button>
              <button
                type="button"
                @click="modelRestrictionMode = 'mapping'"
                :class="[
                  'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                  modelRestrictionMode === 'mapping'
                    ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                <svg
                  class="mr-1.5 inline h-4 w-4"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                  />
                </svg>
                {{ t('admin.accounts.modelMapping') }}
              </button>
            </div>

            <!-- Whitelist Mode -->
            <div v-if="modelRestrictionMode === 'whitelist'">
              <ModelWhitelistSelector v-model="allowedModels" :platform="account?.platform || 'anthropic'" :account-id="account?.id" />
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
                <span v-if="allowedModels.length === 0 && modelMappings.length === 0">{{
                  t('admin.accounts.supportsAllModels')
                }}</span>
              </p>
            </div>

            <!-- Mapping Mode -->
            <div v-else>
              <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
                <p class="text-xs text-purple-700 dark:text-purple-400">
                  <svg
                    class="mr-1 inline h-4 w-4"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  {{ t('admin.accounts.mapRequestModels') }}
                </p>
              </div>

            <!-- Model Mapping List -->
            <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
              <div
                v-for="(mapping, index) in modelMappings"
                :key="getModelMappingKey(mapping)"
                class="flex items-center gap-2"
              >
                <input
                  v-model="mapping.from"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.requestModel')"
                />
                <svg
                  class="h-4 w-4 flex-shrink-0 text-gray-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M14 5l7 7m0 0l-7 7m7-7H3"
                  />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  @click="removeModelMapping(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                >
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
            </div>

            <button
              type="button"
              @click="addModelMapping"
              class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
            >
              <svg
                class="mr-1 inline h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4v16m8-8H4"
                />
              </svg>
              {{ t('admin.accounts.addMapping') }}
            </button>

              <!-- Quick Add Buttons -->
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="preset in presetMappings"
                  :key="preset.label"
                  type="button"
                  @click="addPresetMapping(preset.from, preset.to)"
                  :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
                >
                  + {{ preset.label }}
                </button>
              </div>
            </div>
          </template>
        </div>

        <!-- Pool Mode Section -->
        <div v-show="activeEditTab === 'routing'" class="pt-4">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.poolMode') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.poolModeHint') }}
              </p>
            </div>
            <button
              type="button"
              @click="poolModeEnabled = !poolModeEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                poolModeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  poolModeEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>
          <div v-if="poolModeEnabled" class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
            <p class="text-xs text-blue-700 dark:text-blue-400">
              <Icon name="exclamationCircle" size="sm" class="mr-1 inline" :stroke-width="2" />
              {{ t('admin.accounts.poolModeInfo') }}
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryCount') }}</label>
            <input
              v-model.number="poolModeRetryCount"
              type="number"
             
             
              step="1"
              class="input w-full sm:w-40"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{
                t('admin.accounts.poolModeRetryCountHint', {
                  default: DEFAULT_POOL_MODE_RETRY_COUNT,
                  max: MAX_POOL_MODE_RETRY_COUNT
                })
              }}
            </p>
          </div>
          <div v-if="poolModeEnabled" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.poolModeRetryStatusCodes') }}</label>
            <input
              v-model="poolModeRetryStatusCodesInput"
              type="text"
              class="input"
              :placeholder="DEFAULT_POOL_MODE_RETRY_STATUS_CODES.join(', ')"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.poolModeRetryStatusCodesHint', { default: DEFAULT_POOL_MODE_RETRY_STATUS_CODES.join(', ') }) }}
            </p>
          </div>
        </div>

        <!-- Custom Error Codes Section -->
        <div v-show="activeEditTab === 'routing'" class="pt-4">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.customErrorCodesHint') }}
              </p>
            </div>
            <button
              type="button"
              @click="customErrorCodesEnabled = !customErrorCodesEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                customErrorCodesEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  customErrorCodesEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="customErrorCodesEnabled" class="space-y-3">
            <div class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
              <p class="text-xs text-amber-700 dark:text-amber-400">
                <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
                {{ t('admin.accounts.customErrorCodesWarning') }}
              </p>
            </div>

            <!-- Error Code Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="code in commonErrorCodes"
                :key="code.value"
                type="button"
                @click="toggleErrorCode(code.value)"
                :class="[
                  'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                  selectedErrorCodes.includes(code.value)
                    ? 'bg-red-100 text-red-700 ring-1 ring-red-500 dark:bg-red-900/30 dark:text-red-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
                ]"
              >
                {{ code.value }} {{ code.label }}
              </button>
            </div>

            <!-- Manual input -->
            <div class="flex items-center gap-2">
              <input
                v-model.number="customErrorCodeInput"
                type="number"
               
               
                class="input flex-1"
                :placeholder="t('admin.accounts.enterErrorCode')"
                @keyup.enter="addCustomErrorCode"
              />
              <button type="button" @click="addCustomErrorCode" class="btn btn-secondary px-3">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M12 4v16m8-8H4"
                  />
                </svg>
              </button>
            </div>

            <!-- Selected codes summary -->
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="code in selectedErrorCodes.sort((a, b) => a - b)"
                :key="code"
                class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2.5 py-0.5 text-sm font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
              >
                {{ code }}
                <button
                  type="button"
                  @click="removeErrorCode(code)"
                  class="hover:text-red-900 dark:hover:text-red-300"
                >
                  <Icon name="x" size="sm" :stroke-width="2" />
                </button>
              </span>
              <span v-if="selectedErrorCodes.length === 0" class="text-xs text-gray-400">
                {{ t('admin.accounts.noneSelectedUsesDefault') }}
              </span>
            </div>
          </div>
        </div>

        <!-- Header Override Section (anthropic/openai apikey only) -->
        <div v-show="activeEditTab === 'routing'"
          v-if="isFieldVisible('headerOverride', { platform: account.platform })"
          class="pt-4"
        >
          <div class="mb-3 flex items-center justify-between">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.headerOverride.title') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.headerOverride.hint') }}
              </p>
            </div>
            <button
              type="button"
              @click="headerOverrideEnabled = !headerOverrideEnabled"
              :class="[
                'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
                headerOverrideEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                  headerOverrideEnabled ? 'translate-x-5' : 'translate-x-0'
                ]"
              />
            </button>
          </div>

          <div v-if="headerOverrideEnabled" class="space-y-3">
            <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
              <p class="text-xs text-blue-700 dark:text-blue-400">
                <Icon name="exclamationCircle" size="sm" class="mr-1 inline" :stroke-width="2" />
                {{ t('admin.accounts.headerOverride.info') }}
              </p>
            </div>

            <div v-if="headerOverrideRows.length > 0" class="space-y-2">
              <div
                v-for="(row, index) in headerOverrideRows"
                :key="getHeaderOverrideRowKey(row)"
                class="flex items-center gap-2"
              >
                <input
                  v-model="row.name"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.headerOverride.namePlaceholder')"
                />
                <input
                  v-model="row.value"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.headerOverride.valuePlaceholder')"
                />
                <button
                  type="button"
                  @click="removeHeaderOverrideRow(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                >
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
            </div>

            <button
              type="button"
              @click="addHeaderOverrideRow"
              class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
            >
              <svg class="mr-1 inline h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4v16m8-8H4"
                />
              </svg>
              {{ t('admin.accounts.headerOverride.addRow') }}
            </button>

            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                @click="fillHeaderOverrideTemplate"
                class="rounded-lg bg-primary-50 px-3 py-1 text-xs text-primary-700 transition-colors hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
              >
                + {{ t('admin.accounts.headerOverride.fillTemplate') }}
              </button>
            </div>

            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.headerOverride.emptyValueHint') }}
            </p>
          </div>
        </div>

      </div>

      <!-- OAuth Model Mapping (OAuth 类型没有 apikey 容器，需要独立的模型映射区域) -->
      <div v-show="activeEditTab === 'models'"
        v-if="supportsOAuthModelRestriction"
        class="pt-4"
        data-testid="edit-oauth-model-restriction"
      >
        <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

        <div
          v-if="isOpenAIModelRestrictionDisabled"
          class="mb-3 rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
        >
          <p class="text-xs text-amber-700 dark:text-amber-400">
            {{ t('admin.accounts.openai.modelRestrictionDisabledByPassthrough') }}
          </p>
        </div>

        <template v-else>
          <!-- Mode Toggle -->
          <div class="mb-4 flex gap-2">
            <button
              type="button"
              @click="modelRestrictionMode = 'whitelist'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'whitelist'
                  ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              {{ t('admin.accounts.modelWhitelist') }}
            </button>
            <button
              type="button"
              @click="modelRestrictionMode = 'mapping'"
              :class="[
                'flex-1 rounded-lg px-4 py-2 text-sm font-medium transition-all',
                modelRestrictionMode === 'mapping'
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
              ]"
            >
              {{ t('admin.accounts.modelMapping') }}
            </button>
          </div>

          <!-- Whitelist Mode -->
          <div v-if="modelRestrictionMode === 'whitelist'">
            <ModelWhitelistSelector v-model="allowedModels" :platform="account?.platform || 'anthropic'" :account-id="account?.id" />
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
              <span v-if="allowedModels.length === 0 && modelMappings.length === 0">{{
                t('admin.accounts.supportsAllModels')
              }}</span>
            </p>
          </div>

          <!-- Mapping Mode -->
          <div v-else>
            <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
              <p class="text-xs text-purple-700 dark:text-purple-400">
                {{ t('admin.accounts.mapRequestModels') }}
              </p>
            </div>

            <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
              <div
                v-for="(mapping, index) in modelMappings"
                :key="'oauth-' + getModelMappingKey(mapping)"
                class="flex items-center gap-2"
              >
                <input
                  v-model="mapping.from"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.requestModel')"
                />
                <svg
                  class="h-4 w-4 flex-shrink-0 text-gray-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M14 5l7 7m0 0l-7 7m7-7H3"
                  />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  class="input flex-1"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  @click="removeModelMapping(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                >
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
            </div>

            <button
              type="button"
              @click="addModelMapping"
              class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
            >
              + {{ t('admin.accounts.addMapping') }}
            </button>

            <!-- Quick Add Buttons -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in presetMappings"
                :key="'oauth-' + preset.label"
                type="button"
                @click="addPresetMapping(preset.from, preset.to)"
                :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
              >
                + {{ preset.label }}
              </button>
            </div>
          </div>
        </template>
      </div>

      <!-- Upstream fields (only for upstream type) -->
      <div v-show="activeEditTab === 'basic'" v-if="account.type === 'upstream'" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.upstream.baseUrl') }}</label>
          <input
            v-model="editBaseUrl"
            type="text"
            class="input"
            placeholder="https://cloudcode-pa.googleapis.com"
          />
          <p class="input-hint">{{ t('admin.accounts.upstream.baseUrlHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.upstream.apiKey') }}</label>
          <input
            v-model="editApiKey"
            type="password"
            class="input font-mono"
            placeholder="sk-..."
          />
          <p class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
        </div>
      </div>



      <div
        v-show="activeEditTab === 'basic'"
        v-if="isFieldVisible('antigravityProjectId', { platform: account.platform }) && account.type === 'oauth'"
        class="pt-4"
        data-testid="edit-antigravity-project-id"
      >
        <label class="input-label">{{ t('admin.accounts.antigravityProjectIdLabel') }}</label>
        <input
          v-model="antigravityProjectId"
          data-testid="antigravity-project-id-input"
          type="text"
          class="input font-mono"
          :placeholder="t('admin.accounts.antigravityProjectIdPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.accounts.antigravityProjectIdHint') }}</p>
      </div>

      <!-- Antigravity model restriction (applies to all antigravity types) -->
      <!-- Antigravity 只支持模型映射模式，不支持白名单模式 -->
      <div v-show="activeEditTab === 'models'" v-if="isFieldVisible('antigravityAdvanced', { platform: account.platform })" class="pt-4">
        <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>

        <!-- Mapping Mode Only (no toggle for Antigravity) -->
        <div>
          <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
            <p class="text-xs text-purple-700 dark:text-purple-400">{{ t('admin.accounts.mapRequestModels') }}</p>
          </div>

          <div class="mb-3 flex flex-wrap gap-2">
            <button
              type="button"
              @click="syncAntigravityUpstreamModels"
              :disabled="isSyncingAntigravityUpstream || !account?.id"
              class="rounded-lg border border-emerald-200 px-3 py-1.5 text-sm text-emerald-600 hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-emerald-800 dark:text-emerald-400 dark:hover:bg-emerald-900/30"
            >
              {{ isSyncingAntigravityUpstream ? t('admin.accounts.syncUpstreamModelsLoading') : t('admin.accounts.syncUpstreamModels') }}
            </button>
          </div>

          <div v-if="antigravityModelMappings.length > 0" class="mb-3 space-y-2">
            <div
              v-for="(mapping, index) in antigravityModelMappings"
              :key="getAntigravityModelMappingKey(mapping)"
              class="space-y-1"
            >
              <div class="flex items-center gap-2">
                <input
                  v-model="mapping.from"
                  type="text"
                  :class="[
                    'input flex-1',
                    !isValidWildcardPattern(mapping.from) ? 'border-red-500 dark:border-red-500' : '',
                    mapping.to.includes('*') ? '' : ''
                  ]"
                  :placeholder="t('admin.accounts.requestModel')"
                />
                <svg class="h-4 w-4 flex-shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
                </svg>
                <input
                  v-model="mapping.to"
                  type="text"
                  :class="[
                    'input flex-1',
                    mapping.to.includes('*') ? 'border-red-500 dark:border-red-500' : ''
                  ]"
                  :placeholder="t('admin.accounts.actualModel')"
                />
                <button
                  type="button"
                  @click="removeAntigravityModelMapping(index)"
                  class="rounded-lg p-2 text-red-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
                >
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
              <!-- 校验错误提示 -->
              <p v-if="!isValidWildcardPattern(mapping.from)" class="text-xs text-red-500">
                {{ t('admin.accounts.wildcardOnlyAtEnd') }}
              </p>
              <p v-if="mapping.to.includes('*')" class="text-xs text-red-500">
                {{ t('admin.accounts.targetNoWildcard') }}
              </p>
            </div>
          </div>

          <button
            type="button"
            @click="addAntigravityModelMapping"
            class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
          >
            <svg class="mr-1 inline h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            {{ t('admin.accounts.addMapping') }}
          </button>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in antigravityPresetMappings"
              :key="preset.label"
              type="button"
              @click="addAntigravityPresetMapping(preset.from, preset.to)"
              :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]"
            >
              + {{ preset.label }}
            </button>
          </div>
        </div>
      </div>

      <!-- Temp Unschedulable Rules -->
      <div v-show="activeEditTab === 'advanced'" class="pt-4 space-y-4">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.tempUnschedulable.title') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.hint') }}
            </p>
          </div>
          <button
            type="button"
            @click="tempUnschedEnabled = !tempUnschedEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              tempUnschedEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                tempUnschedEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>

        <div v-if="tempUnschedEnabled" class="space-y-3">
          <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
            <p class="text-xs text-blue-700 dark:text-blue-400">
              <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
              {{ t('admin.accounts.tempUnschedulable.notice') }}
            </p>
          </div>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in tempUnschedPresets"
              :key="preset.label"
              type="button"
              @click="addTempUnschedRule(preset.rule)"
              class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
            >
              + {{ preset.label }}
            </button>
          </div>

          <div v-if="tempUnschedRules.length > 0" class="space-y-3">
            <div
              v-for="(rule, index) in tempUnschedRules"
              :key="getTempUnschedRuleKey(rule)"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="mb-2 flex items-center justify-between">
                <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.tempUnschedulable.ruleIndex', { index: index + 1 }) }}
                </span>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    :disabled="index === 0"
                    @click="moveTempUnschedRule(index, -1)"
                    class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                  >
                    <Icon name="chevronUp" size="sm" :stroke-width="2" />
                  </button>
                  <button
                    type="button"
                    :disabled="index === tempUnschedRules.length - 1"
                    @click="moveTempUnschedRule(index, 1)"
                    class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                  >
                    <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                    </svg>
                  </button>
                  <button
                    type="button"
                    @click="removeTempUnschedRule(index)"
                    class="rounded p-1 text-red-500 transition-colors hover:text-red-600"
                  >
                    <Icon name="x" size="sm" :stroke-width="2" />
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.errorCode') }}</label>
                  <input
                    v-model.number="rule.error_code"
                    type="number"
                   
                   
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.errorCodePlaceholder')"
                  />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.durationMinutes') }}</label>
                  <input
                    v-model.number="rule.duration_minutes"
                    type="number"
                   
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.durationPlaceholder')"
                  />
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.keywords') }}</label>
                  <input
                    v-model="rule.keywords"
                    type="text"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.keywordsPlaceholder')"
                  />
                  <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.keywordsHint') }}</p>
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.accounts.tempUnschedulable.description') }}</label>
                  <input
                    v-model="rule.description"
                    type="text"
                    class="input"
                    :placeholder="t('admin.accounts.tempUnschedulable.descriptionPlaceholder')"
                  />
                </div>
              </div>
            </div>
          </div>

          <button
            type="button"
            @click="addTempUnschedRule()"
            class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-sm text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
          >
            <svg
              class="mr-1 inline h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            {{ t('admin.accounts.tempUnschedulable.addRule') }}
          </button>
        </div>
      </div>

      <!-- Intercept Warmup Requests (Anthropic/Antigravity) -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('interceptWarmup', { platform: account.platform })"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{
              t('admin.accounts.interceptWarmupRequests')
            }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.interceptWarmupRequestsDesc') }}
            </p>
          </div>
          <button
            type="button"
            @click="interceptWarmupRequests = !interceptWarmupRequests"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              interceptWarmupRequests ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                interceptWarmupRequests ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <div v-show="activeEditTab === 'basic'" class="space-y-4" data-testid="edit-tab-basic-scheduling">
        <div class="grid grid-cols-1 gap-4">
          <div v-if="!isSparkShadow">
            <div class="mb-1 flex items-center gap-2">
              <label class="input-label mb-0">{{ t('admin.accounts.proxy') }}</label>
            </div>
            <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
          </div>
          <div>
            <label class="input-label">{{ t('common.status') }}</label>
            <Select v-model="form.status" :options="statusOptions" />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div>
            <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
            <input v-model.number="form.concurrency" type="number" class="input"
              @input="form.concurrency = Math.max(1, form.concurrency || 1)" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
            <input v-model.number="form.load_factor" type="number"
              class="input" :placeholder="String(form.concurrency || 1)"
              @input="form.load_factor = (form.load_factor &amp;&amp; form.load_factor >= 1) ? form.load_factor : null" />
            <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.priority') }}</label>
            <input
              v-model.number="form.priority"
              type="number"
             
              class="input"
              data-tour="account-form-priority"
            />
            <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
          </div>
        </div>

        <div
          v-if="supportsAccountExpiry"
          class="grid grid-cols-1 gap-4 rounded-lg border border-gray-200 p-3 dark:border-dark-600"
          data-testid="edit-account-expiry-section"
        >
          <div>
            <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
            <input
              v-model="expiresAtInput"
              type="datetime-local"
              class="input"
              disabled
              readonly
              data-testid="edit-account-expires-at"
            />
            <p class="input-hint">{{ t('admin.accounts.expiresAtFollowsCostCycle') }}</p>
          </div>
        </div>
      </div><!-- /edit-tab-basic-scheduling -->

      <!-- OpenAI 自动透传开关（OAuth/API Key） -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && (account.type === 'oauth' || account.type === 'setup-token' || account.type === 'apikey')"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.oauthPassthrough') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.oauthPassthroughDesc') }}
            </p>
          </div>
          <button
            type="button"
            @click="openaiPassthroughEnabled = !openaiPassthroughEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              openaiPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                openaiPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- 外部桥开关：账号由独立部署的 OpenAI 兼容桥服务承载（Devin / Cursor…） -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && account.type === 'apikey'"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div class="pr-4">
            <label class="input-label mb-0">{{ t('admin.accounts.openai.externalBridge') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.externalBridgeDesc') }}
            </p>
          </div>
          <button
            type="button"
            data-testid="external-bridge-toggle"
            role="switch"
            :aria-checked="externalBridgeEnabled"
            :aria-label="t('admin.accounts.openai.externalBridge')"
            @click="externalBridgeEnabled = !externalBridgeEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              externalBridgeEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                externalBridgeEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
        <div v-if="externalBridgeEnabled" class="mt-3 max-w-xs">
          <label class="input-label">{{ t('admin.accounts.openai.externalBridgeKind') }}</label>
          <input
            v-model="externalBridgeKind"
            type="text"
            class="input"
            data-testid="external-bridge-kind"
            aria-required="true"
            :aria-label="t('admin.accounts.openai.externalBridgeKind')"
            :placeholder="t('admin.accounts.openai.externalBridgeKindPlaceholder')"
          />
          <p class="input-hint">{{ t('admin.accounts.openai.externalBridgeKindHint') }}</p>
        </div>
      </div>

      <!-- OpenAI Codex 图片工具统一策略（自动注入 + 客户端显式携带） -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && (account.type === 'oauth' || account.type === 'setup-token' || account.type === 'apikey')"
        class="pt-4"
      >
        <div class="overflow-hidden rounded-lg border border-sky-100 bg-sky-50/60  dark:border-sky-900/50 dark:bg-sky-950/20">
          <div class="flex items-start gap-3 px-4 py-3">
            <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-white text-sky-600  ring-1 ring-sky-100 dark:bg-dark-800 dark:text-sky-300 dark:ring-sky-900/60">
              <Icon name="sparkles" size="sm" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <label class="input-label mb-0">{{ t('admin.accounts.openai.codexImageTool') }}</label>
                <span
                  class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                  :class="codexImageToolBadgeClass"
                >
                  {{ codexImageToolBadgeLabel }}
                </span>
              </div>
              <p class="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">
                {{ t('admin.accounts.openai.codexImageToolDesc') }}
              </p>
            </div>
          </div>
          <div class="border-t border-sky-100 bg-white/70 p-2 dark:border-sky-900/50 dark:bg-dark-800/70">
            <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <button
                v-for="option in codexImageToolOptions"
                :key="option.value"
                type="button"
                :data-testid="`codex-image-tool-${option.value}`"
                @click="codexImageToolMode = option.value"
                :class="[
                  'group flex min-h-[62px] items-start gap-2 rounded-md border px-3 py-2 text-left transition-all',
                  codexImageToolMode === option.value
                    ? option.selectedCardClass
                    : 'border-transparent bg-transparent text-slate-600 hover:border-gray-200 hover:bg-gray-50 dark:text-slate-300 dark:hover:border-dark-500 dark:hover:bg-dark-700'
                ]"
              >
                <span
                  :class="[
                    'mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors',
                    codexImageToolMode === option.value
                      ? option.selectedDotClass
                      : 'border-gray-300 text-transparent group-hover:border-gray-400 dark:border-dark-500'
                  ]"
                >
                  <Icon name="check" size="xs" :stroke-width="2" />
                </span>
                <span class="min-w-0">
                  <span class="block text-sm font-medium">{{ option.label }}</span>
                  <span class="mt-0.5 block text-xs leading-4 text-slate-500 dark:text-slate-400">{{ option.description }}</span>
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- OpenAI WS Mode 三态（off/ctx_pool/passthrough） -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && (account.type === 'oauth' || account.type === 'setup-token' || account.type === 'apikey')"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.wsMode') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.wsModeDesc') }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t(openAIWSModeConcurrencyHintKey) }}
            </p>
          </div>
          <div class="w-52">
            <Select v-model="openaiResponsesWebSocketV2Mode" data-testid="edit-openai-ws-mode-select" :options="openAIWSModeOptions" />
          </div>
        </div>
      </div>

      <!-- OpenAI APIKey Responses API support mode -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && account.type === 'apikey'"
        class="space-y-4 pt-4"
      >
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.responsesMode') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.responsesModeDesc') }}
            </p>
          </div>
          <div class="w-56">
            <Select
              v-model="openAIResponsesMode"
              :options="openAIResponsesModeOptions"
              :disabled="!openAITextGenerationCapabilityEnabled"
              data-testid="openai-responses-mode-select"
            />
          </div>
        </div>
        <div
          v-if="openAITextGenerationCapabilityEnabled"
          class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
        >
          <span class="font-medium">{{ t(openAIResponsesStatusKey) }}</span>
        </div>
        <div
          v-else
          class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
          data-testid="openai-responses-mode-not-applicable"
        >
          {{ t('admin.accounts.openai.responsesModeTextDisabledHint') }}
        </div>
        <div>
          <label class="input-label mb-2 block">{{ t('admin.accounts.openai.endpointCapabilities') }}</label>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <label
              v-for="option in openAIEndpointCapabilityOptions"
              :key="option.value"
              class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
            >
              <input
                type="checkbox"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
                :data-testid="`openai-endpoint-capability-${option.value}`"
                :checked="openAIEndpointCapabilities.includes(option.value)"
                @change="toggleOpenAIEndpointCapability(option.value, $event)"
              />
              <span class="text-gray-700 dark:text-gray-200">{{ option.label }}</span>
            </label>
          </div>
          <p class="input-hint">{{ t('admin.accounts.openai.endpointCapabilitiesDesc') }}</p>
        </div>
      </div>

      <!-- Anthropic API Key 自动透传开关 -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('anthropicAdvanced', { platform: account.platform }) && account.type === 'apikey'"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.apiKeyPassthrough') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.apiKeyPassthroughDesc') }}
            </p>
          </div>
          <button
            type="button"
            @click="anthropicPassthroughEnabled = !anthropicPassthroughEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              anthropicPassthroughEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                anthropicPassthroughEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <div
        v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('anthropicAdvanced', { platform: account.platform }) && account.type === 'apikey'"
        class="pt-4"
        data-testid="edit-anthropic-auth-scheme"
      >
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.apiKeyAuthScheme') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.apiKeyAuthSchemeDesc') }}
            </p>
          </div>
          <select v-model="anthropicAPIKeyAuthScheme" class="input w-52 text-sm">
            <option value="x_api_key">{{ t('admin.accounts.anthropic.apiKeyAuthSchemeXApiKey') }}</option>
            <option value="authorization_bearer">{{ t('admin.accounts.anthropic.apiKeyAuthSchemeBearer') }}</option>
          </select>
        </div>
      </div>

      <!-- Anthropic API Key: Mirasim Relay（第三方中继上游，走 mirasim 签名协议） -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('anthropicAdvanced', { platform: account.platform }) && account.type === 'apikey'"
        class="pt-4"
        data-testid="edit-mirasim-relay-section"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.mirasimRelay.enabled') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.mirasimRelay.enabledDesc') }}
            </p>
          </div>
          <button
            type="button"
            data-testid="edit-mirasim-relay-toggle"
            @click="mirasimRelayEnabled = !mirasimRelayEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              mirasimRelayEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                mirasimRelayEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>

        <div
          v-if="mirasimRelayEnabled"
          class="mt-4 space-y-4 border-l-2 border-gray-200 pl-4 dark:border-dark-600"
        >
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.mirasimRelay.enabledHint') }}
          </p>

          <!--
            空映射风险提示 + 一键填充。
            关键约束（改动前请先读）：mirasimModelMappingMissing 是后端派生只读字段，
            为 true 表示该账号 model_mapping 为空，调度侧等价「认领所有模型」——
            上游不支持的模型也会派给本账号，上游 422 → 网关 502。
            修复必须走专用端点（服务端会保全凭据并对非空映射要求 overwrite），
            不要用本表单的普通保存绕过。
          -->
          <div
            v-if="mirasimModelMappingMissing"
            class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20"
            data-testid="edit-mirasim-mapping-missing"
          >
            <p class="text-xs font-medium text-amber-700 dark:text-amber-400">
              {{ t('admin.accounts.mirasimRelay.mappingMissingTitle') }}
            </p>
            <p class="mt-1 text-xs text-amber-700 dark:text-amber-400">
              {{ t('admin.accounts.mirasimRelay.mappingMissingDesc') }}
            </p>
            <button
              type="button"
              class="mt-2 inline-flex items-center rounded-lg bg-amber-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-amber-700 disabled:cursor-not-allowed disabled:opacity-60"
              data-testid="edit-mirasim-apply-mapping"
              :disabled="isApplyingMirasimMapping || !account?.id"
              @click="applyMirasimModelMappingFromUpstream"
            >
              {{
                isApplyingMirasimMapping
                  ? t('admin.accounts.mirasimRelay.applyMappingLoading')
                  : t('admin.accounts.mirasimRelay.applyMapping')
              }}
            </button>
          </div>

          <MirasimOAuthOnboarding
            testid-prefix="edit-mirasim-oauth"
            :auth-url="mirasimAuthUrl"
            :proxy-id="form.proxy_id"
            @apply="handleMirasimOAuthCredential"
          />

          <div>
            <label class="input-label">{{ t('admin.accounts.mirasimRelay.refreshToken') }}</label>
            <input
              v-model="mirasimRefreshToken"
              type="password"
              class="input font-mono"
              data-testid="edit-mirasim-refresh-token"
              autocomplete="new-password"
              data-1p-ignore
              data-lpignore="true"
              data-bwignore="true"
              :placeholder="t('admin.accounts.mirasimRelay.refreshTokenPlaceholder')"
            />
            <p class="input-hint">{{ t('admin.accounts.mirasimRelay.refreshTokenHint') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.accounts.mirasimRelay.deviceKey') }}</label>
            <textarea
              v-model="mirasimDeviceKey"
              rows="3"
              class="input font-mono text-xs"
              data-testid="edit-mirasim-device-key"
              :placeholder="t('admin.accounts.mirasimRelay.deviceKeyPlaceholder')"
            />
            <p class="input-hint">{{ t('admin.accounts.mirasimRelay.deviceKeyHint') }}</p>
          </div>

          <!-- 可选覆盖：留空即使用内置默认值 -->
          <div>
            <button
              type="button"
              data-testid="edit-mirasim-overrides-toggle"
              @click="showMirasimOverrides = !showMirasimOverrides"
              class="flex items-center gap-2 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200"
            >
              <svg
                :class="['h-4 w-4 transition-transform', showMirasimOverrides ? 'rotate-90' : '']"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
              </svg>
              <span>
                {{
                  showMirasimOverrides
                    ? t('admin.accounts.mirasimRelay.hideOverrides')
                    : t('admin.accounts.mirasimRelay.showOverrides')
                }}
              </span>
            </button>
          </div>

          <div v-if="showMirasimOverrides" class="space-y-4">
            <div>
              <label class="input-label">{{ t('admin.accounts.mirasimRelay.relayUrl') }}</label>
              <input
                v-model="mirasimRelayUrl"
                type="text"
                class="input font-mono"
                data-testid="edit-mirasim-relay-url"
                :placeholder="MIRASIM_DEFAULT_RELAY_URL"
              />
              <p class="input-hint">{{ t('admin.accounts.mirasimRelay.relayUrlHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.mirasimRelay.authUrl') }}</label>
              <input
                v-model="mirasimAuthUrl"
                type="text"
                class="input font-mono"
                :placeholder="MIRASIM_DEFAULT_AUTH_URL"
              />
              <p class="input-hint">{{ t('admin.accounts.mirasimRelay.authUrlHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.mirasimRelay.clientVersion') }}</label>
              <input
                v-model="mirasimClientVersion"
                type="text"
                class="input font-mono"
                :placeholder="MIRASIM_DEFAULT_CLIENT_VERSION"
              />
              <p class="input-hint">{{ t('admin.accounts.mirasimRelay.clientVersionHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.mirasimRelay.sealPubkey') }}</label>
              <input
                v-model="mirasimSealPubkey"
                type="text"
                class="input font-mono text-xs"
                :placeholder="MIRASIM_DEFAULT_SEAL_PUBKEY"
              />
              <p class="input-hint">{{ t('admin.accounts.mirasimRelay.sealPubkeyHint') }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Anthropic API Key: Web Search Emulation (hidden when global disabled) -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('anthropicAdvanced', { platform: account.platform }) && account.type === 'apikey' && webSearchGlobalEnabled"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.anthropic.webSearchEmulation') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.anthropic.webSearchEmulationDesc') }}
            </p>
          </div>
          <select v-model="webSearchEmulationMode" class="input w-24 text-sm">
            <option value="default">{{ t('admin.accounts.anthropic.webSearchDefault') }}</option>
            <option value="enabled">{{ t('admin.accounts.anthropic.webSearchEnabled') }}</option>
            <option value="disabled">{{ t('admin.accounts.anthropic.webSearchDisabled') }}</option>
          </select>
        </div>
      </div>

      <!-- 配额控制 (API key 账号) -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account?.type === 'apikey'"
        class="pt-4 space-y-4"
      >
        <div class="mb-3">
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaLimitHint') }}
          </p>
        </div>
        <QuotaLimitCard
          :totalLimit="editQuotaLimit"
          :dailyLimit="editQuotaDailyLimit"
          :weeklyLimit="editQuotaWeeklyLimit"
          :dailyResetMode="editDailyResetMode"
          :dailyResetHour="editDailyResetHour"
          :weeklyResetMode="editWeeklyResetMode"
          :weeklyResetDay="editWeeklyResetDay"
          :weeklyResetHour="editWeeklyResetHour"
          :resetTimezone="editResetTimezone"
          :quotaNotifyGlobalEnabled="quotaNotifyGlobalEnabled"
          :quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled"
          :quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold"
          :quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType"
          :quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled"
          :quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold"
          :quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType"
          :quotaNotifyTotalEnabled="quotaNotifyState.total.enabled"
          :quotaNotifyTotalThreshold="quotaNotifyState.total.threshold"
          :quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType"
          @update:totalLimit="editQuotaLimit = $event"
          @update:dailyLimit="editQuotaDailyLimit = $event"
          @update:weeklyLimit="editQuotaWeeklyLimit = $event"
          @update:dailyResetMode="editDailyResetMode = $event"
          @update:dailyResetHour="editDailyResetHour = $event"
          @update:weeklyResetMode="editWeeklyResetMode = $event"
          @update:weeklyResetDay="editWeeklyResetDay = $event"
          @update:weeklyResetHour="editWeeklyResetHour = $event"
          @update:resetTimezone="editResetTimezone = $event"
          @update:quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled = $event"
          @update:quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold = $event"
          @update:quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType = $event"
          @update:quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled = $event"
          @update:quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold = $event"
          @update:quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType = $event"
          @update:quotaNotifyTotalEnabled="quotaNotifyState.total.enabled = $event"
          @update:quotaNotifyTotalThreshold="quotaNotifyState.total.threshold = $event"
          @update:quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType = $event"
        />
      </div>

      <!-- OpenAI OAuth Codex 官方客户端限制开关 -->
      <div v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && (account.type === 'oauth' || account.type === 'setup-token')"
        class="pt-4"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnly') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.codexCLIOnlyDesc') }}
            </p>
          </div>
          <button
            type="button"
            @click="codexCLIOnlyEnabled = !codexCLIOnlyEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              codexCLIOnlyEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                codexCLIOnlyEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
        <div
          v-if="codexCLIOnlyEnabled"
          class="mt-4 flex items-center justify-between border-l-2 border-gray-200 pl-4 dark:border-dark-600"
        >
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.codexCLIOnlyAppServer') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.codexCLIOnlyAppServerDesc') }}
            </p>
          </div>
          <button
            type="button"
            @click="codexCLIOnlyAppServerEnabled = !codexCLIOnlyAppServerEnabled"
            :class="[
              'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              codexCLIOnlyAppServerEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                codexCLIOnlyAppServerEnabled ? 'translate-x-5' : 'translate-x-0'
              ]"
            />
          </button>
        </div>
      </div>

      <!-- Codex 指纹收敛模式（仅 OpenAI OAuth） -->
      <div
        v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && account.type === 'oauth'"
        class="pt-4"
      >
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <label class="input-label mb-0">{{ t('admin.accounts.openai.codexFingerprintMode') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.codexFingerprintModeDesc') }}
            </p>
          </div>
          <div class="w-52 flex-shrink-0">
            <Select v-model="codexFingerprintMode" data-testid="edit-codex-fingerprint-mode-select" :options="codexFingerprintModeOptions" />
          </div>
        </div>
      </div>

      <div
        v-show="activeEditTab === 'models'"
        v-if="account && isFieldVisible('openAIAdvanced', { platform: account.platform }) && (account.type === 'oauth' || account.type === 'setup-token' || account.type === 'apikey')"
        class="pt-4 space-y-4"
        data-testid="edit-openai-compact-mode"
      >
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.openai.compactMode') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openai.compactModeDesc') }}
            </p>
          </div>
          <div class="w-44">
            <Select v-model="openAICompactMode" :options="openAICompactModeOptions" />
          </div>
        </div>
        <div class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
          <span class="font-medium">{{ t(openAICompactStatusKey) }}</span>
          <span
            v-if="account?.extra?.openai_compact_checked_at"
            class="ml-2 text-gray-500 dark:text-gray-400"
          >
            {{ t('admin.accounts.openai.compactLastChecked') }}:
            {{ formatDateTime(new Date(String(account.extra.openai_compact_checked_at))) }}
          </span>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.openai.compactModelMapping') }}</label>
          <p class="input-hint">{{ t('admin.accounts.openai.compactModelMappingDesc') }}</p>
          <div v-if="openAICompactModelMappings.length > 0" class="mb-3 space-y-2">
            <div
              v-for="(mapping, index) in openAICompactModelMappings"
              :key="getOpenAICompactModelMappingKey(mapping)"
              class="flex items-center gap-2"
            >
              <input
                v-model="mapping.from"
                type="text"
                class="input flex-1"
                :placeholder="t('admin.accounts.fromModel')"
              />
              <span class="text-gray-400">→</span>
              <input
                v-model="mapping.to"
                type="text"
                class="input flex-1"
                :placeholder="t('admin.accounts.toModel')"
              />
              <button type="button" @click="removeOpenAICompactModelMapping(index)" class="text-red-500 hover:text-red-700">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
          <button type="button" @click="addOpenAICompactModelMapping" class="btn btn-secondary text-sm">
            + {{ t('admin.accounts.addMapping') }}
          </button>
        </div>
      </div>



      <div v-show="activeEditTab === 'basic'" class="space-y-4" data-testid="edit-tab-basic-status">
        <div v-if="account && isFieldVisible('antigravityAdvanced', { platform: account.platform })" class="flex items-center gap-2">
          <label class="flex cursor-pointer items-center gap-2">
            <input
              type="checkbox"
              v-model="allowOverages"
              class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            />
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.accounts.allowOverages') }}
            </span>
          </label>
          <div class="group relative">
            <span
              class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
            >
              ?
            </span>
            <div
              class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
            >
              {{ t('admin.accounts.allowOveragesTooltip') }}
              <div
                class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700"
              ></div>
            </div>
          </div>
        </div>

		<GroupSelector
			v-model="form.group_ids"
          :groups="groups"
          :platform="account?.platform"
          :mixed-scheduling="mixedScheduling"
          :columns="1"
          data-testid="account-form-groups"
          data-tour="account-form-groups"
        />
      </div>

      <!-- Mixed Scheduling (only for antigravity accounts, read-only in edit mode) -->
      <div
        v-show="activeEditTab === 'advanced'"
        v-if="account && isFieldVisible('mixedScheduling', { platform: account.platform })"
        class="flex items-center gap-2 pt-4"
      >
        <label class="flex cursor-not-allowed items-center gap-2 opacity-60">
          <input
            type="checkbox"
            v-model="mixedScheduling"
            disabled
            class="h-4 w-4 cursor-not-allowed rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
          />
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.mixedScheduling') }}
          </span>
        </label>
        <div class="group relative">
          <span
            class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full bg-gray-200 text-xs text-gray-500 hover:bg-gray-300 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500"
          >
            ?
          </span>
          <div
            class="pointer-events-none absolute left-0 top-full z-[100] mt-1.5 w-72 rounded bg-gray-900 px-3 py-2 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100 dark:bg-gray-700"
          >
            {{ t('admin.accounts.mixedSchedulingTooltip') }}
            <div
              class="absolute bottom-full left-3 border-4 border-transparent border-b-gray-900 dark:border-b-gray-700"
            ></div>
          </div>
        </div>
      </div>

      <div v-show="activeEditTab === 'cost'" class="space-y-4" data-testid="edit-account-cost-section">
        <div>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.configure') }}</h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
            {{ (account.type === 'oauth' || account.type === 'setup-token') ? t('admin.profit.costTypeAutoSubscription') : t('admin.profit.costTypeAutoMetered') }}
          </p>
        </div>
        <div class="max-w-xs">
          <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
          <input
            v-model.number="form.rate_multiplier"
            type="number"
           
            step="0.001"
            class="input disabled:cursor-not-allowed disabled:opacity-60"
            data-testid="edit-account-rate-multiplier"
          />
		  <p class="input-hint">{{ t('admin.accounts.rateMultiplierCostHint') }}</p>
        </div>

        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="account-cost-preview">
          <p class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.accounts.costPreview.title') }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.costPreview.hint') }}
          </p>
          <div class="mt-3 grid gap-3 sm:grid-cols-2">
            <label class="text-xs text-gray-600 dark:text-gray-300">
              {{ t('admin.accounts.costPreview.basePrice') }}
              <input v-model.number="costPreview.basePrice" type="number" min="0" step="0.01" class="input mt-1" />
            </label>
            <label class="text-xs text-gray-600 dark:text-gray-300">
              {{ t('admin.accounts.costPreview.salesMultiplier') }}
              <input v-model.number="costPreview.salesMultiplier" type="number" min="0" step="0.01" class="input mt-1" />
            </label>
          </div>
          <div class="mt-3 grid grid-cols-3 gap-2 text-xs">
            <div class="rounded bg-gray-50 p-2 dark:bg-dark-700">
              <span class="block text-gray-500">{{ t('admin.accounts.costPreview.userCharge') }}</span>
              <strong class="mt-1 block tabular-nums">{{ formatPreviewNumber(costPreviewUserCharge) }}</strong>
            </div>
            <div class="rounded bg-gray-50 p-2 dark:bg-dark-700">
              <span class="block text-gray-500">{{ t('admin.accounts.costPreview.accountCost') }}</span>
              <strong class="mt-1 block tabular-nums">{{ formatPreviewNumber(costPreviewAccountCost) }}</strong>
            </div>
            <div class="rounded bg-gray-50 p-2 dark:bg-dark-700">
              <span class="block text-gray-500">{{ t('admin.accounts.costPreview.grossMargin') }}</span>
              <strong class="mt-1 block tabular-nums" :class="costPreviewGrossMargin < 0 ? 'text-red-600' : 'text-emerald-600'">
                {{ formatPreviewNumber(costPreviewGrossMargin) }}
              </strong>
            </div>
          </div>
        </div>

        <!-- 上游网关计费倍率探测（仅 API Key 账号） -->
        <div v-if="account?.type === 'apikey'" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="edit-account-upstream-billing">
          <div class="flex items-center justify-between gap-4">
            <div>
              <label class="input-label mb-0">{{ t('admin.accounts.upstreamBilling.autoProbe') }}</label>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.upstreamBilling.autoProbeHint') }}
              </p>
            </div>
            <Toggle
              :model-value="upstreamBillingAutoProbeEnabled"
              data-testid="upstream-billing-auto-probe"
              :aria-label="t('admin.accounts.upstreamBilling.autoProbe')"
              @update:model-value="handleUpstreamBillingAutoProbeChange"
            />
          </div>
        </div>
        <AccountCostConfigPanel
          v-if="account.type === 'oauth' || account.type === 'setup-token'"
          :active="show && !!account && activeEditTab === 'cost'"
          :account-id="account.id"
          :account-name="account.name"
          :account-type="account.type"
          :account-platform="account.platform"
          :show-account-header="false"
          :show-inline-save="true"
          @saved="handleCostConfigSaved"
        />
        <div v-else class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-700" data-testid="edit-account-cost-metered">
          <div class="font-medium text-gray-800 dark:text-dark-100">{{ t('admin.profit.metered') }}</div>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.costTypeAutoMetered') }}</p>
        </div>
      </div>

    </form>

    <template #footer>
      <div v-if="account" class="flex w-full items-center justify-between gap-3">
        <p class="text-xs text-gray-500 dark:text-dark-400">
          <span v-if="activeEditTab === 'cost'">{{ t('admin.accounts.editTabs.costFooterHint') }}</span>
        </p>
        <div class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="edit-account-form"
          :disabled="submitting"
          class="btn btn-primary"
          data-testid="account-form-submit"
          data-tour="account-form-submit"
        >
          <svg
            v-if="submitting"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{ submitting ? t('admin.accounts.updating') : t('common.update') }}
        </button>
        </div>
      </div>
    </template>
  </BaseDialog>

  <!-- Mixed Channel Warning Dialog -->
  <ConfirmDialog
    :show="showMixedChannelWarning"
    :title="t('admin.accounts.mixedChannelWarningTitle')"
    :message="mixedChannelWarningMessageText"
    :confirm-text="t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleMixedChannelConfirm"
    @cancel="handleMixedChannelCancel"
  />
</template>

<style scoped>
.modal-tabs-scroll {
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
}

.modal-tabs-scroll::-webkit-scrollbar {
  display: none;
}
</style>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { mergeModelMappingChanges } from '@/utils/modelMappingMerge'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { useQuotaNotifyState } from '@/composables/useQuotaNotifyState'
import type {
  Account,
  Proxy,
  AdminGroup,
  CheckMixedChannelResponse,
  OpenAICompactMode,
  OpenAIResponsesMode,
  OpenAIEndpointCapability
} from '@/types'
import BaseDialog from '@/ui/BaseDialog.vue'
import Toggle from '@/ui/Toggle.vue'
import AccountCostConfigPanel from '@/components/admin/account/AccountCostConfigPanel.vue'
import ConfirmDialog from '@/ui/ConfirmDialog.vue'
import Select from '@/ui/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import MirasimOAuthOnboarding from './MirasimOAuthOnboarding.vue'
import type { MirasimOAuthIdentity } from '@/api/admin/mirasim'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import QuotaLimitCard from '@/components/account/QuotaLimitCard.vue'
import {
  applyAntigravityProjectID,
  applyAntigravityExtra,
  applyOpenAIPassthrough,
  applyOpenAIExternalBridge,
  applyOpenAICompactMode,
  applyOpenAIWebSocketExtra,
  applyOpenAIEndpointCapabilities,
  normalizeOpenAIEndpointCapabilities,
  applyAnthropicExtra,
  applyMirasimRelay,
  validateMirasimRelayInput,
  mirasimRelayInputFromAccount,
  applyGrokAPIKeyRouting,
  applyHeaderOverride,
  applyInterceptWarmup,
  getHeaderOverrideTemplate,
  isFieldVisible,
  splitHeaderOverridesObject,
  validateHeaderOverrideRows,
  HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY,
  HEADER_OVERRIDES_CREDENTIAL_KEY,
  MIRASIM_DEFAULT_AUTH_URL,
  MIRASIM_DEFAULT_CLIENT_VERSION,
  MIRASIM_DEFAULT_RELAY_URL,
  MIRASIM_DEFAULT_SEAL_PUBKEY,
  type HeaderOverrideRow,
  type MirasimRelayInput
} from '@/features/admin-accounts/form/credentialsBuilder'
import { platformAPIKeyConfig } from '@/features/admin-accounts/form/registry'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  OPENAI_WS_MODE_HTTP_BRIDGE,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode,
  resolveOpenAIWSModeFromExtra
} from '@/utils/openaiWsMode'
import {
  getPresetMappingsByPlatform,
  commonErrorCodes,
  buildModelMappingObject,
  splitModelMappingObject,
  isValidWildcardPattern,
  useModelRestrictionEditor,
  type ModelMappingEntry
} from '@/composables/useModelWhitelist'

interface Props {
  show: boolean
  account: Account | null
  proxies: Proxy[]
  groups: AdminGroup[]
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  updated: [account: Account]
}>()

const { t } = useI18n()

type EditAccountTab = 'basic' | 'models' | 'routing' | 'advanced' | 'cost'
const activeEditTab = ref<EditAccountTab>('basic')
// 池化/自定义错误码/请求头覆写仅 API key 有配置面；OAuth 打开「路由策略」会是空白页。
const supportsRoutingTab = computed(() => {
  const type = props.account?.type
  return type === 'apikey'
})
const editTabs = computed(() => {
  const tabs: { id: EditAccountTab; label: string }[] = [
    { id: 'basic', label: t('admin.accounts.editTabs.basic') },
    { id: 'models', label: t('admin.accounts.editTabs.models') },
  ]
  if (supportsRoutingTab.value) {
    tabs.push({ id: 'routing', label: t('admin.accounts.editTabs.routing') })
  }
  tabs.push(
    { id: 'advanced', label: t('admin.accounts.editTabs.advanced') },
    { id: 'cost', label: t('admin.accounts.editTabs.cost') },
  )
  return tabs
})
const activeEditTabHint = computed(() => t(`admin.accounts.editTabs.${activeEditTab.value}Hint` as const))

watch(
  () => [props.show, props.account?.id, supportsRoutingTab.value] as const,
  () => {
    if (activeEditTab.value === 'routing' && !supportsRoutingTab.value) {
      activeEditTab.value = 'basic'
    }
  },
)

const appStore = useAppStore()

// Spark 影子账号(parent_account_id 非空):代理恒继承母账号,不可独立编辑(外审 B/P1),
// 故隐藏代理选择器。
const isSparkShadow = computed(() => props.account?.parent_account_id != null)
const isSubscriptionAccount = computed(
  () => props.account?.type === 'oauth' || props.account?.type === 'setup-token'
)
// API Key 按量模式无账号调度过期概念；仅订阅号（成本周期）展示过期。
const supportsAccountExpiry = computed(() => isSubscriptionAccount.value)
// OAuth/setup-token 通用模型限制（Antigravity 使用独立映射 UI）
const supportsOAuthModelRestriction = computed(() => {
  const platform = props.account?.platform
  const type = props.account?.type
  if (!platform || (type !== 'oauth' && type !== 'setup-token')) return false
  return isFieldVisible('oauthModelRestriction', { platform })
})

// Platform-specific hint for Base URL
const baseUrlHint = computed(() => {
  if (!props.account) return t('admin.accounts.baseUrlHint')
  return t(platformAPIKeyConfig(props.account.platform).baseUrlHintKey)
})

const antigravityPresetMappings = computed(() => getPresetMappingsByPlatform('antigravity'))

// Model mapping type

interface TempUnschedRuleForm {
  error_code: number | null
  keywords: string
  duration_minutes: number | null
  description: string
}

// State
const submitting = ref(false)
const editBaseUrl = ref('https://api.anthropic.com')
const editApiKey = ref('')
const {
  modelMappings,
  modelRestrictionMode,
  allowedModels,
  addModelMapping,
  removeModelMapping,
  addPresetMapping: addModelPresetMapping,
  resetModelRestriction
} = useModelRestrictionEditor()
const originalModelMapping = ref<Record<string, string>>({})
const openAICompactModelMappings = ref<ModelMappingEntry[]>([])
const DEFAULT_POOL_MODE_RETRY_COUNT = 3
const MAX_POOL_MODE_RETRY_COUNT = 10
const DEFAULT_POOL_MODE_RETRY_STATUS_CODES = [401, 403, 429]
const poolModeEnabled = ref(false)
const poolModeRetryCount = ref(DEFAULT_POOL_MODE_RETRY_COUNT)
const poolModeRetryStatusCodesInput = ref('')

function parsePoolModeRetryStatusCodes(input: string): number[] {
  if (!input || !input.trim()) return []
  const seen = new Set<number>()
  const out: number[] = []
  for (const token of input.split(/[,\s]+/)) {
    const trimmed = token.trim()
    if (!trimmed) continue
    const n = Number(trimmed)
    if (!Number.isFinite(n) || !Number.isInteger(n)) continue
    if (n < 100 || n > 599) continue
    if (seen.has(n)) continue
    seen.add(n)
    out.push(n)
  }
  return out.sort((a, b) => a - b)
}

function formatPoolModeRetryStatusCodes(value: unknown): string {
  if (!Array.isArray(value)) return ''
  const out: number[] = []
  const seen = new Set<number>()
  for (const v of value) {
    const n = typeof v === 'string' ? Number(v.trim()) : Number(v)
    if (!Number.isFinite(n) || !Number.isInteger(n)) continue
    if (n < 100 || n > 599) continue
    if (seen.has(n)) continue
    seen.add(n)
    out.push(n)
  }
  return out.sort((a, b) => a - b).join(', ')
}
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const headerOverrideEnabled = ref(false)
const headerOverrideRows = ref<HeaderOverrideRow[]>([])

const addHeaderOverrideRow = () => {
  headerOverrideRows.value.push({ name: '', value: '' })
}

const removeHeaderOverrideRow = (index: number) => {
  headerOverrideRows.value.splice(index, 1)
}

// 模板按钮：填入标准客户端请求头名称（值留空），跳过已存在的同名行
const fillHeaderOverrideTemplate = () => {
  const existing = new Set(
    headerOverrideRows.value.map((row) => row.name.trim().toLowerCase()).filter(Boolean)
  )
  const rows = headerOverrideRows.value.filter((row) => row.name.trim() || row.value.trim())
  for (const row of getHeaderOverrideTemplate(props.account?.platform || '')) {
    if (!existing.has(row.name)) {
      rows.push(row)
    }
  }
  headerOverrideRows.value = rows
}
const interceptWarmupRequests = ref(false)
const mixedScheduling = ref(false) // For antigravity accounts: enable mixed scheduling
const allowOverages = ref(false) // For antigravity accounts: enable AI Credits overages
const antigravityProjectId = ref('')
const antigravityModelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const antigravityWhitelistModels = ref<string[]>([])
const antigravityModelMappings = ref<ModelMappingEntry[]>([])
const isSyncingAntigravityUpstream = ref(false)
const tempUnschedEnabled = ref(false)
const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
const getModelMappingKey = createStableObjectKeyResolver<ModelMappingEntry>('edit-model-mapping')
const getHeaderOverrideRowKey = createStableObjectKeyResolver<HeaderOverrideRow>('edit-header-override-row')
const getOpenAICompactModelMappingKey = createStableObjectKeyResolver<ModelMappingEntry>('edit-openai-compact-model-mapping')
const getAntigravityModelMappingKey = createStableObjectKeyResolver<ModelMappingEntry>('edit-antigravity-model-mapping')
const getTempUnschedRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('edit-temp-unsched-rule')

const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{ groupName: string; currentPlatform: string; otherPlatform: string } | null>(
  null
)
const mixedChannelWarningRawMessage = ref('')
const mixedChannelWarningAction = ref<(() => Promise<void>) | null>(null)
const antigravityMixedChannelConfirmed = ref(false)

// OpenAI 自动透传开关（OAuth/API Key）
const openaiPassthroughEnabled = ref(false)
const externalBridgeEnabled = ref(false)
const externalBridgeKind = ref('')
const openAICompactMode = ref<OpenAICompactMode>('auto')
const openAIResponsesMode = ref<OpenAIResponsesMode>('auto')
const openAIEndpointCapabilities = ref<OpenAIEndpointCapability[]>(['chat_completions', 'embeddings'])
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexCLIOnlyEnabled = ref(false)
const codexCLIOnlyAppServerEnabled = ref(false)
type CodexFingerprintMode = 'off' | 'device' | 'session' | 'full'
const codexFingerprintMode = ref<CodexFingerprintMode>('off')
type CodexImageToolMode = 'inherit' | 'enabled' | 'disabled' | 'block'
const codexImageToolMode = ref<CodexImageToolMode>('inherit')
type AnthropicAPIKeyAuthScheme = 'x_api_key' | 'authorization_bearer'
const anthropicPassthroughEnabled = ref(false)
const anthropicAPIKeyAuthScheme = ref<AnthropicAPIKeyAuthScheme>('x_api_key')
const webSearchEmulationMode = ref('default')

// Mirasim relay：Anthropic API Key 账号的第三方中继上游（走 mirasim 签名协议）。
// 关键约束（改动前请先读）：
//   - 后端识别规则（isMirasimRelayAccount）是 platform=anthropic && type=apikey &&
//     credentials.mirasim_refresh_token 非空，且 extra.mirasim_relay_enabled 未显式置 false。
//   - refresh token / device key 是敏感键，后端不回显；本表单留空 = 保留原值，
//     靠 credentials_status.has_mirasim_refresh_token 判断是否已有值。
//   - 写入统一交给 applyMirasimRelay，它同时负责 base_url 回退与 api_key 清理。
//   - 关闭时必须写显式 false（edit 模式），否则凭据仍在会被后端重新识别为 mirasim。
const mirasimRelayEnabled = ref(false)
const mirasimRefreshToken = ref('')
const mirasimDeviceKey = ref('')
const mirasimRelayUrl = ref('')
const mirasimAuthUrl = ref('')
const mirasimClientVersion = ref('')
const mirasimSealPubkey = ref('')
const showMirasimOverrides = ref(false)
const mirasimRelayInitiallyEnabled = ref(false)
// Mirasim 授权登录回填：覆写 refresh token 输入框（留空即保留原值的语义由用户可见的输入决定）。
function handleMirasimOAuthCredential(payload: {
  refresh_token: string
  identity?: MirasimOAuthIdentity
}) {
  mirasimRefreshToken.value = payload.refresh_token
  if (!mirasimRelayEnabled.value) mirasimRelayEnabled.value = true
}

const mirasimRelayInput = computed<MirasimRelayInput>(() => ({
  enabled: mirasimRelayEnabled.value,
  refreshToken: mirasimRefreshToken.value,
  deviceKey: mirasimDeviceKey.value,
  relayUrl: mirasimRelayUrl.value,
  authUrl: mirasimAuthUrl.value,
  clientVersion: mirasimClientVersion.value,
  sealPubkey: mirasimSealPubkey.value
}))
const mirasimRelayActive = computed(
  () =>
    props.account?.platform === 'anthropic' &&
    props.account?.type === 'apikey' &&
    mirasimRelayEnabled.value
)
const hasExistingMirasimRefreshToken = computed(
  () => props.account?.credentials_status?.has_mirasim_refresh_token === true
)
const resetMirasimRelayForm = () => {
  mirasimRelayEnabled.value = false
  mirasimRefreshToken.value = ''
  mirasimDeviceKey.value = ''
  mirasimRelayUrl.value = ''
  mirasimAuthUrl.value = ''
  mirasimClientVersion.value = ''
  mirasimSealPubkey.value = ''
  showMirasimOverrides.value = false
  mirasimRelayInitiallyEnabled.value = false
}

// 后端派生只读字段：mirasim relay 账号的 model_mapping 为空。
// 只有该标记为 true 时才提示风险——非 relay 账号的空映射是正常配置。
const mirasimModelMappingMissing = computed(
  () => props.account?.mirasim_model_mapping_missing === true
)
const isApplyingMirasimMapping = ref(false)

// 一键以「上游可用模型清单」填充 identity 映射。
//
// 关键约束（改动前请先读）：
//   - 服务端直接落库，本操作**不需要**再点保存；回填表单白名单只是保持表单与线上一致，
//     避免用户随后保存时把手工别名映射又写回去。
//   - overwrite 由本地是否已有映射决定：本地非空（用户已手工填了别名/白名单）时
//     才请求覆盖；服务端已有映射但本地为空时服务端会返回 409，透传错误提示即可。
//   - changed=false 是幂等成功（服务端逐键相同），不是失败，用 info 提示。
async function applyMirasimModelMappingFromUpstream() {
  const accountId = props.account?.id
  if (!accountId || isApplyingMirasimMapping.value) return

  isApplyingMirasimMapping.value = true
  try {
    const hasLocalMapping = allowedModels.value.length > 0 || modelMappings.value.length > 0
    const result = await adminAPI.accounts.applyMirasimModelMapping(accountId, hasLocalMapping)
    const models = (result.models || []).map((model) => model.trim()).filter(Boolean)
    if (models.length === 0) {
      appStore.showInfo(t('admin.accounts.mirasimRelay.applyMappingEmpty'))
      return
    }

    // 服务端写入的是 identity 映射（from === to），表单状态同步为白名单模式最贴近语义。
    modelRestrictionMode.value = 'whitelist'
    allowedModels.value = models
    modelMappings.value = []

    if (result.changed) {
      appStore.showSuccess(
        t('admin.accounts.mirasimRelay.applyMappingSuccess', { count: models.length })
      )
    } else {
      appStore.showInfo(
        t('admin.accounts.mirasimRelay.applyMappingNoChanges', { count: models.length })
      )
    }
  } catch (error) {
    // 失败只提示，不破坏当前编辑态（用户已填内容保持原样，可重试）。
    const message =
      error instanceof Error && error.message
        ? error.message
        : t('admin.accounts.mirasimRelay.applyMappingFailed')
    appStore.showError(
      t('admin.accounts.mirasimRelay.applyMappingFailedWithMessage', { message })
    )
  } finally {
    isApplyingMirasimMapping.value = false
  }
}
const webSearchGlobalEnabled = ref(false)
const {
  globalEnabled: quotaNotifyGlobalEnabled,
  state: quotaNotifyState,
  loadGlobalState: loadQuotaNotifyGlobal,
  loadFromExtra: loadQuotaNotifyFromExtra,
  writeToExtra: writeQuotaNotifyToExtra,
  reset: resetQuotaNotify,
} = useQuotaNotifyState()

// Load global feature states once
adminAPI.settings.getWebSearchEmulationConfig().then(cfg => {
  webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
}).catch(() => { webSearchGlobalEnabled.value = false })

loadQuotaNotifyGlobal()
const editQuotaLimit = ref<number | null>(null)
const editQuotaDailyLimit = ref<number | null>(null)
const editQuotaWeeklyLimit = ref<number | null>(null)
const editDailyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editDailyResetHour = ref<number | null>(null)
const editWeeklyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editWeeklyResetDay = ref<number | null>(null)
const editWeeklyResetHour = ref<number | null>(null)
const editResetTimezone = ref<string | null>(null)
const openAIWSModeOptions = computed(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
  { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') },
  { value: OPENAI_WS_MODE_HTTP_BRIDGE, label: t('admin.accounts.openai.wsModeHttpBridge') }
])
const openaiResponsesWebSocketV2Mode = computed({
  get: () => {
    if (props.account?.type === 'apikey') {
      return openaiAPIKeyResponsesWebSocketV2Mode.value
    }
    return openaiOAuthResponsesWebSocketV2Mode.value
  },
  set: (mode: OpenAIWSMode) => {
    if (props.account?.type === 'apikey') {
      openaiAPIKeyResponsesWebSocketV2Mode.value = mode
      return
    }
    openaiOAuthResponsesWebSocketV2Mode.value = mode
  }
})
const openAIWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiResponsesWebSocketV2Mode.value)
)
const codexFingerprintModeOptions = computed(() => [
  { value: 'off' as CodexFingerprintMode, label: t('admin.accounts.openai.codexFingerprintOff') },
  { value: 'device' as CodexFingerprintMode, label: t('admin.accounts.openai.codexFingerprintDevice') },
  { value: 'session' as CodexFingerprintMode, label: t('admin.accounts.openai.codexFingerprintSession') },
  { value: 'full' as CodexFingerprintMode, label: t('admin.accounts.openai.codexFingerprintFull') },
])
const codexImageToolOptions = computed<Array<{
  value: CodexImageToolMode
  label: string
  description: string
  selectedCardClass: string
  selectedDotClass: string
}>>(() => [
  {
    value: 'inherit',
    label: t('admin.accounts.openai.codexImageToolInherit'),
    description: t('admin.accounts.openai.codexImageToolInheritDesc'),
    selectedCardClass: 'border-sky-300 bg-sky-50 text-sky-900  ring-1 ring-sky-200 dark:border-sky-700 dark:bg-sky-900/25 dark:text-sky-100 dark:ring-sky-800',
    selectedDotClass: 'border-sky-500 bg-sky-500 text-white'
  },
  {
    value: 'enabled',
    label: t('admin.accounts.openai.codexImageToolEnabled'),
    description: t('admin.accounts.openai.codexImageToolEnabledDesc'),
    selectedCardClass: 'border-emerald-300 bg-emerald-50 text-emerald-900  ring-1 ring-emerald-200 dark:border-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-100 dark:ring-emerald-800',
    selectedDotClass: 'border-emerald-500 bg-emerald-500 text-white'
  },
  {
    value: 'disabled',
    label: t('admin.accounts.openai.codexImageToolDisabled'),
    description: t('admin.accounts.openai.codexImageToolDisabledDesc'),
    selectedCardClass: 'border-amber-300 bg-amber-50 text-amber-900  ring-1 ring-amber-200 dark:border-amber-700 dark:bg-amber-900/25 dark:text-amber-100 dark:ring-amber-800',
    selectedDotClass: 'border-amber-500 bg-amber-500 text-white'
  },
  {
    value: 'block',
    label: t('admin.accounts.openai.codexImageToolBlock'),
    description: t('admin.accounts.openai.codexImageToolBlockDesc'),
    selectedCardClass: 'border-rose-300 bg-rose-50 text-rose-900  ring-1 ring-rose-200 dark:border-rose-700 dark:bg-rose-900/25 dark:text-rose-100 dark:ring-rose-800',
    selectedDotClass: 'border-rose-500 bg-rose-500 text-white'
  }
])
const codexImageToolBadgeLabel = computed(() => {
  switch (codexImageToolMode.value) {
    case 'enabled':
      return t('admin.accounts.openai.codexImageToolBadgeEnabled')
    case 'disabled':
      return t('admin.accounts.openai.codexImageToolBadgeDisabled')
    case 'block':
      return t('admin.accounts.openai.codexImageToolBadgeBlock')
    default:
      return t('admin.accounts.openai.codexImageToolBadgeInherit')
  }
})
const codexImageToolBadgeClass = computed(() => {
  switch (codexImageToolMode.value) {
    case 'enabled':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'disabled':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    case 'block':
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300'
    default:
      return 'bg-slate-100 text-slate-600 dark:bg-dark-600 dark:text-slate-300'
  }
})
const openAICompactModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])
const openAIResponsesModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.responsesModeAuto') },
  { value: 'force_responses', label: t('admin.accounts.openai.responsesModeForceResponses') },
  { value: 'force_chat_completions', label: t('admin.accounts.openai.responsesModeForceChatCompletions') }
])
const openAITextEndpointCapabilityLabel = computed(() => {
  if (openAIResponsesMode.value === 'force_responses') {
    return t('admin.accounts.openai.capabilityResponses')
  }
  if (openAIResponsesMode.value === 'force_chat_completions') {
    return t('admin.accounts.openai.capabilityChatCompletions')
  }
  const extra = props.account?.extra as Record<string, unknown> | undefined
  if (extra?.openai_responses_supported === true) {
    return t('admin.accounts.openai.capabilityResponsesAuto')
  }
  if (extra?.openai_responses_supported === false) {
    return t('admin.accounts.openai.capabilityChatCompletionsAuto')
  }
  return t('admin.accounts.openai.capabilityTextAuto')
})
const openAIEndpointCapabilityOptions = computed<{ value: OpenAIEndpointCapability; label: string }[]>(() => [
  { value: 'chat_completions', label: openAITextEndpointCapabilityLabel.value },
  { value: 'embeddings', label: t('admin.accounts.openai.capabilityEmbeddings') }
])
const openAITextGenerationCapabilityEnabled = computed(() =>
  openAIEndpointCapabilities.value.includes('chat_completions')
)


const readOpenAIEndpointCapabilities = (credentials?: Record<string, unknown>): OpenAIEndpointCapability[] => {
  const raw = credentials?.openai_capabilities
  if (Array.isArray(raw)) {
    return normalizeOpenAIEndpointCapabilities(
      raw.filter((value): value is OpenAIEndpointCapability =>
        value === 'chat_completions' || value === 'embeddings'
      )
    )
  }
  if (raw !== null && typeof raw === 'object') {
    const capabilityMap = raw as Record<string, unknown>
    return normalizeOpenAIEndpointCapabilities(
      openAIEndpointCapabilityOptions.value
        .map((option) => option.value)
        .filter((value) => capabilityMap[value] === true)
    )
  }
  return ['chat_completions', 'embeddings']
}

const toggleOpenAIEndpointCapability = (capability: OpenAIEndpointCapability, event?: Event) => {
  if (openAIEndpointCapabilities.value.includes(capability)) {
    if (openAIEndpointCapabilities.value.length <= 1) {
      const input = event?.target as HTMLInputElement | null
      if (input) input.checked = true
      return
    }
    openAIEndpointCapabilities.value = openAIEndpointCapabilities.value.filter(
      (value) => value !== capability
    )
    if (!openAITextGenerationCapabilityEnabled.value) {
      openAIResponsesMode.value = 'auto'
    }
    return
  }
  openAIEndpointCapabilities.value = normalizeOpenAIEndpointCapabilities([
    ...openAIEndpointCapabilities.value,
    capability
  ])
}

const normalizeOpenAIResponsesMode = (mode: unknown): OpenAIResponsesMode => {
  if (mode === 'force_responses' || mode === 'force_chat_completions') {
    return mode
  }
  return 'auto'
}
const isOpenAIModelRestrictionDisabled = computed(() =>
  props.account?.platform === 'openai' && openaiPassthroughEnabled.value
)
const openAIResponsesStatusKey = computed(() => {
  if (openAIResponsesMode.value === 'force_responses') {
    return 'admin.accounts.openai.responsesStatusForcedResponses'
  }
  if (openAIResponsesMode.value === 'force_chat_completions') {
    return 'admin.accounts.openai.responsesStatusForcedChatCompletions'
  }
  const extra = props.account?.extra as Record<string, unknown> | undefined
  if (extra?.openai_responses_supported === true) {
    return 'admin.accounts.openai.responsesStatusAutoSupported'
  }
  if (extra?.openai_responses_supported === false) {
    return 'admin.accounts.openai.responsesStatusAutoUnsupported'
  }
  return 'admin.accounts.openai.responsesStatusAutoUnknown'
})
const openAICompactStatusKey = computed(() => {
  const extra = props.account?.extra as Record<string, unknown> | undefined
  if (!props.account || props.account.platform !== 'openai') return ''
  const mode = typeof extra?.openai_compact_mode === 'string' ? extra.openai_compact_mode : 'auto'
  if (mode === 'force_on') return 'admin.accounts.openai.compactSupported'
  if (mode === 'force_off') return 'admin.accounts.openai.compactUnsupported'
  if (typeof extra?.openai_compact_supported === 'boolean') {
    return extra.openai_compact_supported
      ? 'admin.accounts.openai.compactSupported'
      : 'admin.accounts.openai.compactUnsupported'
  }
  return 'admin.accounts.openai.compactAuto'
})

// Computed: current preset mappings based on platform
const presetMappings = computed(() => getPresetMappingsByPlatform(props.account?.platform || 'anthropic'))
const tempUnschedPresets = computed(() => [
  {
    label: t('admin.accounts.tempUnschedulable.presets.overloadLabel'),
    rule: {
      error_code: 529,
      keywords: 'overloaded, too many',
      duration_minutes: 60,
      description: t('admin.accounts.tempUnschedulable.presets.overloadDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.rateLimitLabel'),
    rule: {
      error_code: 429,
      keywords: 'rate limit, too many requests',
      duration_minutes: 10,
      description: t('admin.accounts.tempUnschedulable.presets.rateLimitDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.unavailableLabel'),
    rule: {
      error_code: 503,
      keywords: 'unavailable, maintenance',
      duration_minutes: 30,
      description: t('admin.accounts.tempUnschedulable.presets.unavailableDesc')
    }
  }
])

// Computed: default base URL based on platform
const defaultBaseUrl = computed(() => {
  return platformAPIKeyConfig(props.account?.platform ?? '').baseUrl
})

const mixedChannelWarningMessageText = computed(() => {
  if (mixedChannelWarningDetails.value) {
    return t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value)
  }
  return mixedChannelWarningRawMessage.value
})

const form = reactive({
  name: '',
  notes: '',
  proxy_id: null as number | null,
  concurrency: 1,
  load_factor: null as number | null,
  priority: 1,
  rate_multiplier: 1,
  status: 'active' as 'active' | 'inactive' | 'error',
  group_ids: [] as number[],
  expires_at: null as number | null
})

const costPreview = reactive({ basePrice: 1, salesMultiplier: 1 })
const normalizedPreviewNumber = (value: number) => Number.isFinite(value) && value > 0 ? value : 0
const costPreviewUserCharge = computed(() => normalizedPreviewNumber(costPreview.basePrice) * normalizedPreviewNumber(costPreview.salesMultiplier))
const costPreviewAccountCost = computed(() => normalizedPreviewNumber(costPreview.basePrice) * normalizedPreviewNumber(form.rate_multiplier))
const costPreviewGrossMargin = computed(() => costPreviewUserCharge.value - costPreviewAccountCost.value)
const formatPreviewNumber = (value: number) => Number.isFinite(value) ? value.toFixed(4) : '0.0000'

// 上游网关计费信息仅自动探测供参考，不回写综合成本倍率。
const upstreamBillingAutoProbeEnabled = ref(false)

const handleUpstreamBillingAutoProbeChange = (enabled: boolean) => {
  upstreamBillingAutoProbeEnabled.value = enabled
}

const statusOptions = computed(() => {
  const options = [
    { value: 'active', label: t('common.active') },
    { value: 'inactive', label: t('common.inactive') }
  ]
  if (form.status === 'error') {
    options.push({ value: 'error', label: t('admin.accounts.status.error') })
  }
  return options
})

const expiresAtInput = computed({
  get: () => formatDateTimeLocal(form.expires_at),
  set: (value: string) => {
    form.expires_at = parseDateTimeLocal(value)
  }
})

// Watchers
const normalizePoolModeRetryCount = (value: number) => {
  if (!Number.isFinite(value)) {
    return DEFAULT_POOL_MODE_RETRY_COUNT
  }
  const normalized = Math.trunc(value)
  if (normalized < 0) {
    return 0
  }
  if (normalized > MAX_POOL_MODE_RETRY_COUNT) {
    return MAX_POOL_MODE_RETRY_COUNT
  }
  return normalized
}

const loadModelRestrictionFromMapping = (rawMapping?: Record<string, unknown>) => {
  const parsed = splitModelMappingObject(rawMapping)
  allowedModels.value = parsed.allowedModels
  modelMappings.value = parsed.modelMappings
  originalModelMapping.value = Object.fromEntries(
    Object.entries(rawMapping || {}).filter((entry): entry is [string, string] => typeof entry[1] === 'string')
  )
  modelRestrictionMode.value =
    parsed.modelMappings.length > 0 && parsed.allowedModels.length === 0
      ? 'mapping'
      : 'whitelist'
}

const buildModelRestrictionMapping = () =>
  buildModelMappingObject('combined', allowedModels.value, modelMappings.value)

const applyOpenAIModelMappingCredentials = (credentials: Record<string, unknown>) => {
  const shouldApplyModelMapping = !openaiPassthroughEnabled.value

  if (shouldApplyModelMapping) {
    const modelMapping = buildModelRestrictionMapping()
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    } else {
      delete credentials.model_mapping
    }
  } else if (!credentials.model_mapping) {
    delete credentials.model_mapping
  }

  const compactModelMapping = buildModelMappingObject('mapping', [], openAICompactModelMappings.value)
  if (compactModelMapping) {
    credentials.compact_model_mapping = compactModelMapping
  } else {
    delete credentials.compact_model_mapping
  }
}

const syncFormFromAccount = (newAccount: Account | null) => {
  if (!newAccount) {
    return
  }
  originalModelMapping.value = {}
  antigravityMixedChannelConfirmed.value = false
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
  activeEditTab.value = 'basic'
  form.name = newAccount.name
  form.notes = newAccount.notes || ''
  form.proxy_id = newAccount.proxy_id
  form.concurrency = newAccount.concurrency
  form.load_factor = newAccount.load_factor ?? null
  form.priority = newAccount.priority
  form.rate_multiplier = newAccount.rate_multiplier ?? 1
  form.status = (newAccount.status === 'active' || newAccount.status === 'inactive' || newAccount.status === 'error')
    ? newAccount.status
    : 'active'
  form.group_ids = newAccount.group_ids || []
  form.expires_at = newAccount.expires_at ?? null

  // 回填上游计费倍率探测开关（extra）
  const probeExtra = newAccount.extra as Record<string, unknown> | undefined
  upstreamBillingAutoProbeEnabled.value = probeExtra?.upstream_billing_probe_enabled === true

  // Load intercept warmup requests setting (applies to all account types)
  const credentials = newAccount.credentials as Record<string, unknown> | undefined
  interceptWarmupRequests.value = credentials?.intercept_warmup_requests === true
  antigravityProjectId.value =
    newAccount.platform === 'antigravity' &&
    newAccount.type === 'oauth' &&
    typeof credentials?.antigravity_project_id === 'string'
      ? credentials.antigravity_project_id.trim()
      : ''

  // Load mixed scheduling setting (only for antigravity accounts)
  mixedScheduling.value = false
  allowOverages.value = false
	const extra = newAccount.extra as Record<string, unknown> | undefined
	mixedScheduling.value = extra?.mixed_scheduling === true
	allowOverages.value = extra?.allow_overages === true

  // Load OpenAI passthrough toggle (OpenAI OAuth/SetupToken/API Key)
  openaiPassthroughEnabled.value = false
  externalBridgeEnabled.value = false
  externalBridgeKind.value = ''
  openAICompactMode.value = 'auto'
  openAIResponsesMode.value = 'auto'
  openAIEndpointCapabilities.value = ['chat_completions', 'embeddings']
  openAICompactModelMappings.value = []
  openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  codexCLIOnlyEnabled.value = false
  codexCLIOnlyAppServerEnabled.value = false
  codexFingerprintMode.value = 'off'
  codexImageToolMode.value = 'inherit'
  anthropicPassthroughEnabled.value = false
  anthropicAPIKeyAuthScheme.value = 'x_api_key'
  webSearchEmulationMode.value = 'default'
  resetMirasimRelayForm()
  if (newAccount.platform === 'openai' && (newAccount.type === 'oauth' || newAccount.type === 'setup-token' || newAccount.type === 'apikey')) {
    openaiPassthroughEnabled.value = extra?.openai_passthrough === true || extra?.openai_oauth_passthrough === true
    externalBridgeEnabled.value = extra?.external_bridge === true || extra?.external_bridge === 'true'
    externalBridgeKind.value = typeof extra?.external_bridge_kind === 'string' ? extra.external_bridge_kind : ''
    openAICompactMode.value = (extra?.openai_compact_mode as OpenAICompactMode) || 'auto'
    if (newAccount.type === 'apikey') {
      openAIResponsesMode.value = normalizeOpenAIResponsesMode(extra?.openai_responses_mode)
      openAIEndpointCapabilities.value = readOpenAIEndpointCapabilities(
        newAccount.credentials as Record<string, unknown> | undefined
      )
      if (!openAITextGenerationCapabilityEnabled.value) {
        openAIResponsesMode.value = 'auto'
      }
    }
    const codexImageGenerationBridgeValue = typeof extra?.codex_image_generation_bridge === 'boolean'
      ? extra.codex_image_generation_bridge
      : extra?.codex_image_generation_bridge_enabled
    if (extra?.codex_image_generation_explicit_tool_policy === 'strip') {
      codexImageToolMode.value = 'block'
    } else if (codexImageGenerationBridgeValue === true) {
      codexImageToolMode.value = 'enabled'
    } else if (codexImageGenerationBridgeValue === false) {
      codexImageToolMode.value = 'disabled'
    }
    openaiOAuthResponsesWebSocketV2Mode.value = resolveOpenAIWSModeFromExtra(extra, {
      modeKey: 'openai_oauth_responses_websockets_v2_mode',
      enabledKey: 'openai_oauth_responses_websockets_v2_enabled',
      fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
      defaultMode: OPENAI_WS_MODE_OFF
    })
    openaiAPIKeyResponsesWebSocketV2Mode.value = resolveOpenAIWSModeFromExtra(extra, {
      modeKey: 'openai_apikey_responses_websockets_v2_mode',
      enabledKey: 'openai_apikey_responses_websockets_v2_enabled',
      fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
      defaultMode: OPENAI_WS_MODE_OFF
    })
    if (newAccount.type === 'oauth' || newAccount.type === 'setup-token') {
      codexCLIOnlyEnabled.value = extra?.codex_cli_only === true
      codexCLIOnlyAppServerEnabled.value =
        extra?.codex_cli_only_allow_app_server === true
    }
    if (newAccount.type === 'oauth') {
      const fingerprintMode = extra?.codex_fingerprint_mode as string | undefined
      codexFingerprintMode.value = (['off', 'device', 'session', 'full'].includes(fingerprintMode || '')
        ? fingerprintMode as CodexFingerprintMode
        : 'off')
    }
    const credentials = newAccount.credentials as Record<string, unknown> | undefined
    const compactMappings = credentials?.compact_model_mapping as Record<string, string> | undefined
    if (compactMappings && typeof compactMappings === 'object') {
      openAICompactModelMappings.value = Object.entries(compactMappings).map(([from, to]) => ({ from, to }))
    }
  }
  if (newAccount.platform === 'anthropic' && newAccount.type === 'apikey') {
    anthropicPassthroughEnabled.value = extra?.anthropic_passthrough === true
    anthropicAPIKeyAuthScheme.value = extra?.anthropic_apikey_auth_scheme === 'authorization_bearer'
      ? 'authorization_bearer'
      : 'x_api_key'
    // 三态：string "default"/"enabled"/"disabled"，向后兼容旧 bool
    const wsVal = extra?.web_search_emulation
    if (wsVal === 'enabled' || wsVal === 'disabled') {
      webSearchEmulationMode.value = wsVal
    } else if (wsVal === true) {
      webSearchEmulationMode.value = 'enabled'
    } else {
      webSearchEmulationMode.value = 'default'
    }
    // Mirasim relay：enabled 判定与后端 isMirasimRelayAccount 对齐（凭据存在且未显式关闭）
    const mirasimLoaded = mirasimRelayInputFromAccount(
      extra,
      newAccount.credentials_status?.has_mirasim_refresh_token === true
    )
    mirasimRelayEnabled.value = mirasimLoaded.enabled
    mirasimRelayInitiallyEnabled.value = mirasimLoaded.enabled
    mirasimRefreshToken.value = mirasimLoaded.refreshToken
    mirasimDeviceKey.value = mirasimLoaded.deviceKey
    mirasimRelayUrl.value = mirasimLoaded.relayUrl
    mirasimAuthUrl.value = mirasimLoaded.authUrl
    mirasimClientVersion.value = mirasimLoaded.clientVersion
    mirasimSealPubkey.value = mirasimLoaded.sealPubkey
    showMirasimOverrides.value = Boolean(
      mirasimLoaded.relayUrl || mirasimLoaded.authUrl || mirasimLoaded.clientVersion || mirasimLoaded.sealPubkey
    )
  }

  // Load quota limit for API-key accounts.
  if (newAccount.type === 'apikey') {
    const quotaVal = extra?.quota_limit as number | undefined
    editQuotaLimit.value = (quotaVal && quotaVal > 0) ? quotaVal : null
    const dailyVal = extra?.quota_daily_limit as number | undefined
    editQuotaDailyLimit.value = (dailyVal && dailyVal > 0) ? dailyVal : null
    const weeklyVal = extra?.quota_weekly_limit as number | undefined
    editQuotaWeeklyLimit.value = (weeklyVal && weeklyVal > 0) ? weeklyVal : null
    // Load quota reset mode config
    editDailyResetMode.value = (extra?.quota_daily_reset_mode as 'rolling' | 'fixed') || null
    editDailyResetHour.value = (extra?.quota_daily_reset_hour as number) ?? null
    editWeeklyResetMode.value = (extra?.quota_weekly_reset_mode as 'rolling' | 'fixed') || null
    editWeeklyResetDay.value = (extra?.quota_weekly_reset_day as number) ?? null
    editWeeklyResetHour.value = (extra?.quota_weekly_reset_hour as number) ?? null
    editResetTimezone.value = (extra?.quota_reset_timezone as string) || null
    // Load quota notify config
    loadQuotaNotifyFromExtra(extra)
  } else {
    editQuotaLimit.value = null
    editQuotaDailyLimit.value = null
    editQuotaWeeklyLimit.value = null
    editDailyResetMode.value = null
    editDailyResetHour.value = null
    editWeeklyResetMode.value = null
    editWeeklyResetDay.value = null
    editWeeklyResetHour.value = null
    editResetTimezone.value = null
    resetQuotaNotify()
  }

  // Load antigravity model mapping (Antigravity 只支持映射模式)
  if (newAccount.platform === 'antigravity') {
    const credentials = newAccount.credentials as Record<string, unknown> | undefined

    // Antigravity 始终使用映射模式
    antigravityModelRestrictionMode.value = 'mapping'
    antigravityWhitelistModels.value = []

    // 从 model_mapping 读取映射配置
    const rawAgMapping = credentials?.model_mapping as Record<string, string> | undefined
    if (rawAgMapping && typeof rawAgMapping === 'object') {
      const entries = Object.entries(rawAgMapping)
      // 无论是白名单样式(key===value)还是真正的映射，都统一转换为映射列表
      antigravityModelMappings.value = entries.map(([from, to]) => ({ from, to }))
    } else {
      // 兼容旧数据：从 model_whitelist 读取，转换为映射格式
      const rawWhitelist = credentials?.model_whitelist
      if (Array.isArray(rawWhitelist) && rawWhitelist.length > 0) {
        antigravityModelMappings.value = rawWhitelist
          .map((v) => String(v).trim())
          .filter((v) => v.length > 0)
          .map((m) => ({ from: m, to: m }))
      } else {
        antigravityModelMappings.value = []
      }
    }
  } else {
    antigravityModelRestrictionMode.value = 'mapping'
    antigravityWhitelistModels.value = []
    antigravityModelMappings.value = []
  }

  loadTempUnschedRules(credentials)

  // Reset header override state (loaded below only for apikey accounts)
  headerOverrideEnabled.value = false
  headerOverrideRows.value = []

  // Initialize API Key fields for apikey type
  if (newAccount.type === 'apikey' && newAccount.credentials) {
    const credentials = newAccount.credentials as Record<string, unknown>
    const platformDefaultUrl = platformAPIKeyConfig(newAccount.platform).baseUrl
    editBaseUrl.value = (credentials.base_url as string) || platformDefaultUrl

    // Load model mappings and detect mode
    loadModelRestrictionFromMapping(credentials.model_mapping as Record<string, unknown> | undefined)

    // Load pool mode
    poolModeEnabled.value = credentials.pool_mode === true
    poolModeRetryCount.value = normalizePoolModeRetryCount(
      Number(credentials.pool_mode_retry_count ?? DEFAULT_POOL_MODE_RETRY_COUNT)
    )
    poolModeRetryStatusCodesInput.value = formatPoolModeRetryStatusCodes(credentials.pool_mode_retry_status_codes)

    // Load custom error codes
    customErrorCodesEnabled.value = credentials.custom_error_codes_enabled === true
    const existingErrorCodes = credentials.custom_error_codes as number[] | undefined
    if (existingErrorCodes && Array.isArray(existingErrorCodes)) {
      selectedErrorCodes.value = [...existingErrorCodes]
    } else {
      selectedErrorCodes.value = []
    }

    // Load header override (anthropic/openai apikey only)
    headerOverrideEnabled.value =
      isFieldVisible('headerOverride', { platform: newAccount.platform }) &&
      credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] === true
    headerOverrideRows.value = splitHeaderOverridesObject(
      credentials[HEADER_OVERRIDES_CREDENTIAL_KEY]
    )
  } else if (newAccount.type === 'upstream' && newAccount.credentials) {
    const credentials = newAccount.credentials as Record<string, unknown>
    editBaseUrl.value = (credentials.base_url as string) || ''
  } else {
    const platformDefaultUrl = platformAPIKeyConfig(newAccount.platform).baseUrl
    editBaseUrl.value = platformDefaultUrl

    // Load model mappings for OAuth/setup-token accounts (non-antigravity)
    if (
      (newAccount.type === 'oauth' || newAccount.type === 'setup-token') &&
      isFieldVisible('oauthModelRestriction', { platform: newAccount.platform }) &&
      newAccount.credentials
    ) {
      const oauthCredentials = newAccount.credentials as Record<string, unknown>
      loadModelRestrictionFromMapping(oauthCredentials.model_mapping as Record<string, unknown> | undefined)
    } else {
      resetModelRestriction()
    }
    poolModeEnabled.value = false
    poolModeRetryCount.value = DEFAULT_POOL_MODE_RETRY_COUNT
    poolModeRetryStatusCodesInput.value = ''
    customErrorCodesEnabled.value = false
    selectedErrorCodes.value = []
  }
  editApiKey.value = ''
}

watch(
  [() => props.show, () => props.account],
  ([show, newAccount], [wasShow, previousAccount]) => {
    if (!show || !newAccount) {
      activeEditTab.value = 'basic'
      return
    }
    if (!wasShow || newAccount !== previousAccount) {
      syncFormFromAccount(newAccount)
    }
  },
  { immediate: true }
)

const addPresetMapping = (from: string, to: string) => {
  if (!addModelPresetMapping(from, to)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
  }
}

const addAntigravityModelMapping = () => {
  antigravityModelMappings.value.push({ from: '', to: '' })
}

const addOpenAICompactModelMapping = () => {
  openAICompactModelMappings.value.push({ from: '', to: '' })
}

const removeOpenAICompactModelMapping = (index: number) => {
  openAICompactModelMappings.value.splice(index, 1)
}

const removeAntigravityModelMapping = (index: number) => {
  antigravityModelMappings.value.splice(index, 1)
}

const addAntigravityPresetMapping = (from: string, to: string) => {
  const exists = antigravityModelMappings.value.some((m) => m.from === from)
  if (exists) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
    return
  }
  antigravityModelMappings.value.push({ from, to })
}

const syncAntigravityUpstreamModels = async () => {
  if (!props.account?.id || isSyncingAntigravityUpstream.value) return

  isSyncingAntigravityUpstream.value = true
  try {
    const result = await adminAPI.accounts.syncUpstreamModels(props.account.id)
    const upstreamModels = result.models.map((model) => model.trim()).filter(Boolean)
    if (upstreamModels.length === 0) {
      appStore.showInfo(t('admin.accounts.syncUpstreamModelsEmpty'))
      return
    }

    let addedCount = 0
    for (const model of upstreamModels) {
      const exists = antigravityModelMappings.value.some((mapping) => mapping.from === model)
      if (!exists) {
        antigravityModelMappings.value.push({ from: model, to: model })
        addedCount += 1
      }
    }

    if (addedCount > 0) {
      appStore.showSuccess(t('admin.accounts.syncUpstreamModelsSuccess', { count: addedCount, total: upstreamModels.length }))
    } else {
      appStore.showInfo(t('admin.accounts.syncUpstreamModelsNoChanges', { count: upstreamModels.length }))
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : t('admin.accounts.syncUpstreamModelsFailed')
    appStore.showError(t('admin.accounts.syncUpstreamModelsError', { message }))
  } finally {
    isSyncingAntigravityUpstream.value = false
  }
}

// Error code toggle helper
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    // Adding code - check for 429/529 warning
    if (code === 429) {
      if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
        return
      }
    } else if (code === 529) {
      if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
        return
      }
    }
    selectedErrorCodes.value.push(code)
  } else {
    selectedErrorCodes.value.splice(index, 1)
  }
}

// Add custom error code from input
const addCustomErrorCode = () => {
  const code = customErrorCodeInput.value
  if (code === null || code < 100 || code > 599) {
    appStore.showError(t('admin.accounts.invalidErrorCode'))
    return
  }
  if (selectedErrorCodes.value.includes(code)) {
    appStore.showInfo(t('admin.accounts.errorCodeExists'))
    return
  }
  // Check for 429/529 warning
  if (code === 429) {
    if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
      return
    }
  } else if (code === 529) {
    if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
      return
    }
  }
  selectedErrorCodes.value.push(code)
  customErrorCodeInput.value = null
}

// Remove error code
const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const addTempUnschedRule = (preset?: TempUnschedRuleForm) => {
  if (preset) {
    tempUnschedRules.value.push({ ...preset })
    return
  }
  tempUnschedRules.value.push({
    error_code: null,
    keywords: '',
    duration_minutes: 30,
    description: ''
  })
}

const removeTempUnschedRule = (index: number) => {
  tempUnschedRules.value.splice(index, 1)
}

const moveTempUnschedRule = (index: number, direction: number) => {
  const target = index + direction
  if (target < 0 || target >= tempUnschedRules.value.length) return
  const rules = tempUnschedRules.value
  const current = rules[index]
  rules[index] = rules[target]
  rules[target] = current
}

const buildTempUnschedRules = (rules: TempUnschedRuleForm[]) => {
  const out: Array<{
    error_code: number
    keywords: string[]
    duration_minutes: number
    description: string
  }> = []

  for (const rule of rules) {
    const errorCode = Number(rule.error_code)
    const duration = Number(rule.duration_minutes)
    const keywords = splitTempUnschedKeywords(rule.keywords)
    if (!Number.isFinite(errorCode) || errorCode < 100 || errorCode > 599) {
      continue
    }
    if (!Number.isFinite(duration) || duration <= 0) {
      continue
    }
    if (keywords.length === 0) {
      continue
    }
    out.push({
      error_code: Math.trunc(errorCode),
      keywords,
      duration_minutes: Math.trunc(duration),
      description: rule.description.trim()
    })
  }

  return out
}

const applyTempUnschedConfig = (credentials: Record<string, unknown>) => {
  if (!tempUnschedEnabled.value) {
    delete credentials.temp_unschedulable_enabled
    delete credentials.temp_unschedulable_rules
    return true
  }

  const rules = buildTempUnschedRules(tempUnschedRules.value)
  if (rules.length === 0) {
    appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
    return false
  }

  credentials.temp_unschedulable_enabled = true
  credentials.temp_unschedulable_rules = rules
  return true
}

function loadTempUnschedRules(credentials?: Record<string, unknown>) {
  tempUnschedEnabled.value = credentials?.temp_unschedulable_enabled === true
  const rawRules = credentials?.temp_unschedulable_rules
  if (!Array.isArray(rawRules)) {
    tempUnschedRules.value = []
    return
  }

  tempUnschedRules.value = rawRules.map((rule) => {
    const entry = rule as Record<string, unknown>
    return {
      error_code: toPositiveNumber(entry.error_code),
      keywords: formatTempUnschedKeywords(entry.keywords),
      duration_minutes: toPositiveNumber(entry.duration_minutes),
      description: typeof entry.description === 'string' ? entry.description : ''
    }
  })
}


function formatTempUnschedKeywords(value: unknown) {
  if (Array.isArray(value)) {
    return value
      .filter((item): item is string => typeof item === 'string')
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
      .join(', ')
  }
  if (typeof value === 'string') {
    return value
  }
  return ''
}

const splitTempUnschedKeywords = (value: string) => {
  return value
    .split(/[,;]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

function toPositiveNumber(value: unknown) {
  const num = Number(value)
  if (!Number.isFinite(num) || num <= 0) {
    return null
  }
  return Math.trunc(num)
}

const needsMixedChannelCheck = () => props.account?.platform === 'antigravity' || props.account?.platform === 'anthropic'

const buildMixedChannelDetails = (resp?: CheckMixedChannelResponse) => {
  const details = resp?.details
  if (!details) {
    return null
  }
  return {
    groupName: details.group_name || 'Unknown',
    currentPlatform: details.current_platform || 'Unknown',
    otherPlatform: details.other_platform || 'Unknown'
  }
}

const clearMixedChannelDialog = () => {
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
}

const openMixedChannelDialog = (opts: {
  response?: CheckMixedChannelResponse
  message?: string
  onConfirm: () => Promise<void>
}) => {
  mixedChannelWarningDetails.value = buildMixedChannelDetails(opts.response)
  mixedChannelWarningRawMessage.value =
    opts.message || opts.response?.message || t('admin.accounts.failedToUpdate')
  mixedChannelWarningAction.value = opts.onConfirm
  showMixedChannelWarning.value = true
}

const withAntigravityConfirmFlag = (payload: Record<string, unknown>) => {
  if (needsMixedChannelCheck() && antigravityMixedChannelConfirmed.value) {
    return {
      ...payload,
      confirm_mixed_channel_risk: true
    }
  }
  const cloned = { ...payload }
  delete cloned.confirm_mixed_channel_risk
  return cloned
}

const ensureAntigravityMixedChannelConfirmed = async (onConfirm: () => Promise<void>): Promise<boolean> => {
  if (!needsMixedChannelCheck()) {
    return true
  }
  if (antigravityMixedChannelConfirmed.value) {
    return true
  }
  if (!props.account) {
    return false
  }

  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({
      platform: props.account.platform,
      group_ids: form.group_ids,
      account_id: props.account.id
    })
    if (!result.has_risk) {
      return true
    }
    openMixedChannelDialog({
      response: result,
      onConfirm: async () => {
        antigravityMixedChannelConfirmed.value = true
        await onConfirm()
      }
    })
    return false
  } catch (error: any) {
    appStore.showError(error.message || t('admin.accounts.failedToUpdate'))
    return false
  }
}

const formatDateTimeLocal = formatDateTimeLocalInput
const parseDateTimeLocal = parseDateTimeLocalInput

// Methods
const handleClose = () => {
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
  emit('close')
}

const submitUpdateAccount = async (accountID: number, updatePayload: Record<string, unknown>) => {
  submitting.value = true
  try {
    // Only apply the edits made in this drawer to the latest mapping. Opening an
    // old drawer and saving an unrelated field must not roll back model syncs.
    if (props.account?.platform === 'openai' && props.account.type === 'apikey' && updatePayload.credentials) {
      const latest = await adminAPI.accounts.getById(accountID)
      const credentials = updatePayload.credentials as Record<string, unknown>
      const merged = mergeModelMappingChanges(
        originalModelMapping.value,
        (credentials.model_mapping || {}) as Record<string, string>,
        (latest.credentials?.model_mapping || {}) as Record<string, string>
      )
      if (merged.conflicts.length) {
        appStore.showError(t('admin.accounts.openai.modelMappingConflict', { models: merged.conflicts.join(', ') }))
        return
      }
      if (Object.keys(merged.mapping).length) credentials.model_mapping = merged.mapping
      else delete credentials.model_mapping
    }
    const updatedAccount = await adminAPI.accounts.update(accountID, withAntigravityConfirmFlag(updatePayload))
    appStore.showSuccess(t('admin.accounts.accountUpdated'))
    emit('updated', updatedAccount)
    handleClose()
  } catch (error: any) {
    if (error.status === 409 && error.error === 'mixed_channel_warning' && needsMixedChannelCheck()) {
      openMixedChannelDialog({
        message: error.message,
        onConfirm: async () => {
          antigravityMixedChannelConfirmed.value = true
          await submitUpdateAccount(accountID, updatePayload)
        }
      })
      return
    }
    appStore.showError(error.message || t('admin.accounts.failedToUpdate'))
  } finally {
    submitting.value = false
  }
}

const handleCostConfigSaved = async (payload?: { account_expires_at?: string | null }) => {
  if (!props.account) return
  if (payload && Object.prototype.hasOwnProperty.call(payload, 'account_expires_at')) {
    const raw = payload.account_expires_at
    if (!raw) {
      form.expires_at = null
    } else {
      const ms = Date.parse(raw)
      if (Number.isFinite(ms)) {
        form.expires_at = Math.floor(ms / 1000)
      }
    }
  }
  // Cost configuration is saved by its own child panel. Do not emit the
  // account-level `updated` event here: the parent list would replace the
  // account prop while this drawer is open, resetting the active tab and
  // making the outer account form race with the inner save.
}

const handleSubmit = async () => {

  if (!props.account) return
  const accountID = props.account.id
  if (props.account.platform === 'openai' && props.account.type === 'apikey' &&
      externalBridgeEnabled.value && !externalBridgeKind.value.trim()) {
    activeEditTab.value = 'advanced'
    appStore.showError(t('admin.accounts.openai.externalBridgeKindRequired'))
    return
  }

  // 不用 HTML required：抽屉多 Tab 下隐藏字段会触发
  // "An invalid form control with name='' is not focusable."
  if (!String(form.name || '').trim()) {
    activeEditTab.value = 'basic'
    appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
    return
  }

  if (form.status !== 'active' && form.status !== 'inactive' && form.status !== 'error') {
    appStore.showError(t('admin.accounts.pleaseSelectStatus'))
    return
  }

  const updatePayload: Record<string, unknown> = { ...form }
  try {
    // 后端期望 proxy_id: 0 表示清除代理，而不是 null
    if (updatePayload.proxy_id === null) {
      updatePayload.proxy_id = 0
    }
    if (isSubscriptionAccount.value) {
      // 订阅号过期时间由成本周期驱动，账号更新不覆盖
      delete updatePayload.expires_at
    } else {
      // API Key 等非订阅账号无调度过期概念
      updatePayload.expires_at = 0
    }
    // load_factor: 空值/NaN/0/负数 时发送 0（后端约定 <= 0 = 清除）
    const lf = form.load_factor
    if (lf == null || Number.isNaN(lf) || lf <= 0) {
      updatePayload.load_factor = 0
    }

    // For apikey type, handle credentials update
    if (props.account.type === 'apikey') {
      const currentCredentials = (props.account.credentials as Record<string, unknown>) || {}
      const newBaseUrl = editBaseUrl.value.trim() || defaultBaseUrl.value
      const shouldApplyModelMapping = !(props.account.platform === 'openai' && openaiPassthroughEnabled.value)

      // Always update credentials for apikey type to handle model mapping changes
      const newCredentials: Record<string, unknown> = {
        ...currentCredentials,
        base_url: newBaseUrl
      }

      // Handle API key
      // 后端响应已脱敏：currentCredentials 不会再包含 api_key 原文。
      // 用户填入新值则覆盖；留空时优先看 credentials_status.has_api_key；
      // 若后端尚未升级（无 credentials_status），回退读旧结构 currentCredentials.api_key。
      // 两者都无才报错。
      const hasExistingApiKey =
        props.account.credentials_status?.has_api_key ?? Boolean(currentCredentials.api_key)
      if (editApiKey.value.trim()) {
        newCredentials.api_key = editApiKey.value.trim()
      } else if (!hasExistingApiKey && !mirasimRelayActive.value) {
        appStore.showError(t('admin.accounts.apiKeyIsRequired'))
        return
      }

      // Mirasim relay 账号没有 api_key：运行时凭据链会用 refresh token 换取 ticket。
      // 敏感值后端不回显，因此已有 refresh token 时允许留空。
      if (mirasimRelayActive.value) {
        const mirasimError = validateMirasimRelayInput(
          mirasimRelayInput.value,
          hasExistingMirasimRefreshToken.value
        )
        if (mirasimError) {
          appStore.showError(t(`admin.accounts.mirasimRelay.${mirasimError}`))
          return
        }
      }

      // Add model mapping if configured（OpenAI 开启自动透传时保留现有映射，不再编辑）
      if (shouldApplyModelMapping) {
        const modelMapping = buildModelRestrictionMapping()
        if (modelMapping) {
          newCredentials.model_mapping = modelMapping
        } else {
          delete newCredentials.model_mapping
        }
      } else if (currentCredentials.model_mapping) {
        newCredentials.model_mapping = currentCredentials.model_mapping
      }
      if (props.account.platform === 'openai') {
        applyOpenAIEndpointCapabilities(newCredentials, openAIEndpointCapabilities.value)
        const compactModelMapping = buildModelMappingObject('mapping', [], openAICompactModelMappings.value)
        if (compactModelMapping) {
          newCredentials.compact_model_mapping = compactModelMapping
        } else {
          delete newCredentials.compact_model_mapping
        }
      }
      applyGrokAPIKeyRouting(newCredentials, props.account.platform, props.account.type)

      // Add pool mode if enabled
      if (poolModeEnabled.value) {
        newCredentials.pool_mode = true
        newCredentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
        const parsedRetryStatusCodes = parsePoolModeRetryStatusCodes(poolModeRetryStatusCodesInput.value)
        if (parsedRetryStatusCodes.length > 0) {
          newCredentials.pool_mode_retry_status_codes = parsedRetryStatusCodes
        } else {
          delete newCredentials.pool_mode_retry_status_codes
        }
      } else {
        delete newCredentials.pool_mode
        delete newCredentials.pool_mode_retry_count
        delete newCredentials.pool_mode_retry_status_codes
      }

      // Add custom error codes if enabled
      if (customErrorCodesEnabled.value) {
        newCredentials.custom_error_codes_enabled = true
        newCredentials.custom_error_codes = [...selectedErrorCodes.value]
      } else {
        delete newCredentials.custom_error_codes_enabled
        delete newCredentials.custom_error_codes
      }

      // Add header override if enabled (anthropic/openai apikey only)
      if (isFieldVisible('headerOverride', { platform: props.account.platform })) {
        if (headerOverrideEnabled.value) {
          const headerError = validateHeaderOverrideRows(headerOverrideRows.value)
          if (headerError) {
            appStore.showError(t(`admin.accounts.headerOverride.${headerError}`))
            return
          }
        }
        applyHeaderOverride(newCredentials, headerOverrideEnabled.value, headerOverrideRows.value, 'edit')
      }

      // Add intercept warmup requests setting
      applyInterceptWarmup(newCredentials, interceptWarmupRequests.value, 'edit')
      if (!applyTempUnschedConfig(newCredentials)) {
        return
      }

      updatePayload.credentials = newCredentials
    } else if (props.account.type === 'upstream') {
      const currentCredentials = (props.account.credentials as Record<string, unknown>) || {}
      const newCredentials: Record<string, unknown> = { ...currentCredentials }

      newCredentials.base_url = editBaseUrl.value.trim()

      if (editApiKey.value.trim()) {
        newCredentials.api_key = editApiKey.value.trim()
      }

      // Add intercept warmup requests setting
      applyInterceptWarmup(newCredentials, interceptWarmupRequests.value, 'edit')

      if (!applyTempUnschedConfig(newCredentials)) {
        return
      }

      updatePayload.credentials = newCredentials
    } else {
      // For oauth/setup-token types, only update intercept_warmup_requests if changed
      const currentCredentials = (props.account.credentials as Record<string, unknown>) || {}
      const newCredentials: Record<string, unknown> = { ...currentCredentials }

      applyInterceptWarmup(newCredentials, interceptWarmupRequests.value, 'edit')
      if (!applyTempUnschedConfig(newCredentials)) {
        return
      }

      updatePayload.credentials = newCredentials
    }

    // OAuth/setup-token: persist model mapping to credentials (non-antigravity)
    if (supportsOAuthModelRestriction.value) {
      const currentCredentials = isSparkShadow.value
        ? {}
        : (updatePayload.credentials as Record<string, unknown>) ||
          ((props.account.credentials as Record<string, unknown>) || {})
      const newCredentials: Record<string, unknown> = { ...currentCredentials }
      if (props.account.platform === 'openai') {
        applyOpenAIModelMappingCredentials(newCredentials)
      } else {
        const modelMapping = buildModelRestrictionMapping()
        if (modelMapping) {
          newCredentials.model_mapping = modelMapping
        } else {
          delete newCredentials.model_mapping
        }
      }

      updatePayload.credentials = newCredentials
    }

    // Antigravity: persist model mapping to credentials (applies to all antigravity types)
    // Antigravity 只支持映射模式
    if (props.account.platform === 'antigravity') {
      const currentCredentials = (updatePayload.credentials as Record<string, unknown>) ||
        ((props.account.credentials as Record<string, unknown>) || {})
      const newCredentials: Record<string, unknown> = { ...currentCredentials }
      if (props.account.type === 'oauth') {
        applyAntigravityProjectID(newCredentials, antigravityProjectId.value, 'edit')
      }

      // 移除旧字段
      delete newCredentials.model_whitelist
      delete newCredentials.model_mapping

      // 只使用映射模式
      const antigravityModelMapping = buildModelMappingObject(
        'mapping',
        [],
        antigravityModelMappings.value
      )
      if (antigravityModelMapping) {
        newCredentials.model_mapping = antigravityModelMapping
      }

      updatePayload.credentials = newCredentials
    }

    // For antigravity accounts, handle mixed_scheduling and allow_overages in extra
    if (props.account.platform === 'antigravity') {
      const currentExtra = (updatePayload.extra as Record<string, unknown>) || (props.account.extra as Record<string, unknown>) || {}
      const newExtra: Record<string, unknown> = { ...currentExtra }
      applyAntigravityExtra(newExtra, mixedScheduling.value, allowOverages.value)
      updatePayload.extra = newExtra
    }

    // For Anthropic API Key accounts, handle passthrough mode + web search emulation in extra
    if (props.account.platform === 'anthropic' && props.account.type === 'apikey') {
      const currentExtra = (updatePayload.extra as Record<string, unknown>) || (props.account.extra as Record<string, unknown>) || {}
      const newExtra: Record<string, unknown> = { ...currentExtra }
      applyAnthropicExtra(newExtra, anthropicPassthroughEnabled.value, anthropicAPIKeyAuthScheme.value, webSearchEmulationMode.value)
      // Mirasim relay：同时改写 credentials（base_url / refresh token / api_key），
      // 所以必须传入前面已构建好的 updatePayload.credentials。
      applyMirasimRelay(
        (updatePayload.credentials as Record<string, unknown>) || {},
        newExtra,
        mirasimRelayInput.value,
        'edit',
        { currentBaseUrl: editBaseUrl.value, previousEnabled: mirasimRelayInitiallyEnabled.value }
      )
      updatePayload.extra = newExtra
    }

    // For OpenAI OAuth/SetupToken/API Key accounts, handle passthrough mode in extra
    if (props.account.platform === 'openai' && (props.account.type === 'oauth' || props.account.type === 'setup-token' || props.account.type === 'apikey')) {
      const currentExtra = (props.account.extra as Record<string, unknown>) || {}
      const newExtra: Record<string, unknown> = { ...currentExtra }
      const hadCodexCLIOnlyEnabled = currentExtra.codex_cli_only === true
      applyOpenAIWebSocketExtra(newExtra, props.account.type, openaiOAuthResponsesWebSocketV2Mode.value, openaiAPIKeyResponsesWebSocketV2Mode.value, isOpenAIWSModeEnabled)
      applyOpenAIPassthrough(newExtra, openaiPassthroughEnabled.value, 'edit')
      applyOpenAIExternalBridge(newExtra, props.account.type === 'apikey' && externalBridgeEnabled.value, externalBridgeKind.value)
      applyOpenAICompactMode(newExtra, openAICompactMode.value, 'edit')
		if (props.account.type === 'apikey') {
        if (!openAITextGenerationCapabilityEnabled.value || openAIResponsesMode.value === 'auto') {
          delete newExtra.openai_responses_mode
        } else {
          newExtra.openai_responses_mode = openAIResponsesMode.value
        }
		}
		delete newExtra.codex_image_generation_bridge_enabled
      switch (codexImageToolMode.value) {
        case 'enabled':
        case 'disabled':
          newExtra.codex_image_generation_bridge = codexImageToolMode.value === 'enabled'
          delete newExtra.codex_image_generation_explicit_tool_policy
          break
        case 'block':
          newExtra.codex_image_generation_explicit_tool_policy = 'strip'
          delete newExtra.codex_image_generation_bridge
          break
        default:
          delete newExtra.codex_image_generation_bridge
          delete newExtra.codex_image_generation_explicit_tool_policy
      }

      if (props.account.type === 'oauth' || props.account.type === 'setup-token') {
        if (codexCLIOnlyEnabled.value) {
          newExtra.codex_cli_only = true
        } else if (hadCodexCLIOnlyEnabled) {
          // 关闭时显式写 false，避免 extra 为空被后端忽略导致旧值无法清除
          newExtra.codex_cli_only = false
        } else {
          delete newExtra.codex_cli_only
        }
        // Claude Code 插件放行已迁移到全局 codex_cli_only_whitelist，编辑时清理废弃账号级快捷字段。
        delete newExtra.codex_cli_only_allowed_clients
        if (codexCLIOnlyEnabled.value && codexCLIOnlyAppServerEnabled.value) {
          newExtra.codex_cli_only_allow_app_server = true
        } else {
          delete newExtra.codex_cli_only_allow_app_server
        }
      }

      if (props.account.type === 'oauth') {
        newExtra.codex_fingerprint_mode = codexFingerprintMode.value
      }

      updatePayload.extra = newExtra
    }

    // For API-key accounts, handle quota_limit in extra.
    if (props.account.type === 'apikey') {
      const currentExtra = (updatePayload.extra as Record<string, unknown>) ||
        (props.account.extra as Record<string, unknown>) || {}
      const newExtra: Record<string, unknown> = { ...currentExtra }
      // 上游计费信息只做观测；永久清理旧的倍率写回开关。
      if (upstreamBillingAutoProbeEnabled.value) {
        newExtra.upstream_billing_probe_enabled = true
      } else {
        delete newExtra.upstream_billing_probe_enabled
      }
      delete newExtra.upstream_billing_rate_sync_enabled
      // Total quota
      if (editQuotaLimit.value != null && editQuotaLimit.value > 0) {
        newExtra.quota_limit = editQuotaLimit.value
      } else {
        delete newExtra.quota_limit
      }
      // Daily quota
      if (editQuotaDailyLimit.value != null && editQuotaDailyLimit.value > 0) {
        newExtra.quota_daily_limit = editQuotaDailyLimit.value
      } else {
        delete newExtra.quota_daily_limit
        delete newExtra.quota_daily_used
        delete newExtra.quota_daily_start
      }
      // Weekly quota
      if (editQuotaWeeklyLimit.value != null && editQuotaWeeklyLimit.value > 0) {
        newExtra.quota_weekly_limit = editQuotaWeeklyLimit.value
      } else {
        delete newExtra.quota_weekly_limit
        delete newExtra.quota_weekly_used
        delete newExtra.quota_weekly_start
      }
      // Quota reset mode config
      if (editDailyResetMode.value === 'fixed') {
        newExtra.quota_daily_reset_mode = 'fixed'
        newExtra.quota_daily_reset_hour = editDailyResetHour.value ?? 0
      } else {
        delete newExtra.quota_daily_reset_mode
        delete newExtra.quota_daily_reset_hour
      }
      if (editWeeklyResetMode.value === 'fixed') {
        newExtra.quota_weekly_reset_mode = 'fixed'
        newExtra.quota_weekly_reset_day = editWeeklyResetDay.value ?? 1
        newExtra.quota_weekly_reset_hour = editWeeklyResetHour.value ?? 0
      } else {
        delete newExtra.quota_weekly_reset_mode
        delete newExtra.quota_weekly_reset_day
        delete newExtra.quota_weekly_reset_hour
      }
      if (editDailyResetMode.value === 'fixed' || editWeeklyResetMode.value === 'fixed') {
        newExtra.quota_reset_timezone = editResetTimezone.value || 'UTC'
      } else {
        delete newExtra.quota_reset_timezone
      }
      // Quota notify config
      writeQuotaNotifyToExtra(newExtra, 'update')
      updatePayload.extra = newExtra
    }

    const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
      await submitUpdateAccount(accountID, updatePayload)
    })
    if (!canContinue) {
      return
    }

    await submitUpdateAccount(accountID, updatePayload)
  } catch (error: any) {
    appStore.showError(error.message || t('admin.accounts.failedToUpdate'))
  }
}

// Handle mixed channel warning confirmation
const handleMixedChannelConfirm = async () => {
  const action = mixedChannelWarningAction.value
  if (!action) {
    clearMixedChannelDialog()
    return
  }
  clearMixedChannelDialog()
  submitting.value = true
  try {
    await action()
  } finally {
    submitting.value = false
  }
}

const handleMixedChannelCancel = () => {
  clearMixedChannelDialog()
}
</script>
