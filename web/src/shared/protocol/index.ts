// WebSocket protocol of docs/architecture/protocol.md. These types mirror
// internal/ws/protocol (Go); golden.test.ts checks both sides against the
// shared examples in internal/ws/protocol/testdata.

export const MessageType = {
  Hello: 'sync.hello',
  Snapshot: 'sync.snapshot',
  Presence: 'presence.update',
  Noop: 'sync.noop',
  SceneCreate: 'scene.create',
  SceneUpdate: 'scene.update',
  SceneDelete: 'scene.delete',
  SceneReorder: 'scene.reorder',
  SceneActivate: 'scene.activate',
  SceneView: 'scene.view',
  ScenesChanged: 'scenes.changed',
  SceneChanged: 'scene.changed',
  SceneActivated: 'scene.activated',
  TokenCreate: 'token.create',
  TokenUpdate: 'token.update',
  TokenMove: 'token.move',
  TokenDrag: 'token.drag',
  TokenDelete: 'token.delete',
  TokenChanged: 'token.changed',
  TokenRemoved: 'token.removed',
  TokenDragging: 'token.dragging',
  FogSet: 'fog.set',
  FogReveal: 'fog.reveal',
  FogHide: 'fog.hide',
  FogReset: 'fog.reset',
  FogChanged: 'fog.changed',
  DrawingCreate: 'drawing.create',
  DrawingUpdate: 'drawing.update',
  DrawingDelete: 'drawing.delete',
  DrawingChanged: 'drawing.changed',
  DrawingRemoved: 'drawing.removed',
  PingPlace: 'ping.place',
  PingShown: 'ping.shown',
  RulerUpdate: 'ruler.update',
  RulerClear: 'ruler.clear',
  RulerShown: 'ruler.shown',
  ChatSend: 'chat.send',
  RollRequest: 'roll.request',
  ChatHistory: 'chat.history',
  ChatMessage: 'chat.message',
  HandoutShow: 'handout.show',
  CombatStart: 'combat.start',
  CombatEnd: 'combat.end',
  CombatAdd: 'combat.add',
  CombatRemove: 'combat.remove',
  CombatSetInitiative: 'combat.setInitiative',
  CombatRollInitiative: 'combat.rollInitiative',
  CombatSetHidden: 'combat.setHidden',
  CombatNext: 'combat.next',
  CombatPrev: 'combat.prev',
  CombatChanged: 'combat.changed',
  Ping: 'ping',
  Pong: 'pong',
  Ack: 'ack',
  Error: 'error',
} as const

export const ErrorCode = {
  BadRequest: 'bad_request',
  Forbidden: 'forbidden',
  NotFound: 'not_found',
  Conflict: 'conflict',
  RateLimited: 'rate_limited',
  Internal: 'internal',
} as const
export type ErrorCode = (typeof ErrorCode)[keyof typeof ErrorCode]

export const CloseCode = {
  SessionRevoked: 4001,
  PlayerRemoved: 4003,
  GameEnded: 4004,
  Idle: 4008,
  RateLimited: 4029,
} as const

export interface Envelope {
  t: string
  id?: string
  re?: string
  seq?: number
  d?: unknown
}
export interface Hello {
  gameSessionId?: string
  lastSeq?: number
}
export interface HelloAck {
  resumed: boolean
}
export interface ErrorData {
  code: ErrorCode
  message: string
}
export interface PlayerInfo {
  id: string
  displayName: string
  color: string
  role: 'gm' | 'player'
  online: boolean
}
export interface SnapshotMe {
  accountId: string
  isGm: boolean
  playerId: string | null
}
export interface Grid {
  type: 'square' | 'hexPointy' | 'hexFlat'
  sizePx: number
  unitFt: number
  offsetX: number
  offsetY: number
  color: string
  visible: boolean
}
export interface AssetRef {
  assetId: string
  url: string
}
/** Shape is an area of the scene in scene pixels; the same limits as on the server. */
export type Shape =
  | { type: 'rect'; x: number; y: number; w: number; h: number }
  | { type: 'circle'; x: number; y: number; r: number }
  | { type: 'polygon'; points: [number, number][] }
