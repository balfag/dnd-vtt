import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { PlayerInfo, SceneView, Token } from '../../../shared/protocol'
import { MessageType } from '../../../shared/protocol'
import type { GameConnection } from '../../../shared/ws/connection'
import { gameStore } from '../../game/store'
import { createMoves } from '../moves'
import { ZoomControls } from '../ZoomControls'
import { canvasBridge } from './canvasBridge'
import { centerOn, fit, screenToWorld, type Camera, type Point } from './camera'
import { canMove, endDrag, moveDrag, startDrag, type DragState } from './drag'
import { cellDistanceFt } from './grid'
import {
  cycleToken,
  noGesture,
  onKey,
  onWheel,
  pointerDown,
  pointerMove,
  pointerUp,
  stepPosition,
  zoomCenter,
  ZOOM_STEP,
  type Gesture,
} from './input'
import { SceneRenderer } from './SceneRenderer'
import type { TokenPlacement } from './TokenLayer'

const startCamera: Camera = { x: 0, y: 0, zoom: 1 }
// A press that moves less than this (screen px) is a click.
const clickSlopPx = 4
// If the server answers a move but sends no token.changed (nothing changed), the pending move is dropped after this.
const confirmFallbackMs = 1000
const pruneEveryMs = 500

export interface CanvasMe {
  isGM: boolean
  playerId: string | null
}

interface SceneCanvasProps {
  scene: SceneView | null
  emptyText: string
  players: PlayerInfo[]
  me: CanvasMe
  connection: GameConnection | null
  /** announce says something to the viewer; visible also shows a note for a few seconds. */
  announce: (text: string, visible: boolean) => void
}

function typesText(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.tagName === 'SELECT')
  )
}

function samePoint(a: Point, b: Point): boolean {
  return a.x === b.x && a.y === b.y
}

/**
 * SceneCanvas hosts the PixiJS renderer, owns the camera and turns input into camera moves
 * and token moves. emptyText labels the canvas while there is no scene.
 */
