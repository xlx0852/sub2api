import { computed, onBeforeUnmount, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { KimiCreateAccountRequest, KimiDeviceAuthorization } from '@/api/admin/kimi'

export function useKimiOAuth() {
  const session = ref<KimiDeviceAuthorization | null>(null)
  const loading = ref(false)
  const error = ref('')
  let timer: ReturnType<typeof setTimeout> | null = null
  let deadline = 0

  const remainingSeconds = ref(0)
  const authorized = computed(() => session.value?.status === 'authorized')

  const clearTimer = () => {
    if (timer) clearTimeout(timer)
    timer = null
  }

  const updateRemaining = () => {
    remainingSeconds.value = Math.max(0, Math.ceil((deadline - Date.now()) / 1000))
  }

  const schedulePoll = (seconds?: number) => {
    clearTimer()
    updateRemaining()
    if (!session.value || session.value.status !== 'pending' || remainingSeconds.value <= 0) return
    const delay = Math.max(1, seconds || session.value.retry_after || session.value.interval || 5)
    timer = setTimeout(poll, delay * 1000)
  }

  const applySession = (value: KimiDeviceAuthorization, preserveDeadline = true) => {
    session.value = value
    if (!preserveDeadline || deadline === 0) deadline = Date.now() + value.expires_in * 1000
    updateRemaining()
  }

  const poll = async () => {
    if (!session.value?.session_id || session.value.status !== 'pending') return
    try {
      const value = await adminAPI.kimi.getDeviceAuthorizationStatus(session.value.session_id)
      applySession(value)
      if (value.status === 'pending') schedulePoll(value.retry_after)
    } catch (err: any) {
      error.value = err.response?.data?.message || err.response?.data?.detail || 'Kimi authorization status check failed'
      clearTimer()
    }
  }

  const start = async (proxyId?: number | null) => {
    clearTimer()
    loading.value = true
    error.value = ''
    session.value = null
    deadline = 0
    try {
      const value = await adminAPI.kimi.startDeviceAuthorization(proxyId)
      applySession(value, false)
      schedulePoll(value.interval)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.message || err.response?.data?.detail || 'Failed to start Kimi authorization'
      return false
    } finally {
      loading.value = false
    }
  }

  const cancel = async () => {
    clearTimer()
    const sessionId = session.value?.session_id
    session.value = null
    deadline = 0
    remainingSeconds.value = 0
    if (sessionId) await adminAPI.kimi.cancelDeviceAuthorization(sessionId).catch(() => undefined)
  }

  const createAccount = async (payload: Omit<KimiCreateAccountRequest, 'session_id'>) => {
    if (!session.value?.session_id || !authorized.value) throw new Error('Kimi authorization is not complete')
    await adminAPI.kimi.createAccountFromDevice({ ...payload, session_id: session.value.session_id })
    clearTimer()
  }

  const reauthorizeAccount = async (accountId: number) => {
    if (!session.value?.session_id || !authorized.value) throw new Error('Kimi authorization is not complete')
    const account = await adminAPI.kimi.reauthorizeAccountFromDevice(accountId, session.value.session_id)
    clearTimer()
    return account
  }

  const resetState = () => {
    clearTimer()
    session.value = null
    error.value = ''
    loading.value = false
    deadline = 0
    remainingSeconds.value = 0
  }

  onBeforeUnmount(clearTimer)
  return { session, loading, error, authorized, remainingSeconds, start, poll, cancel, createAccount, reauthorizeAccount, resetState }
}
