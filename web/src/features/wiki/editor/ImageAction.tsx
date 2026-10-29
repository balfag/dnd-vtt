import { useEffect, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'

export interface ShownImage {
  assetId: string
  alt: string
}

interface ImageActionProps {
  /** container is the element the editor and this button live in; it is the positioning parent. */
  container: RefObject<HTMLElement | null>
  onShow: (image: ShownImage) => void
}

interface Hovered extends ShownImage {
  top: number
  right: number
}

/** ImageAction is the "show to players" button over the corner of the image the pointer is on. */
export function ImageAction({ container, onShow }: ImageActionProps) {
  const { t } = useTranslation()
  const [hovered, setHovered] = useState<Hovered | null>(null)

  useEffect(() => {
    const box = container.current
    if (box === null) return
    const over = (e: MouseEvent) => {
      const img = e.target instanceof Element ? e.target.closest('img[data-asset-id]') : null
      if (img === null) return
      const at = img.getBoundingClientRect()
      const parent = box.getBoundingClientRect()
      setHovered({
        assetId: img.getAttribute('data-asset-id') ?? '',
        alt: img.getAttribute('alt') ?? '',
        top: at.top - parent.top + 8,
        right: parent.right - at.right + 8,
      })
    }
    const leave = () => {
      setHovered(null)
    }
    box.addEventListener('mouseover', over)
    box.addEventListener('mouseleave', leave)
    return () => {
      box.removeEventListener('mouseover', over)
      box.removeEventListener('mouseleave', leave)
    }
  }, [container])

  if (hovered === null) return null
  return (
    <button
      type="button"
      className="wiki-imageaction"
      style={{ top: hovered.top, right: hovered.right }}
      onClick={() => {
        onShow({ assetId: hovered.assetId, alt: hovered.alt })
      }}
    >
      {t('handout.show.button')}
    </button>
  )
}