export interface FogOp {
  op: 'reveal' | 'hide'
  shape: Shape
}
export interface Fog {
  enabled: boolean
  /** In the order of application: the last op whose shape holds a point decides. */
  ops: FogOp[]
}
export interface TokenBar {
  key: string
  value: number
  max: number
  visibility: 'all' | 'owners' | 'gm'
}
export interface Token {
  id: string
  sceneId: string
  layer: 'map' | 'tokens' | 'gm'
  x: number
  y: number
  w: number
  h: number
  rotation: number
  z: number
  name: string
  nameVisibility: 'all' | 'hover' | 'gm'
  image: AssetRef | null
  hidden: boolean
  locked: boolean
  owners: string[]
  bars: TokenBar[]
  conditions: string[]
}
export type DrawingKind = 'freehand' | 'rect' | 'circle' | 'text' | 'template'
export type TemplateShape = 'cone' | 'sphere' | 'line' | 'cube' | 'cylinder'
/** DrawingDataByKind is the data of a drawing by its kind; coordinates are finite, within ±100000. */
export interface DrawingDataByKind {
  /** 2–2000 points. */
  freehand: { points: [number, number][] }
  /** The top left corner; w, h > 0. */
  rect: { x: number; y: number; w: number; h: number }
  /** The center; r > 0. */
  circle: { x: number; y: number; r: number }
  /** text is 1–500 characters, size 8–200 px. */
  text: { x: number; y: number; text: string; size: number }
  /**
   * x, y is the origin (cone, line) or the center; size is 5–1000 ft (the length of a
   * cone and a line, the radius of a sphere and a cylinder, the edge of a cube);
   * direction is in degrees clockwise from the X axis, [0, 360) as the server sends it.
   */
  template: { shape: TemplateShape; x: number; y: number; size: number; direction: number }
}
/** DrawingContent is the kind of a drawing with the data that goes with it. */
export type DrawingContent = {
  [K in DrawingKind]: { kind: K; data: DrawingDataByKind[K] }
}[DrawingKind]
/** Drawing is a drawing or an area template of a scene. */
export type Drawing = DrawingContent & {
  id: string
  sceneId: string
  color: string
  authorPlayerId: string
}
/** SceneView is a scene as the client sees it. */
export interface SceneView {
  id: string
  name: string
  width: number
  height: number
  grid: Grid
  background: AssetRef | null
  fog: Fog
  /** Only the tokens the viewer may see. */
  tokens: Token[]
  /** All the drawings of the scene, by id. */
  drawings: Drawing[]
}
/** SceneInfo is a scene without its tokens and drawings: scene.changed has neither, the client keeps its own. */
export type SceneInfo = Omit<SceneView, 'tokens' | 'drawings'>
export interface SceneListItem {
  id: string
  name: string
  sort: number
}
/** DiceDie is one rolled die. */
export interface DiceDie {
  value: number
  /** Removed by kh/kl/dh/dl. */
  dropped?: boolean
  /** The original value when the die was rerolled. */
  rerolledFrom?: number
  /** Added by an explosion. */
  exploded?: boolean
}
export interface DiceGroup {
  /** Dice before explosions. */
  count: number
  sides: number
  dice: DiceDie[]
}
/** DiceTerm is one summand: a constant, or a rolled dice group. */
export interface DiceTerm {
  sign: 1 | -1
  /** The constant, or the sum of the kept dice. */
  value: number
  dice?: DiceGroup
}
export interface DiceResult {
  /** Canonical. */
  formula: string
  total: number
  terms: DiceTerm[]
}
export type RollVisibility = 'public' | 'gm' | 'self' | 'blind'
export interface ChatRoll {
  /** Canonical. */
  formula: string
  label: string
  /** Absent for the author of a blind roll. */
  result?: DiceResult
}
/** ChatMessage is the data of chat.message and an item of the snapshot and of the history. */
export interface ChatMessage {
  id: string
  createdAt: number
  /** null for system messages. */
  authorPlayerId: string | null
  kind: 'text' | 'roll' | 'system' | 'handout'
  visibility: 'public' | 'private' | RollVisibility
  /** Private messages only. */
  to?: string[]
  /** Text and system messages. */
  text?: string
  /** Roll messages only. */
  roll?: ChatRoll
  /** Handout messages only. */
  handout?: ChatHandout
}
/** ChatHandout is the payload of a handout message: a normalized wiki document fragment. */
export interface ChatHandout {
  title: string
  content: unknown
}
/** HandoutShow is the data of handout.show; to makes the handout private. */
export interface HandoutShow {
  title: string
  content: unknown
  to?: string[]
}
/** Combatant is a participant of the tracker; name is empty when the viewer may not see it. */
export interface Combatant {
  id: string
  tokenId: string
  name: string
  /** null: not set. */
  initiative: number | null
  hidden: boolean
}
/** Combat is the initiative tracker of a scene as the viewer sees it. */
export interface Combat {
  sceneId: string
  round: number
  /** null: before the first combat.next, or the turn is of a combatant hidden from the viewer. */
  turnCombatantId: string | null
  /** In turn order. */
  combatants: Combatant[]
}
export interface SnapshotGame {
  gameSessionId: string
  startedAt: number
  campaign: { id: string; name: string }
  players: PlayerInfo[]
  activeSceneId: string | null
  /** The GM gets every scene; a player gets []. */
  scenes: SceneListItem[]
  scene: SceneView | null
  /** The last 50 messages visible to the viewer, ascending. */
  chat: ChatMessage[]
  /** The tracker of the active scene for the viewer; null: none. */
  combat: Combat | null
}
export interface Snapshot {
  seq: number
  me: SnapshotMe
  game: SnapshotGame | null
}
export interface Presence {
  playerId: string
  online: boolean
  player?: PlayerInfo
}

