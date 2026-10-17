import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { SceneView } from '../../../shared/protocol'
import { ZoomControls } from '../ZoomControls'
import { fit, type Camera } from './camera'
import {
  noGesture,
  onKey,
  onWheel,
  pointerDown,
  pointerMove,
  pointerUp,
  zoomCenter,
  ZOOM_STEP,
  type Gesture,
} from './input'
import { SceneRenderer } from './SceneRenderer'

const startCamera: Camera = { x: 0, y: 0, zoom: 1 }

function typesText(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.tagName === 'SELECT')
  )
}

/**
 * SceneCanvas hosts the PixiJS renderer, owns the camera and turns input into camera moves.
 * emptyText labels the canvas while there is no scene.
 */
export function SceneCanvas({ scene, emptyText }: { scene: SceneView | null; emptyText: string }) {
  const { t } = useTranslation()
  const hostRef = useRef<HTMLDivElement>(null)
  const [renderer, setRenderer] = useState<SceneRenderer | null>(null)
  const [zoom, setZoom] = useState(startCamera.zoom)
  const cameraRef = useRef<Camera>(startCamera)
  const sceneRef = useRef<SceneView | null>(scene)
  const fittedFor = useRef<string | null>(null)

  useEffect(() => {
    const host = hostRef.current
    if (host === null) return
    // PixiJS initialises asynchronously and StrictMode runs the effect twice:
    // a renderer that finishes after the cleanup is destroyed at once.
    let cancelled = false
    let created: SceneRenderer | null = null
    SceneRenderer.create(host).then(
      (r) => {
        if (cancelled) {
          r.destroy()
          return
        }
        created = r
        setRenderer(r)
      },
      (err: unknown) => {
        console.error('table: cannot start the canvas', err)
      },
    )
    return () => {
      cancelled = true
      created?.destroy()
      setRenderer(null)
    }
  }, [])

  const apply = useCallback(
    (next: Camera) => {
      cameraRef.current = next
      setZoom(next.zoom)
      renderer?.setCamera(next)
    },
    [renderer],
  )

  useEffect(() => {
    sceneRef.current = scene
    if (renderer === null) return
    renderer.setScene(scene)
    if (scene === null) {
      fittedFor.current = null
      return
    }
    if (fittedFor.current !== scene.id) {
      fittedFor.current = scene.id
      apply(fit(scene, renderer.viewport()))
    } else {
      renderer.setCamera(cameraRef.current)
    }
  }, [renderer, scene, apply])

  useEffect(() => {
    const host = hostRef.current
    if (host === null || renderer === null) return
    let gesture: Gesture = noGesture

    function local(e: { clientX: number; clientY: number }): { x: number; y: number } {
      const rect = host?.getBoundingClientRect()
      return { x: e.clientX - (rect?.left ?? 0), y: e.clientY - (rect?.top ?? 0) }
    }
    function setPanning(on: boolean) {
      if (on) host?.setAttribute('data-panning', '')
      else host?.removeAttribute('data-panning')
    }

    const wheel = (e: WheelEvent) => {
      e.preventDefault()
      apply(onWheel(cameraRef.current, { deltaY: e.deltaY, ctrlKey: e.ctrlKey, point: local(e) }))
    }
    const down = (e: PointerEvent) => {
      if (e.pointerType === 'mouse' && e.button !== 0 && e.button !== 1) return
      host.setPointerCapture(e.pointerId)
      gesture = pointerDown(gesture, e.pointerId, local(e))
      setPanning(true)
    }
    const move = (e: PointerEvent) => {
      const r = pointerMove(gesture, cameraRef.current, e.pointerId, local(e))
      gesture = r.gesture
      if (r.camera !== cameraRef.current) apply(r.camera)
    }
    const up = (e: PointerEvent) => {
      gesture = pointerUp(gesture, e.pointerId)
      if (gesture.pointers.size === 0) setPanning(false)
    }
    // The middle button would start browser autoscroll.
    const mouseDown = (e: MouseEvent) => {
      if (e.button === 1) e.preventDefault()
    }
    const key = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || typesText(e.target)) return
      const next = onKey(cameraRef.current, e.key, renderer.viewport(), sceneRef.current)
      if (next === null) return
      e.preventDefault()
      apply(next)
    }

    host.addEventListener('wheel', wheel, { passive: false })
    host.addEventListener('pointerdown', down)
    host.addEventListener('pointermove', move)
    host.addEventListener('pointerup', up)
    host.addEventListener('pointercancel', up)
    host.addEventListener('mousedown', mouseDown)
    document.addEventListener('keydown', key)
    return () => {
      host.removeEventListener('wheel', wheel)
      host.removeEventListener('pointerdown', down)
      host.removeEventListener('pointermove', move)
      host.removeEventListener('pointerup', up)
      host.removeEventListener('pointercancel', up)
      host.removeEventListener('mousedown', mouseDown)
      document.removeEventListener('keydown', key)
    }
  }, [renderer, apply])

  function step(factor: number) {
    if (renderer !== null) apply(zoomCenter(cameraRef.current, renderer.viewport(), factor))
  }

  return (
    <div className="canvas">
      <div
        ref={hostRef}
        className="canvas__host"
        role="application"
        aria-label={scene === null ? emptyText : t('table.canvas.label', { name: scene.name })}
        tabIndex={0}
      />
      <ZoomControls
        zoom={zoom}
        onZoomIn={() => {
          step(ZOOM_STEP)
        }}
        onZoomOut={() => {
          step(1 / ZOOM_STEP)
        }}
        onFit={() => {
          if (renderer !== null && scene !== null) apply(fit(scene, renderer.viewport()))
        }}
      />
    </div>
  )
}
