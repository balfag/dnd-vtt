import { useCallback } from 'react'
import type { ChatMessage } from '../../shared/protocol'
import { gameStore, useGame } from '../game/store'

/** useHandoutQueue gives the handout whose window is open now (the first of the queue) and how to close it. */
export function useHandoutQueue(): {
  current: ChatMessage | null
  queue: ChatMessage[]
  close: () => void
} {
  const queue = useGame((s) => s.handoutQueue)
  const current = queue[0] ?? null
  const currentId = current?.id ?? null
  const close = useCallback(() => {
    if (currentId !== null) gameStore.getState().dismissHandout(currentId)
  }, [currentId])
  return { current, queue, close }
}

/** openHandout opens the window of a handout of the chat again. */
export function openHandout(message: ChatMessage): void {
  gameStore.getState().showHandout(message)
}