// Messages the server sends.
export type HelloAckMessage = { t: 'ack'; re: string; d: HelloAck }
export type SnapshotMessage = { t: 'sync.snapshot'; d: Snapshot }
export type PresenceMessage = { t: 'presence.update'; seq: number; d: Presence }
/** NoopMessage says that the event number is taken and there is nothing for this recipient. */
export type NoopMessage = { t: 'sync.noop'; seq: number }
export type ScenesChangedMessage = {
  t: 'scenes.changed'
  seq: number
  d: { scenes: SceneListItem[] }
}
export type SceneChangedMessage = { t: 'scene.changed'; seq: number; d: { scene: SceneInfo } }
export type SceneActivatedMessage = { t: 'scene.activated'; seq: number; d: { scene: SceneView } }
/** FogChangedMessage: tokens is there for players only, the whole list they see now. */
export type FogChangedMessage = {
  t: 'fog.changed'
  seq: number
  d: { sceneId: string; fog: Fog; tokens?: Token[] }
}
export type TokenChangedMessage = { t: 'token.changed'; seq: number; d: { token: Token } }
export type TokenRemovedMessage = { t: 'token.removed'; seq: number; d: { tokenId: string } }
/** TokenDraggingMessage is transient: it has no seq. */
export type TokenDraggingMessage = {
  t: 'token.dragging'
  d: { tokenId: string; x: number; y: number }
}
export type DrawingChangedMessage = { t: 'drawing.changed'; seq: number; d: { drawing: Drawing } }
export type DrawingRemovedMessage = { t: 'drawing.removed'; seq: number; d: { drawingId: string } }
export type DrawingCreateAckMessage = { t: 'ack'; re: string; d: { drawingId: string } }
/** PingShownMessage is transient: it has no seq. focus asks the client to move the camera. */
export type PingShownMessage = {
  t: 'ping.shown'
  d: { playerId: string; sceneId: string; x: number; y: number; focus: boolean }
}
/** RulerShownMessage is transient: it has no seq. points is empty when the ruler is cleared. */
export type RulerShownMessage = {
  t: 'ruler.shown'
  d: { playerId: string; sceneId: string; points: [number, number][] }
}
export type TokenCreateAckMessage = { t: 'ack'; re: string; d: { tokenId: string } }
export type SceneViewAckMessage = { t: 'ack'; re: string; d: { scene: SceneView } }
export type ChatMessageEvent = { t: 'chat.message'; seq: number; d: ChatMessage }
export type ChatHistoryAckMessage = { t: 'ack'; re: string; d: { messages: ChatMessage[] } }
/** CombatChangedMessage carries the whole tracker after a change; combat is null when the fight is over. */
export type CombatChangedMessage = {
  t: 'combat.changed'
  seq: number
  d: { sceneId: string; combat: Combat | null }
}
export type PongMessage = { t: 'pong'; re: string }
export type ErrorMessage = { t: 'error'; re?: string; d: ErrorData }

const colorPattern = /^#[0-9a-f]{6}$/
const errorCodes: readonly string[] = Object.values(ErrorCode)

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isInteger(v: unknown, min: number): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v >= min
}

function isOptionalString(v: unknown): boolean {
  return v === undefined || typeof v === 'string'
}

function isErrorCode(v: unknown): v is ErrorCode {
  return typeof v === 'string' && errorCodes.includes(v)
}

function isPlayerInfo(v: unknown): v is PlayerInfo {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.displayName === 'string' &&
    typeof v.color === 'string' &&
    colorPattern.test(v.color) &&
    (v.role === 'gm' || v.role === 'player') &&
    typeof v.online === 'boolean'
  )
}

const gridTypes: readonly string[] = ['square', 'hexPointy', 'hexFlat']

function isGrid(v: unknown): v is Grid {
  return (
    isObject(v) &&
    typeof v.type === 'string' &&
    gridTypes.includes(v.type) &&
    typeof v.sizePx === 'number' &&
    typeof v.unitFt === 'number' &&
    typeof v.offsetX === 'number' &&
    typeof v.offsetY === 'number' &&
    typeof v.color === 'string' &&
    colorPattern.test(v.color) &&
    typeof v.visible === 'boolean'
  )
}

function isAssetRef(v: unknown): v is AssetRef {
  return isObject(v) && typeof v.assetId === 'string' && typeof v.url === 'string'
}

const maxCoord = 100000
const maxPolygonPoints = 200

function isCoord(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v) && Math.abs(v) <= maxCoord
}

function isPositiveCoord(v: unknown): v is number {
  return isCoord(v) && v > 0
}