export function SceneCanvas({
  scene,
  emptyText,
  players,
  me,
  connection,
  announce,
}: SceneCanvasProps) {
  const { t } = useTranslation()
  const hostRef = useRef<HTMLDivElement>(null)
  const [renderer, setRenderer] = useState<SceneRenderer | null>(null)
  const [zoom, setZoom] = useState(startCamera.zoom)
  const cameraRef = useRef<Camera>(startCamera)
  const sceneRef = useRef<SceneView | null>(scene)
  const fittedFor = useRef<string | null>(null)

  // Things the handlers read at the moment of the event, kept out of the effect dependencies.
  const playersRef = useRef(players)
  const meRef = useRef(me)
  const connectionRef = useRef(connection)
  const announceRef = useRef(announce)
  const tRef = useRef(t)
  useEffect(() => {
    playersRef.current = players
    meRef.current = me
    connectionRef.current = connection
    announceRef.current = announce
    tRef.current = t
  })

  // The interaction state of the canvas; it never causes a React render, only a redraw.
  const moves = useRef(createMoves())
  const pending = useRef(new Map<string, { token: Token; seq: number }>())
  const selectedId = useRef<string | null>(null)
  const hoverId = useRef<string | null>(null)
  const drag = useRef<DragState | null>(null)
  const snap = useRef(true)
  const pointer = useRef<Point>({ x: 0, y: 0 })

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

  /** refresh hands the renderer what to draw: tokens where they are shown now, selection, drag. */
  const refresh = useCallback(() => {
    if (renderer === null) return
    const sc = sceneRef.current
    const placements = new Map<string, TokenPlacement>()
    const d = drag.current
    const dragging = gameStore.getState().dragging
    let ghost: { token: Token; x: number; y: number } | null = null
    let distance: { at: Point; text: string } | null = null
    if (sc !== null) {
      const bounds = { width: sc.width, height: sc.height }
      for (const tk of sc.tokens) {
        const server = { x: tk.x, y: tk.y }
        if (d?.tokenId === tk.id) {
          const at = endDrag(d, sc.grid, tk, snap.current, bounds)
          placements.set(tk.id, { ...at, smooth: false })
          if (!samePoint(at, d.origin)) {
            ghost = { token: tk, ...d.origin }
            distance = {
              at: pointer.current,
              text: tRef.current('table.token.distance', {
                ft: cellDistanceFt(sc.grid, d.origin, at),
              }),
            }
          }
          continue
        }
        const own = moves.current.position(tk)
        if (!samePoint(own, server)) {
          placements.set(tk.id, { ...own, smooth: false })
          continue
        }
        const remote = dragging[tk.id]
        placements.set(tk.id, { ...(remote ?? server), smooth: true })
      }
    }
    const combat = gameStore.getState().game?.combat ?? null
    const turn = combat?.combatants.find((c) => c.id === combat.turnCombatantId)
    renderer.setTokens({
      scene: sc,
      players: playersRef.current,
      isGM: meRef.current.isGM,
      placements,
      selectedId: selectedId.current,
      turnTokenId: turn?.tokenId ?? null,
      hoverId: hoverId.current,
      ghost,
      distance,
      reducedMotion: window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    })
  }, [renderer])

  useEffect(() => {
    sceneRef.current = scene
    if (renderer === null) return
    renderer.setScene(scene)
    if (scene === null) {
      fittedFor.current = null
    } else if (fittedFor.current !== scene.id) {
      fittedFor.current = scene.id
      apply(fit(scene, renderer.viewport()))
    } else {
      renderer.setCamera(cameraRef.current)
    }
    if (selectedId.current !== null && !scene?.tokens.some((tk) => tk.id === selectedId.current)) {
      selectedId.current = null
      canvasBridge.getState().setSelected(null)
    }
    refresh()
  }, [renderer, scene, apply, refresh])

  // The selection belongs to the canvas: nothing stays selected for the panels when it is gone.
  useEffect(() => {
    return () => {
      canvasBridge.getState().setSelected(null)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [players, me.isGM, me.playerId, refresh])

  // Drags of other players change faster than the scene does: follow the store directly.
  useEffect(() => {
    if (renderer === null) return
    const unsubscribe = gameStore.subscribe((state, prev) => {
      const next = state.game?.scene ?? null
      if (next?.tokens !== prev.game?.scene?.tokens) {
        for (const [id, p] of pending.current) {
          const now = next?.tokens.find((tk) => tk.id === id)
          if (now === p.token) continue
          // token.changed or token.removed arrived: the server position is the truth again.
          moves.current.confirm(id)
          pending.current.delete(id)
        }
      }
      if (next !== null && state.dragging !== prev.dragging) refresh()
      if (state.game?.combat !== prev.game?.combat) refresh()
    })
    const timer = setInterval(() => {
      gameStore.getState().pruneDragging(Date.now())
    }, pruneEveryMs)
    return () => {
      unsubscribe()
      clearInterval(timer)
    }
  }, [renderer, refresh])

  useEffect(() => {
    const host = hostRef.current
    if (host === null || renderer === null) return
    let gesture: Gesture = noGesture
    let dragPointer: number | null = null
    let tap: { id: number; at: Point } | null = null
    let spaceHeld = false

    function local(e: { clientX: number; clientY: number }): Point {
      const rect = host?.getBoundingClientRect()
      return { x: e.clientX - (rect?.left ?? 0), y: e.clientY - (rect?.top ?? 0) }
    }
    function setPanning(on: boolean) {
      if (on) host?.setAttribute('data-panning', '')
      else host?.removeAttribute('data-panning')
    }
    function setCursor(kind: 'grab' | 'grabbing' | null) {
      if (kind === null) host?.removeAttribute('data-cursor')
      else host?.setAttribute('data-cursor', kind)
    }
    function tokenById(id: string | null): Token | null {
      return sceneRef.current?.tokens.find((tk) => tk.id === id) ?? null
    }
    function select(id: string | null) {
      if (selectedId.current === id) return
      selectedId.current = id
      canvasBridge.getState().setSelected(id)
      refresh()
    }
    function cancelDrag() {
      if (drag.current === null) return
      drag.current = null
      dragPointer = null
      setCursor(null)
      refresh()
    }

    /** commitMove shows the move at once and asks the server; a refusal puts the token back. */
    function commitMove(tk: Token, to: Point) {
      const seq = (pending.current.get(tk.id)?.seq ?? 0) + 1
      moves.current.start(tk.id, to)
      pending.current.set(tk.id, { token: tk, seq })
      const settle = () => {
        if (pending.current.get(tk.id)?.seq !== seq) return
        moves.current.confirm(tk.id)
        pending.current.delete(tk.id)
        refresh()
      }
      const conn = connectionRef.current
      const sent =
        conn === null
          ? Promise.reject(new Error('no connection'))
          : conn.command(MessageType.TokenMove, { tokenId: tk.id, x: to.x, y: to.y })
      void sent.then(
        () => {
          setTimeout(settle, confirmFallbackMs)
        },
        () => {
          if (pending.current.get(tk.id)?.seq !== seq) return
          moves.current.fail(tk.id)
          pending.current.delete(tk.id)
          announceRef.current(tRef.current('table.token.moveFailed'), true)
          refresh()
        },
      )
      refresh()
    }

    const hoverAt = (p: Point) => {
      const id = renderer.tokenAt(p)
      const tk = tokenById(id)
      setCursor(tk !== null && canMove(tk, meRef.current) ? 'grab' : null)
      if (id !== hoverId.current) {
        hoverId.current = id
        refresh()
      }
    }

    const wheel = (e: WheelEvent) => {
      e.preventDefault()
      apply(onWheel(cameraRef.current, { deltaY: e.deltaY, ctrlKey: e.ctrlKey, point: local(e) }))
    }
    const down = (e: PointerEvent) => {
      if (e.pointerType === 'mouse' && e.button !== 0 && e.button !== 1) return
      host.setPointerCapture(e.pointerId)
      const p = local(e)
      pointer.current = p
      if (drag.current !== null) {
        // A second finger: the token stays where it was, the fingers pan and zoom the map.
        cancelDrag()
      } else if (e.button === 0 && !spaceHeld && gesture.pointers.size === 0) {
        const id = renderer.tokenAt(p)
        const tk = tokenById(id)
        if (tk !== null) {
          select(tk.id)
          if (canMove(tk, meRef.current)) {
            drag.current = startDrag(tk, screenToWorld(cameraRef.current, p), performance.now())
            dragPointer = e.pointerId
            snap.current = !e.altKey
            setCursor('grabbing')
            refresh()
          }
          // A token the viewer cannot move is just selected: dragging it does not pan the map.
          return
        } else {
          tap = { id: e.pointerId, at: p }
        }
      }
      gesture = pointerDown(gesture, e.pointerId, p)
      setPanning(true)
    }
    const move = (e: PointerEvent) => {
      const p = local(e)
      pointer.current = p
      const sc = sceneRef.current
      const d = drag.current
      if (d !== null && e.pointerId === dragPointer && sc !== null) {
        snap.current = !e.altKey
        const tk = tokenById(d.tokenId)
        if (tk === null) {
          cancelDrag()
          return
        }
        const r = moveDrag(
          d,
          screenToWorld(cameraRef.current, p),
          performance.now(),
          sc.grid,
          tk,
          snap.current,
          { width: sc.width, height: sc.height },
        )
        drag.current = r.state
        if (r.send !== null) {
          connectionRef.current?.transient(MessageType.TokenDrag, {
            tokenId: d.tokenId,
            x: r.send.x,
            y: r.send.y,
          })
        }
        refresh()
        return
      }
      if (tap?.id === e.pointerId && Math.hypot(p.x - tap.at.x, p.y - tap.at.y) > clickSlopPx) {
        tap = null
      }
      const r = pointerMove(gesture, cameraRef.current, e.pointerId, p)
      gesture = r.gesture
      if (r.camera !== cameraRef.current) apply(r.camera)
      if (gesture.pointers.size === 0 && e.buttons === 0) hoverAt(p)
    }
    const up = (e: PointerEvent) => {
      const sc = sceneRef.current
      const d = drag.current
      if (d !== null && e.pointerId === dragPointer && sc !== null) {
        const tk = tokenById(d.tokenId)
        drag.current = null
        dragPointer = null
        if (tk !== null) {
          const to = endDrag(d, sc.grid, tk, !e.altKey, { width: sc.width, height: sc.height })
          // A click without a move is just a selection.
          if (!samePoint(to, d.origin)) commitMove(tk, to)
        }
        hoverAt(local(e))
        refresh()
        return
      }
      if (tap?.id === e.pointerId) {
        tap = null
        if (e.type === 'pointerup') select(null)
      }
      gesture = pointerUp(gesture, e.pointerId)
      if (gesture.pointers.size === 0) setPanning(false)
    }
    const leave = () => {
      if (hoverId.current === null || drag.current !== null) return
      hoverId.current = null
      setCursor(null)
      refresh()
    }
    // The middle button would start browser autoscroll.
    const mouseDown = (e: MouseEvent) => {
      if (e.button === 1) e.preventDefault()
    }
    const altChanged = (e: KeyboardEvent) => {
      if (e.key !== 'Alt' || drag.current === null) return
      snap.current = e.type === 'keyup'
      refresh()
    }
    const key = (e: KeyboardEvent) => {
      altChanged(e)
      if (typesText(e.target)) return
      const focused = document.activeElement === host
      if (e.key === ' ' && focused) {
        spaceHeld = true
        e.preventDefault()
        return
      }
      if (e.key === 'Escape') {
        cancelDrag()
        select(null)
        return
      }
      if (e.ctrlKey || e.metaKey || e.altKey) return
      const sc = sceneRef.current
      if (focused && sc !== null && e.key.toLowerCase() === 'n') {
        const id = cycleToken(sc.tokens, selectedId.current, e.shiftKey, meRef.current)
        if (id === null) return
        e.preventDefault()
        select(id)
        const name = tokenById(id)?.name ?? ''
        announceRef.current(
          name === ''
            ? tRef.current('table.token.unnamed')
            : tRef.current('table.token.selected', { name }),
          false,
        )
        return
      }
      const selected = tokenById(selectedId.current)
      if (focused && sc !== null && selected !== null && canMove(selected, meRef.current)) {
        // Repeated arrows step from where the token is shown, not from the last server position.
        const shown = { ...selected, ...moves.current.position(selected) }
        const to = stepPosition(shown, e.key, sc.grid, { width: sc.width, height: sc.height })
        if (to !== null) {
          e.preventDefault()
          commitMove(selected, to)
          return
        }
        if (e.key.startsWith('Arrow')) {
          e.preventDefault()
          return
        }
      }
      const next = onKey(cameraRef.current, e.key, renderer.viewport(), sc)
      if (next === null) return
      e.preventDefault()
      apply(next)
    }
    const keyUp = (e: KeyboardEvent) => {
      altChanged(e)
      if (e.key === ' ') spaceHeld = false
    }

    // A panel asks to look at a token: select it and bring it to the middle, if it is on the scene.
    let lastFocus = canvasBridge.getState().focus?.n ?? 0
    const unsubscribeFocus = canvasBridge.subscribe((state) => {
      const request = state.focus
      if (request === null || request.n === lastFocus) return
      lastFocus = request.n
      const tk = tokenById(request.tokenId)
      if (tk === null) return
      select(tk.id)
      const at = moves.current.position(tk)
      apply(centerOn(cameraRef.current, at, renderer.viewport()))
    })

    host.addEventListener('wheel', wheel, { passive: false })
    host.addEventListener('pointerdown', down)
    host.addEventListener('pointermove', move)
    host.addEventListener('pointerup', up)
    host.addEventListener('pointercancel', up)
    host.addEventListener('pointerleave', leave)
    host.addEventListener('mousedown', mouseDown)
    document.addEventListener('keydown', key)
    document.addEventListener('keyup', keyUp)
    return () => {
      unsubscribeFocus()
      host.removeEventListener('wheel', wheel)
      host.removeEventListener('pointerdown', down)
      host.removeEventListener('pointermove', move)
      host.removeEventListener('pointerup', up)
      host.removeEventListener('pointercancel', up)
      host.removeEventListener('pointerleave', leave)
      host.removeEventListener('mousedown', mouseDown)
      document.removeEventListener('keydown', key)
      document.removeEventListener('keyup', keyUp)
      drag.current = null
    }
  }, [renderer, apply, refresh])

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
