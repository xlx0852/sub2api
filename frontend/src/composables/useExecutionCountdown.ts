import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

/**
 * Live countdown to the next scheduled execution.
 *
 * Both the diagnostics panel and the account usage drawer render the next run time, so the
 * ticking clock lives here instead of being duplicated. The clock only runs while `start()`
 * has been called and stops on `stop()`, so a closed view never leaves an interval behind.
 */
export interface ExecutionCountdownOptions {
  /** Deadlines to watch; the earliest one drives the due callback. */
  getTargets?: () => (string | null | undefined)[]
  /** Called once per distinct deadline, a few seconds after it passes. */
  onDue?: () => void
}

export function useExecutionCountdown(options: ExecutionCountdownOptions = {}) {
  const { t } = useI18n()
  const nowMs = ref(Date.now())

  let tickTimer: ReturnType<typeof setInterval> | null = null
  let dueTimer: ReturnType<typeof setTimeout> | null = null
  let dueHandledFor: string | null = null

  /** Remaining whole seconds until the target, or Infinity when there is none. */
  const countdownSeconds = (target?: string | null) => {
    if (!target) return Number.POSITIVE_INFINITY
    const targetMs = new Date(target).getTime()
    if (Number.isNaN(targetMs)) return Number.POSITIVE_INFINITY
    return Math.floor((targetMs - nowMs.value) / 1000)
  }

  /** "2分47秒后" / "3小时05分09秒后" / "正在执行…" — never a negative value. */
  const formatCountdown = (target?: string | null) => {
    const remaining = countdownSeconds(target)
    if (!Number.isFinite(remaining)) return '—'
    if (remaining <= 0) return t('admin.scheduledTests.countdownDue')
    const hours = Math.floor(remaining / 3600)
    const minutes = Math.floor((remaining % 3600) / 60)
    const seconds = remaining % 60
    if (hours > 0) {
      return t('admin.scheduledTests.countdownHours', {
        hours,
        minutes: String(minutes).padStart(2, '0'),
        seconds: String(seconds).padStart(2, '0')
      })
    }
    return t('admin.scheduledTests.countdownMinutes', {
      minutes,
      seconds: String(seconds).padStart(2, '0')
    })
  }

  const isDue = (target?: string | null) => countdownSeconds(target) <= 0

  /**
   * Fires `onDue` once per distinct due time, a few seconds after it passes, so the caller can
   * refresh and show the new result without the operator pressing refresh.
   */
  const notifyWhenDue = (targets: (string | null | undefined)[]) => {
    if (!options.onDue) return
    const nextDue = targets.filter((value): value is string => Boolean(value)).sort()[0]
    if (!nextDue || dueHandledFor === nextDue) return
    const remaining = countdownSeconds(nextDue)
    if (remaining > 0 || remaining < -5) return
    if (dueTimer) return
    dueHandledFor = nextDue
    dueTimer = setTimeout(() => {
      dueTimer = null
      options.onDue?.()
    }, 6000)
  }

  const start = () => {
    if (tickTimer) return
    tickTimer = setInterval(() => {
      nowMs.value = Date.now()
      // Re-check inside the tick: a deadline can pass while the view is already open.
      if (options.getTargets) {
        notifyWhenDue(options.getTargets())
      }
    }, 1000)
  }

  const stop = () => {
    if (tickTimer) {
      clearInterval(tickTimer)
      tickTimer = null
    }
    if (dueTimer) {
      clearTimeout(dueTimer)
      dueTimer = null
    }
  }

  /** Resets the "already refreshed for this deadline" marker, e.g. when data is reloaded. */
  const resetDueMarker = () => {
    dueHandledFor = null
  }

  return { nowMs, countdownSeconds, formatCountdown, isDue, notifyWhenDue, start, stop, resetDueMarker }
}