function hasExactKeys(v: Record<string, unknown>, keys: string[]): boolean {
  const own = Object.keys(v)
  return own.length === keys.length && keys.every((k) => k in v)
}

export function isShape(v: unknown): v is Shape {
  if (!isObject(v)) return false
  switch (v.type) {
    case 'rect':
      return (
        hasExactKeys(v, ['type', 'x', 'y', 'w', 'h']) &&
        isCoord(v.x) &&
        isCoord(v.y) &&
        isPositiveCoord(v.w) &&
        isPositiveCoord(v.h)
      )
    case 'circle':
      return (
        hasExactKeys(v, ['type', 'x', 'y', 'r']) &&
        isCoord(v.x) &&
        isCoord(v.y) &&
        isPositiveCoord(v.r)
      )
    case 'polygon':
      return (
        hasExactKeys(v, ['type', 'points']) &&
        Array.isArray(v.points) &&
        v.points.length >= 3 &&
        v.points.length <= maxPolygonPoints &&
        v.points.every((p) => Array.isArray(p) && p.length === 2 && isCoord(p[0]) && isCoord(p[1]))
      )
    default:
      return false
  }
}

function isFogOp(v: unknown): v is FogOp {
  return (
    isObject(v) &&
    hasExactKeys(v, ['op', 'shape']) &&
    (v.op === 'reveal' || v.op === 'hide') &&
    isShape(v.shape)
  )
}

function isFog(v: unknown): v is Fog {
  return (
    isObject(v) && typeof v.enabled === 'boolean' && Array.isArray(v.ops) && v.ops.every(isFogOp)
  )
}

const visibilities: readonly unknown[] = ['all', 'owners', 'gm']

function isTokenBar(v: unknown): v is TokenBar {
  return (
    isObject(v) &&
    typeof v.key === 'string' &&
    typeof v.value === 'number' &&
    typeof v.max === 'number' &&
    visibilities.includes(v.visibility)
  )
}

function isStringArray(v: unknown): v is string[] {
  return Array.isArray(v) && v.every((x) => typeof x === 'string')
}

export function isToken(v: unknown): v is Token {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.sceneId === 'string' &&
    (v.layer === 'map' || v.layer === 'tokens' || v.layer === 'gm') &&
    typeof v.x === 'number' &&
    typeof v.y === 'number' &&
    typeof v.w === 'number' &&
    typeof v.h === 'number' &&
    typeof v.rotation === 'number' &&
    typeof v.z === 'number' &&
    typeof v.name === 'string' &&
    (v.nameVisibility === 'all' || v.nameVisibility === 'hover' || v.nameVisibility === 'gm') &&
    (v.image === null || isAssetRef(v.image)) &&
    typeof v.hidden === 'boolean' &&
    typeof v.locked === 'boolean' &&
    isStringArray(v.owners) &&
    Array.isArray(v.bars) &&
    v.bars.every(isTokenBar) &&
    isStringArray(v.conditions)
  )
}

function isPoint(v: unknown): v is [number, number] {
  return Array.isArray(v) && v.length === 2 && isCoord(v[0]) && isCoord(v[1])
}

const templateShapes: readonly unknown[] = ['cone', 'sphere', 'line', 'cube', 'cylinder']

function isTextLength(v: unknown): v is string {
  return typeof v === 'string' && Array.from(v).length >= 1 && Array.from(v).length <= 500
}

function isInRange(v: unknown, min: number, max: number): v is number {
  return typeof v === 'number' && Number.isFinite(v) && v >= min && v <= max
}

/** isDrawingData checks the data of a drawing of the kind by the table of protocol.md. */
export function isDrawingData(kind: unknown, v: unknown): boolean {
  if (!isObject(v)) return false
  switch (kind) {
    case 'freehand':
      return (
        hasExactKeys(v, ['points']) &&
        Array.isArray(v.points) &&
        v.points.length >= 2 &&
        v.points.length <= 2000 &&
        v.points.every(isPoint)
      )
    case 'rect':
      return (
        hasExactKeys(v, ['x', 'y', 'w', 'h']) &&
        isCoord(v.x) &&
        isCoord(v.y) &&
        isPositiveCoord(v.w) &&
        isPositiveCoord(v.h)
      )
    case 'circle':
      return (
        hasExactKeys(v, ['x', 'y', 'r']) && isCoord(v.x) && isCoord(v.y) && isPositiveCoord(v.r)
      )
    case 'text':
      return (
        hasExactKeys(v, ['x', 'y', 'text', 'size']) &&
        isCoord(v.x) &&
        isCoord(v.y) &&
        isTextLength(v.text) &&
        isInRange(v.size, 8, 200)
      )
    case 'template':
      return (
        hasExactKeys(v, ['shape', 'x', 'y', 'size', 'direction']) &&
        templateShapes.includes(v.shape) &&
        isCoord(v.x) &&
        isCoord(v.y) &&
        isInRange(v.size, 5, 1000) &&
        isInRange(v.direction, 0, 360) &&
        v.direction < 360
      )
    default:
      return false
  }
}

