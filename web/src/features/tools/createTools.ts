import type { PlayerInfo, SceneView } from '../../shared/protocol'
import type { GameConnection } from '../../shared/ws/connection'
import { CommandError } from '../../shared/ws/errors'
import type { Camera } from '../table/canvas/camera'
import { DrawTool } from './DrawTool'
import { RulerTool } from './RulerTool'
import { TemplateTool } from './TemplateTool'
import { DRAW_COLORS, type DrawOptions, type TemplateOptions, type ToolEnv } from './toolState'

interface Ref<T> {
  readonly current: T
}

/** ToolRefs are the things of the canvas the tools read at the moment of an event. */
export interface ToolRefs {
  scene: Ref<SceneView | null>
  camera: Ref<Camera>
  connection: Ref<GameConnection | null>
  players: Ref<PlayerInfo[]>
  me: Ref<{ playerId: string | null }>
  drawOptions: Ref<DrawOptions>
  templateOptions: Ref<TemplateOptions>
  announce: Ref<(text: string, visible: boolean) => void>
  t: Ref<(key: string, options?: Record<string, unknown>) => string>
  changed: Ref<() => void>
}

/** CanvasTools are the tools of everyone with the environment they share. */
export interface CanvasTools {
  env: ToolEnv
  ruler: RulerTool
  draw: DrawTool
  template: TemplateTool
}

/** createTools makes the tools; they read the refs when something happens, never while a component renders. */
export function createTools(refs: ToolRefs): CanvasTools {
  const env: ToolEnv = {
    scene: () => refs.scene.current,
    zoom: () => refs.camera.current.zoom,
    drawOptions: () => refs.drawOptions.current,
    templateOptions: () => refs.templateOptions.current,
    myColor: () =>
      refs.players.current.find((p) => p.id === refs.me.current.playerId)?.color ?? DRAW_COLORS[0],
    transient: (type, data) => {
      refs.connection.current?.transient(type, data)
    },
    command: (type, data) =>
      refs.connection.current?.command(type, data) ??
      Promise.reject(new CommandError('disconnected')),
    announce: (text, visible) => {
      refs.announce.current(text, visible)
    },
    t: (key, options) => refs.t.current(key, options),
    changed: () => {
      refs.changed.current()
    },
  }
  return {
    env,
    ruler: new RulerTool(env),
    draw: new DrawTool(env),
    template: new TemplateTool(env),
  }
}
