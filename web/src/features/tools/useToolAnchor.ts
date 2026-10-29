import { useLayoutEffect, type RefObject } from 'react'

/**
 * useToolAnchor puts a panel beside the button of its tool: the panel's top edge is level with
 * the button, and the panel never starts below the middle of the canvas.
 */
export function useToolAnchor(ref: RefObject<HTMLElement | null>, tool: string): void {
  useLayoutEffect(() => {
    const place = () => {
      const el = ref.current
      const parent = el?.offsetParent
      const button = document.querySelector(`[data-tool="${tool}"]`)
      if (el == null || parent == null || button === null) return
      const top = button.getBoundingClientRect().top - parent.getBoundingClientRect().top
      el.style.setProperty('--anchor-top', `${String(Math.max(0, Math.round(top)))}px`)
    }
    place()
    window.addEventListener('resize', place)
    return () => {
      window.removeEventListener('resize', place)
    }
  }, [ref, tool])
}