export function isDrawing(v: unknown): v is Drawing {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.sceneId === 'string' &&
    typeof v.color === 'string' &&
    colorPattern.test(v.color) &&
    typeof v.authorPlayerId === 'string' &&
    isDrawingData(v.kind, v.data)
  )
}

function isSceneView(v: unknown): v is SceneView {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.name === 'string' &&
    typeof v.width === 'number' &&
    typeof v.height === 'number' &&
    isGrid(v.grid) &&
    (v.background === null || isAssetRef(v.background)) &&
    isFog(v.fog) &&
    Array.isArray(v.tokens) &&
    v.tokens.every(isToken) &&
    Array.isArray(v.drawings) &&
    v.drawings.every(isDrawing)
  )
}

function isSceneInfo(v: unknown): v is SceneInfo {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.name === 'string' &&
    typeof v.width === 'number' &&
    typeof v.height === 'number' &&
    isGrid(v.grid) &&
    (v.background === null || isAssetRef(v.background)) &&
    isFog(v.fog) &&
    !('tokens' in v) &&
    !('drawings' in v)
  )
}

function isSceneListItem(v: unknown): v is SceneListItem {
  return (
    isObject(v) && typeof v.id === 'string' && typeof v.name === 'string' && isInteger(v.sort, 0)
  )
}

function isSnapshotMe(v: unknown): v is SnapshotMe {
  return (
    isObject(v) &&
    typeof v.accountId === 'string' &&
    typeof v.isGm === 'boolean' &&
    (v.playerId === null || typeof v.playerId === 'string')
  )
}

function isDiceDie(v: unknown): v is DiceDie {
  return (
    isObject(v) &&
    isInteger(v.value, 1) &&
    (v.dropped === undefined || typeof v.dropped === 'boolean') &&
    (v.rerolledFrom === undefined || isInteger(v.rerolledFrom, 1)) &&
    (v.exploded === undefined || typeof v.exploded === 'boolean')
  )
}

function isDiceGroup(v: unknown): v is DiceGroup {
  return (
    isObject(v) &&
    isInteger(v.count, 1) &&
    isInteger(v.sides, 2) &&
    Array.isArray(v.dice) &&
    v.dice.every(isDiceDie)
  )
}

function isDiceTerm(v: unknown): v is DiceTerm {
  return (
    isObject(v) &&
    (v.sign === 1 || v.sign === -1) &&
    isInteger(v.value, 0) &&
    (v.dice === undefined || isDiceGroup(v.dice))
  )
}

function isDiceResult(v: unknown): v is DiceResult {
  return (
    isObject(v) &&
    typeof v.formula === 'string' &&
    typeof v.total === 'number' &&
    Number.isInteger(v.total) &&
    Array.isArray(v.terms) &&
    v.terms.every(isDiceTerm)
  )
}

function isChatHandout(v: unknown): v is ChatHandout {
  return isObject(v) && typeof v.title === 'string' && isObject(v.content)
}

function isChatRoll(v: unknown): v is ChatRoll {
  return (
    isObject(v) &&
    typeof v.formula === 'string' &&
    typeof v.label === 'string' &&
    (v.result === undefined || isDiceResult(v.result))
  )
}

const chatKinds: readonly unknown[] = ['text', 'roll', 'system', 'handout']
const chatVisibilities: readonly unknown[] = ['public', 'private', 'gm', 'self', 'blind']

/** isChatMessageData checks a message and that its parts agree: a roll has a roll, a private message has recipients. */
function isChatMessageData(v: unknown): v is ChatMessage {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    isInteger(v.createdAt, 0) &&
    (v.authorPlayerId === null || typeof v.authorPlayerId === 'string') &&
    chatKinds.includes(v.kind) &&
    chatVisibilities.includes(v.visibility) &&
    (v.to === undefined || isStringArray(v.to)) &&
    (v.text === undefined || typeof v.text === 'string') &&
    (v.roll === undefined || isChatRoll(v.roll)) &&
    (v.kind === 'roll') === (v.roll !== undefined) &&
    (v.handout === undefined || isChatHandout(v.handout)) &&
    (v.kind === 'handout') === (v.handout !== undefined) &&
    (v.visibility === 'private') === (v.to !== undefined && v.to.length > 0)
  )
}

function isCombatant(v: unknown): v is Combatant {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    typeof v.tokenId === 'string' &&
    typeof v.name === 'string' &&
    (v.initiative === null ||
      (typeof v.initiative === 'number' && Number.isInteger(v.initiative))) &&
    typeof v.hidden === 'boolean'
  )
}

function isCombat(v: unknown): v is Combat {
  return (
    isObject(v) &&
    typeof v.sceneId === 'string' &&
    isInteger(v.round, 1) &&
    (v.turnCombatantId === null || typeof v.turnCombatantId === 'string') &&
    Array.isArray(v.combatants) &&
    v.combatants.every(isCombatant)
  )
}

