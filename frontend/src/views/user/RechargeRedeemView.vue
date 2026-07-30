<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <div>
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
              {{ t('nav.rechargeRedeem') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ activeTab === 'recharge' ? t('purchase.description') : t('redeem.description') }}
            </p>
          </div>
          <div class="inline-flex w-full rounded-xl bg-gray-100 p-1 dark:bg-dark-700 sm:w-auto" role="tablist">
            <button
              type="button"
              role="tab"
              :aria-selected="activeTab === 'recharge'"
              class="recharge-redeem-tab"
              :class="activeTab === 'recharge' && 'recharge-redeem-tab-active'"
              @click="selectTab('recharge')"
            >
              <Icon name="creditCard" size="sm" />
              {{ t('nav.buySubscription') }}
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="activeTab === 'redeem'"
              class="recharge-redeem-tab"
              :class="activeTab === 'redeem' && 'recharge-redeem-tab-active'"
              @click="selectTab('redeem')"
            >
              <Icon name="gift" size="sm" />
              {{ t('nav.redeem') }}
            </button>
          </div>
        </div>
      </section>

      <PaymentView v-if="activeTab === 'recharge'" embedded />
      <RedeemView v-else embedded />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PaymentView from './PaymentView.vue'
import RedeemView from './RedeemView.vue'

type RechargeTab = 'recharge' | 'redeem'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const activeTab = computed<RechargeTab>(() =>
  route.query.tab === 'redeem' ? 'redeem' : 'recharge',
)

function selectTab(tab: RechargeTab) {
  if (tab === activeTab.value) return
  router.push({ path: '/purchase', query: tab === 'redeem' ? { tab } : {} })
}
</script>

<style scoped>
.recharge-redeem-tab {
  @apply inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-gray-500 transition-colors dark:text-dark-300 sm:flex-none;
}

.recharge-redeem-tab:hover {
  @apply text-gray-800 dark:text-white;
}

.recharge-redeem-tab-active {
  @apply bg-white text-primary-700 shadow-sm dark:bg-dark-600 dark:text-primary-200;
}
</style>
