import { ref, onMounted, onUnmounted } from 'vue'

/**
 * usePolling — 通用轮询 composable
 *
 * @param fn      轮询执行的异步函数
 * @param intervalMs 轮询间隔（毫秒）
 * @param immediate  是否立即执行（默认 true）
 * @returns { start, stop, isPolling }
 *
 * 自动在 onMounted 时启动，onUnmounted 时清理。
 */
export function usePolling(
  fn: () => Promise<void>,
  intervalMs: number,
  immediate = true,
) {
  const isPolling = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null

  async function tick() {
    try {
      await fn()
    } catch {
      // 轮询中静默吞掉错误，避免中断循环
    }
  }

  function start() {
    if (timer !== null) return
    isPolling.value = true
    if (immediate) {
      tick()
    }
    timer = setInterval(tick, intervalMs)
  }

  function stop() {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
    isPolling.value = false
  }

  onMounted(() => {
    start()
  })

  onUnmounted(() => {
    stop()
  })

  return { start, stop, isPolling }
}