function isSnapshotGame(v: unknown): v is SnapshotGame {
  return (
    isObject(v) &&
    typeof v.gameSessionId === 'string' &&
    typeof v.startedAt === 'number' &&
    isObject(v.campaign) &&
    typeof v.campaign.id === 'string' &&
    typeof v.campaign.name === 'string' &&
    Array.isArray(v.players) &&
    v.players.every(isPlayerInfo) &&
    (v.activeSceneId === null || typeof v.activeSceneId === 'string') &&
    Array.isArray(v.scenes) &&
    v.scenes.every(isSceneListItem) &&
    (v.scene === null || isSceneView(v.scene)) &&
    Array.isArray(v.chat) &&
    v.chat.every(isChatMessageData) &&
    (v.combat === null || isCombat(v.combat))
  )
}

export function isEnvelope(v: unknown): v is Envelope {
  return (
    isObject(v) &&
    typeof v.t === 'string' &&
    isOptionalString(v.id) &&
    isOptionalString(v.re) &&
    (v.seq === undefined || isInteger(v.seq, 1))
  )
}

export function isHelloAck(v: unknown): v is HelloAckMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Ack &&
    typeof v.re === 'string' &&
    isObject(v.d) &&
    typeof v.d.resumed === 'boolean'
  )
}

export function isSnapshot(v: unknown): v is SnapshotMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Snapshot &&
    isObject(v.d) &&
    isInteger(v.d.seq, 0) &&
    isSnapshotMe(v.d.me) &&
    (v.d.game === null || isSnapshotGame(v.d.game))
  )
}

export function isPresence(v: unknown): v is PresenceMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Presence &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    typeof v.d.playerId === 'string' &&
    typeof v.d.online === 'boolean' &&
    (v.d.player === undefined || isPlayerInfo(v.d.player))
  )
}

export function isNoop(v: unknown): v is NoopMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Noop &&
    isInteger(v.seq, 1) &&
    v.d === undefined &&
    isEnvelope(v)
  )
}

export function isScenesChanged(v: unknown): v is ScenesChangedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.ScenesChanged &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    Array.isArray(v.d.scenes) &&
    v.d.scenes.every(isSceneListItem)
  )
}

export function isSceneChanged(v: unknown): v is SceneChangedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.SceneChanged &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    isSceneInfo(v.d.scene)
  )
}

export function isSceneActivated(v: unknown): v is SceneActivatedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.SceneActivated &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    isSceneView(v.d.scene)
  )
}

export function isFogChanged(v: unknown): v is FogChangedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.FogChanged &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    typeof v.d.sceneId === 'string' &&
    isFog(v.d.fog) &&
    (v.d.tokens === undefined || (Array.isArray(v.d.tokens) && v.d.tokens.every(isToken)))
  )
}

export function isTokenChanged(v: unknown): v is TokenChangedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.TokenChanged &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    isToken(v.d.token)
  )
}

export function isTokenRemoved(v: unknown): v is TokenRemovedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.TokenRemoved &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    typeof v.d.tokenId === 'string'
  )
}

export function isTokenDragging(v: unknown): v is TokenDraggingMessage {
  return (
    isObject(v) &&
    v.t === MessageType.TokenDragging &&
    v.seq === undefined &&
    isObject(v.d) &&
    typeof v.d.tokenId === 'string' &&
    typeof v.d.x === 'number' &&
    typeof v.d.y === 'number'
  )
}

export function isDrawingChanged(v: unknown): v is DrawingChangedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.DrawingChanged &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    isDrawing(v.d.drawing)
  )
}

export function isDrawingRemoved(v: unknown): v is DrawingRemovedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.DrawingRemoved &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    typeof v.d.drawingId === 'string'
  )
}

export function isDrawingCreateAck(v: unknown): v is DrawingCreateAckMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Ack &&
    typeof v.re === 'string' &&
    isObject(v.d) &&
    typeof v.d.drawingId === 'string'
  )
}

export function isPingShown(v: unknown): v is PingShownMessage {
  return (
    isObject(v) &&
    v.t === MessageType.PingShown &&
    v.seq === undefined &&
    isObject(v.d) &&
    typeof v.d.playerId === 'string' &&
    typeof v.d.sceneId === 'string' &&
    typeof v.d.x === 'number' &&
    typeof v.d.y === 'number' &&
    typeof v.d.focus === 'boolean'
  )
}

export function isRulerShown(v: unknown): v is RulerShownMessage {
  return (
    isObject(v) &&
    v.t === MessageType.RulerShown &&
    v.seq === undefined &&
    isObject(v.d) &&
    typeof v.d.playerId === 'string' &&
    typeof v.d.sceneId === 'string' &&
    Array.isArray(v.d.points) &&
    v.d.points.every(isPoint)
  )
}

