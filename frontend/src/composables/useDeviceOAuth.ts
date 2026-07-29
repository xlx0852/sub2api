import { computed, onBeforeUnmount, ref } from 'vue'
import type {
  DeviceAuthorization,
  DeviceCreateAccountRequest
} from '@/api/admin/deviceOAuth'
import type { Account } from '@/types'

export interface DeviceOAuthClient<T extends DeviceAuthorization = DeviceAuthorization> {
  start(proxyId?: number | null): Promise<T>
  status(sessionId: string): Promise<T>
  cancel(sessionId: string): Promise<void>
  createAccount(payload: DeviceCreateAccountRequest): Promise<Account>
  reauthorizeAccount(accountId: number, sessionId: string): Promise<Account>
}

export interface DeviceOAuthOptions<T extends DeviceAuthorization = DeviceAuthorization> {
  client: DeviceOAuthClient<T>
  provider: string
  startError?: string
  statusError?: string
}

/**
 * Shared state machine for OAuth device-code flows.
 *
 * The server owns the upstream polling and returns a pending/authorized/denied
 * session.  The browser only schedules the next status request, so this works
 * for providers with different polling intervals and expiration windows.
 */
export function useDeviceOAuth<T extends DeviceAuthorization = DeviceAuthorization>(
  options: DeviceOAuthOptions<T>
) {
  const session = ref<T | null>(null)
  const loading = ref(false)
  const error = ref('')
  const remainingSeconds = ref(0)
  const authorized = computed(() => session.value?.status === 'authorized')

  let timer: ReturnType<typeof setTimeout> | null = null
  let deadline = 0
  let generation = 0

  const clearTimer = () => {
    if (timer) clearTimeout(timer)
    timer = null
  }

  const updateRemaining = () => {
    remainingSeconds.value = Math.max(0, Math.ceil((deadline - Date.now()) / 1000))
  }

  const applySession = (value: T, resetDeadline = false) => {
    session.value = value
    if (resetDeadline || deadline === 0) {
      deadline = Date.now() + Math.max(0, Number(value.expires_in) || 0) * 1000
    }
    updateRemaining()
  }

  const schedulePoll = (seconds?: number) => {
    clearTimer()
    updateRemaining()
    if (!session.value || session.value.status !== 'pending' || remainingSeconds.value <= 0) return
    const delay = Math.max(1, Number(seconds || session.value.retry_after || session.value.interval || 5))
    const currentGeneration = generation
    timer = setTimeout(() => {
      if (currentGeneration === generation) void poll()
    }, delay * 1000)
  }

  const poll = async (): Promise<boolean> => {
    const current = session.value
    if (!current?.session_id || current.status !== 'pending') return false
    const currentGeneration = generation
    try {
      const value = await options.client.status(current.session_id)
      if (currentGeneration !== generation) return false
      applySession(value)
      if (value.status === 'pending') schedulePoll(value.retry_after)
      return value.status === 'authorized'
    } catch (err: any) {
      if (currentGeneration !== generation) return false
      error.value =
        err?.response?.data?.message ||
        err?.response?.data?.detail ||
        options.statusError ||
        `${options.provider} authorization status check failed`
      clearTimer()
      return false
    }
  }

  const start = async (proxyId?: number | null): Promise<boolean> => {
    clearTimer()
    generation += 1
    loading.value = true
    error.value = ''
    session.value = null
    deadline = 0
    remainingSeconds.value = 0
    try {
      const value = await options.client.start(proxyId)
      applySession(value, true)
      schedulePoll(value.interval)
      return true
    } catch (err: any) {
      error.value =
        err?.response?.data?.message ||
        err?.response?.data?.detail ||
        options.startError ||
        `Failed to start ${options.provider} authorization`
      return false
    } finally {
      loading.value = false
    }
  }

  const cancel = async () => {
    clearTimer()
    generation += 1
    const sessionId = session.value?.session_id
    session.value = null
    deadline = 0
    remainingSeconds.value = 0
    if (sessionId) await options.client.cancel(sessionId).catch(() => undefined)
  }

  const createAccount = async (payload: Omit<DeviceCreateAccountRequest, 'session_id'>) => {
    if (!session.value?.session_id || !authorized.value) {
      throw new Error(`${options.provider} authorization is not complete`)
    }
    const result = await options.client.createAccount({ ...payload, session_id: session.value.session_id })
    clearTimer()
    return result
  }

  const reauthorizeAccount = async (accountId: number) => {
    if (!session.value?.session_id || !authorized.value) {
      throw new Error(`${options.provider} authorization is not complete`)
    }
    const result = await options.client.reauthorizeAccount(accountId, session.value.session_id)
    clearTimer()
    return result
  }

  const resetState = () => {
    clearTimer()
    generation += 1
    session.value = null
    error.value = ''
    loading.value = false
    deadline = 0
    remainingSeconds.value = 0
  }

  onBeforeUnmount(clearTimer)
  return {
    session,
    loading,
    error,
    authorized,
    remainingSeconds,
    start,
    poll,
    cancel,
    createAccount,
    reauthorizeAccount,
    resetState
  }
}
