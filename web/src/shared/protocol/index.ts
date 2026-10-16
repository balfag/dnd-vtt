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
export interface Fog {
  enabled: boolean
  ops: unknown[]
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
/** SceneView is a scene as the client sees it; drawings join later. */
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
}
export interface SceneListItem {
  id: string
  name: string
  sort: number
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
export type SceneChangedMessage = { t: 'scene.changed'; seq: number; d: { scene: SceneView } }
export type SceneActivatedMessage = { t: 'scene.activated'; seq: number; d: { scene: SceneView } }
export type TokenChangedMessage = { t: 'token.changed'; seq: number; d: { token: Token } }
export type TokenRemovedMessage = { t: 'token.removed'; seq: number; d: { tokenId: string } }
/** TokenDraggingMessage is transient: it has no seq. */
export type TokenDraggingMessage = {
  t: 'token.dragging'
  d: { tokenId: string; x: number; y: number }
}
export type TokenCreateAckMessage = { t: 'ack'; re: string; d: { tokenId: string } }
export type SceneViewAckMessage = { t: 'ack'; re: string; d: { scene: SceneView } }
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

function isFog(v: unknown): v is Fog {
  return isObject(v) && typeof v.enabled === 'boolean' && Array.isArray(v.ops)
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
    v.tokens.every(isToken)
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
    (v.scene === null || isSceneView(v.scene))
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
    isSceneView(v.d.scene)
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