export function isTokenCreateAck(v: unknown): v is TokenCreateAckMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Ack &&
    typeof v.re === 'string' &&
    isObject(v.d) &&
    typeof v.d.tokenId === 'string'
  )
}

export function isSceneViewAck(v: unknown): v is SceneViewAckMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Ack &&
    typeof v.re === 'string' &&
    isObject(v.d) &&
    isSceneView(v.d.scene)
  )
}

export function isChatMessage(v: unknown): v is ChatMessageEvent {
  return (
    isObject(v) && v.t === MessageType.ChatMessage && isInteger(v.seq, 1) && isChatMessageData(v.d)
  )
}

export function isChatHistoryAck(v: unknown): v is ChatHistoryAckMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Ack &&
    typeof v.re === 'string' &&
    isObject(v.d) &&
    Array.isArray(v.d.messages) &&
    v.d.messages.every(isChatMessageData)
  )
}

export function isCombatChanged(v: unknown): v is CombatChangedMessage {
  return (
    isObject(v) &&
    v.t === MessageType.CombatChanged &&
    isInteger(v.seq, 1) &&
    isObject(v.d) &&
    typeof v.d.sceneId === 'string' &&
    (v.d.combat === null || isCombat(v.d.combat))
  )
}

export function isPong(v: unknown): v is PongMessage {
  return isObject(v) && v.t === MessageType.Pong && typeof v.re === 'string'
}

export function isErrorMessage(v: unknown): v is ErrorMessage {
  return (
    isObject(v) &&
    v.t === MessageType.Error &&
    isOptionalString(v.re) &&
    isObject(v.d) &&
    isErrorCode(v.d.code) &&
    typeof v.d.message === 'string'
  )
}

// Commands the client sends.
export function helloCommand(id: string, hello: Hello): Envelope {
  return { t: MessageType.Hello, id, d: hello }
}

export function pingCommand(id: string): Envelope {
  return { t: MessageType.Ping, id }
}

export interface SceneCreate {
  name: string
  width?: number
  height?: number
  grid?: Grid
}
export interface SceneUpdate {
  sceneId: string
  name?: string
  width?: number
  height?: number
  grid?: Grid
  /** null removes the background; absent leaves it. */
  backgroundAssetId?: string | null
}

export function sceneCreateCommand(id: string, d: SceneCreate): Envelope {
  return { t: MessageType.SceneCreate, id, d }
}

export function sceneUpdateCommand(id: string, d: SceneUpdate): Envelope {
  return { t: MessageType.SceneUpdate, id, d }
}

/** sceneRefCommand builds scene.delete, scene.activate or scene.view. */
export function sceneRefCommand(
  t:
    | typeof MessageType.SceneDelete
    | typeof MessageType.SceneActivate
    | typeof MessageType.SceneView,
  id: string,
  sceneId: string,
): Envelope {
  return { t, id, d: { sceneId } }
}

export function sceneReorderCommand(id: string, sceneIds: string[]): Envelope {
  return { t: MessageType.SceneReorder, id, d: { sceneIds } }
}

/** TokenFields are the settable fields of a token; absent ones stay as they are. */
export interface TokenFields {
  layer?: Token['layer']
  x?: number
  y?: number
  w?: number
  h?: number
  rotation?: number
  z?: number
  name?: string
  nameVisibility?: Token['nameVisibility']
  /** null removes the image; absent leaves it. */
  imageAssetId?: string | null
  hidden?: boolean
  locked?: boolean
  /** An empty list clears; absent leaves it. */
  owners?: string[]
  bars?: TokenBar[]
  conditions?: string[]
}

export function tokenCreateCommand(id: string, sceneId: string, fields: TokenFields): Envelope {
  return { t: MessageType.TokenCreate, id, d: { sceneId, ...fields } }
}

export function tokenUpdateCommand(id: string, tokenId: string, fields: TokenFields): Envelope {
  return { t: MessageType.TokenUpdate, id, d: { tokenId, ...fields } }
}

export function tokenMoveCommand(id: string, tokenId: string, x: number, y: number): Envelope {
  return { t: MessageType.TokenMove, id, d: { tokenId, x, y } }
}

/** tokenDragMessage has no id: the server never answers it. */
export function tokenDragMessage(tokenId: string, x: number, y: number): Envelope {
  return { t: MessageType.TokenDrag, d: { tokenId, x, y } }
}

export function tokenDeleteCommand(id: string, tokenId: string): Envelope {
  return { t: MessageType.TokenDelete, id, d: { tokenId } }
}

export function fogSetCommand(id: string, sceneId: string, enabled: boolean): Envelope {
  return { t: MessageType.FogSet, id, d: { sceneId, enabled } }
}

/** fogRevealCommand and fogHideCommand add an operation to the end of the fog. */
export function fogRevealCommand(id: string, sceneId: string, shape: Shape): Envelope {
  return { t: MessageType.FogReveal, id, d: { sceneId, shape } }
}

export function fogHideCommand(id: string, sceneId: string, shape: Shape): Envelope {
  return { t: MessageType.FogHide, id, d: { sceneId, shape } }
}

/** fogResetCommand removes every operation; enabled stays. */
export function fogResetCommand(id: string, sceneId: string): Envelope {
  return { t: MessageType.FogReset, id, d: { sceneId } }
}

/** drawingCreateCommand adds a drawing; content is the kind with the data that goes with it. */
export function drawingCreateCommand(
  id: string,
  sceneId: string,
  content: DrawingContent,
  color: string,
): Envelope {
  return { t: MessageType.DrawingCreate, id, d: { sceneId, ...content, color } }
}

/** drawingUpdateCommand changes the data (of the kind of the drawing) and/or the color. */
export function drawingUpdateCommand(
  id: string,
  drawingId: string,
  changes: { data?: DrawingContent['data']; color?: string },
): Envelope {
  return { t: MessageType.DrawingUpdate, id, d: { drawingId, ...changes } }
}

export function drawingDeleteCommand(id: string, drawingId: string): Envelope {
  return { t: MessageType.DrawingDelete, id, d: { drawingId } }
}

/** pingPlaceMessage has no id: the server never answers it. focus is for the GM only. */
export function pingPlaceMessage(sceneId: string, x: number, y: number, focus?: boolean): Envelope {
  return {
    t: MessageType.PingPlace,
    d: focus === undefined ? { sceneId, x, y } : { sceneId, x, y, focus },
  }
}

/** rulerUpdateMessage has no id; the server takes 1–50 points and shows them to the others. */
export function rulerUpdateMessage(sceneId: string, points: [number, number][]): Envelope {
  return { t: MessageType.RulerUpdate, d: { sceneId, points } }
}

/** rulerClearMessage has no id. */
export function rulerClearMessage(sceneId: string): Envelope {
  return { t: MessageType.RulerClear, d: { sceneId } }
}

/** chatSendCommand sends a text message; a non-empty to makes it private. */
export function chatSendCommand(id: string, text: string, to?: string[]): Envelope {
  return { t: MessageType.ChatSend, id, d: to === undefined ? { text } : { text, to } }
}

export function rollRequestCommand(
  id: string,
  formula: string,
  visibility: RollVisibility,
  label?: string,
): Envelope {
  return {
    t: MessageType.RollRequest,
    id,
    d: label === undefined ? { formula, visibility } : { formula, visibility, label },
  }
}

/** chatHistoryCommand asks for the messages before the message with the id before. */
export function chatHistoryCommand(id: string, before: string, limit?: number): Envelope {
  return {
    t: MessageType.ChatHistory,
    id,
    d: limit === undefined ? { before } : { before, limit },
  }
}

/** combatCommand is combat.start, combat.end, combat.next or combat.prev of a scene. */
export function combatCommand(
  type:
    | typeof MessageType.CombatStart
    | typeof MessageType.CombatEnd
    | typeof MessageType.CombatNext
    | typeof MessageType.CombatPrev,
  id: string,
  sceneId: string,
): Envelope {
  return { t: type, id, d: { sceneId } }
}

export function combatStartCommand(id: string, sceneId: string): Envelope {
  return combatCommand(MessageType.CombatStart, id, sceneId)
}

export function combatEndCommand(id: string, sceneId: string): Envelope {
  return combatCommand(MessageType.CombatEnd, id, sceneId)
}

export function combatAddCommand(id: string, sceneId: string, tokenIds: string[]): Envelope {
  return { t: MessageType.CombatAdd, id, d: { sceneId, tokenIds } }
}

export function combatRemoveCommand(id: string, combatantId: string): Envelope {
  return { t: MessageType.CombatRemove, id, d: { combatantId } }
}

/** combatSetInitiativeCommand sets the initiative; null clears it. The value is always sent. */
export function combatSetInitiativeCommand(
  id: string,
  combatantId: string,
  value: number | null,
): Envelope {
  return { t: MessageType.CombatSetInitiative, id, d: { combatantId, value } }
}

export function combatRollInitiativeCommand(id: string, combatantIds: string[]): Envelope {
  return { t: MessageType.CombatRollInitiative, id, d: { combatantIds } }
}

export function combatSetHiddenCommand(id: string, combatantId: string, hidden: boolean): Envelope {
  return { t: MessageType.CombatSetHidden, id, d: { combatantId, hidden } }
}

export function combatNextCommand(id: string, sceneId: string): Envelope {
  return combatCommand(MessageType.CombatNext, id, sceneId)
}

export function combatPrevCommand(id: string, sceneId: string): Envelope {
  return combatCommand(MessageType.CombatPrev, id, sceneId)
}
