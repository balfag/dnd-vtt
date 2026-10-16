import { describe, expect, it } from 'vitest'
import errorBadRequest from '../../../../internal/ws/protocol/testdata/error_bad_request.json'
import helloAckFresh from '../../../../internal/ws/protocol/testdata/hello_ack_fresh.json'
import helloAckResumed from '../../../../internal/ws/protocol/testdata/hello_ack_resumed.json'
import helloCommandExample from '../../../../internal/ws/protocol/testdata/hello_command.json'
import helloEmptyCommand from '../../../../internal/ws/protocol/testdata/hello_empty_command.json'
import noop from '../../../../internal/ws/protocol/testdata/noop.json'
import ping from '../../../../internal/ws/protocol/testdata/ping.json'
import pong from '../../../../internal/ws/protocol/testdata/pong.json'
import presenceNewPlayer from '../../../../internal/ws/protocol/testdata/presence_new_player.json'
import presenceOffline from '../../../../internal/ws/protocol/testdata/presence_offline.json'
import sceneActivated from '../../../../internal/ws/protocol/testdata/scene_activated.json'
import sceneChanged from '../../../../internal/ws/protocol/testdata/scene_changed.json'
import sceneCreateExample from '../../../../internal/ws/protocol/testdata/scene_create_command.json'
import sceneUpdateExample from '../../../../internal/ws/protocol/testdata/scene_update_command.json'
import sceneViewAck from '../../../../internal/ws/protocol/testdata/scene_view_ack.json'
import scenesChanged from '../../../../internal/ws/protocol/testdata/scenes_changed.json'
import snapshotGmNoGame from '../../../../internal/ws/protocol/testdata/snapshot_gm_no_game.json'
import snapshotGmScenes from '../../../../internal/ws/protocol/testdata/snapshot_gm_scenes.json'
import snapshotPlayer from '../../../../internal/ws/protocol/testdata/snapshot_player.json'
import {
  helloCommand,
  isEnvelope,
  isErrorMessage,
  isHelloAck,
  isNoop,
  isPong,
  isPresence,
  isSceneActivated,
  isSceneChanged,
  isSceneViewAck,
  isScenesChanged,
  isSnapshot,
  pingCommand,
  sceneCreateCommand,
  sceneReorderCommand,
  sceneRefCommand,
  sceneUpdateCommand,
} from './index'

type Guard = (v: unknown) => boolean

describe('server examples pass their guards', () => {
  const cases: [string, unknown, Guard][] = [
    ['hello_ack_resumed', helloAckResumed, isHelloAck],
    ['hello_ack_fresh', helloAckFresh, isHelloAck],
    ['snapshot_player', snapshotPlayer, isSnapshot],
    ['snapshot_gm_no_game', snapshotGmNoGame, isSnapshot],
    ['snapshot_gm_scenes', snapshotGmScenes, isSnapshot],
    ['scenes_changed', scenesChanged, isScenesChanged],
    ['scene_changed', sceneChanged, isSceneChanged],
    ['scene_activated', sceneActivated, isSceneActivated],
    ['scene_view_ack', sceneViewAck, isSceneViewAck],
    ['presence_offline', presenceOffline, isPresence],
    ['presence_new_player', presenceNewPlayer, isPresence],
    ['noop', noop, isNoop],
    ['pong', pong, isPong],
    ['error_bad_request', errorBadRequest, isErrorMessage],
  ]
  it.each(cases)('%s', (_name, example, guard) => {
    expect(guard(example)).toBe(true)
    expect(isEnvelope(example)).toBe(true)
  })

  it('client examples are envelopes too', () => {
    for (const example of [
      helloCommandExample,
      helloEmptyCommand,
      ping,
      sceneCreateExample,
      sceneUpdateExample,
    ]) {
      expect(isEnvelope(example)).toBe(true)
    }
  })
})

describe('command builders match the examples', () => {
  it('helloCommand with data', () => {
    expect(
      helloCommand('c-1', { gameSessionId: '01J00000000000000000000GS1', lastSeq: 17 }),
    ).toStrictEqual(helloCommandExample)
  })

  it('helloCommand without data', () => {
    expect(helloCommand('c-1', {})).toStrictEqual(helloEmptyCommand)
  })

  it('sceneCreateCommand', () => {
    expect(
      sceneCreateCommand('c-3', {
        name: 'Таверна',
        width: 3000,
        height: 2000,
        grid: {
          type: 'square',
          sizePx: 70,
          unitFt: 5,
          offsetX: 0,
          offsetY: 0,
          color: '#9aa8b3',
          visible: true,
        },
      }),
    ).toStrictEqual(sceneCreateExample)
  })

  it('sceneUpdateCommand keeps backgroundAssetId: null', () => {
    expect(
      sceneUpdateCommand('c-4', {
        sceneId: '01J00000000000000000000SC1',
        name: 'Таверна «Дракон»',
        backgroundAssetId: null,
      }),
    ).toStrictEqual(sceneUpdateExample)
  })

  it('scene ref and reorder commands', () => {
    expect(sceneRefCommand('scene.activate', 'c-6', 'S1')).toStrictEqual({
      t: 'scene.activate',
      id: 'c-6',
      d: { sceneId: 'S1' },
    })
    expect(sceneReorderCommand('c-7', ['S2', 'S1'])).toStrictEqual({
      t: 'scene.reorder',
      id: 'c-7',
      d: { sceneIds: ['S2', 'S1'] },
    })
  })

  it('pingCommand has no d field', () => {
    expect(pingCommand('p-5')).toStrictEqual(ping)
  })
})

// corrupt returns a deep copy of base with the value at path replaced, or the
// property removed when remove is true.
function corrupt(base: unknown, path: string[], value: unknown, remove = false): unknown {
  const copy: unknown = structuredClone(base)
  let cur: unknown = copy
  for (const key of path.slice(0, -1)) {
    if (typeof cur !== 'object' || cur === null) throw new Error(`bad test path ${path.join('.')}`)
    cur = Reflect.get(cur, key)
  }
  const last = path[path.length - 1]
  if (typeof cur !== 'object' || cur === null || last === undefined) {
    throw new Error(`bad test path ${path.join('.')}`)
  }
  if (remove) {
    Reflect.deleteProperty(cur, last)
  } else {
    Reflect.set(cur, last, value)
  }
  return copy
}

describe('guards reject damaged messages', () => {
  const cases: [string, Guard, unknown][] = [
    [
      'snapshot me.isGm is a string',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'me', 'isGm'], 'false'),
    ],
    [
      'snapshot game.players is not an array',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'game', 'players'], 'x'),
    ],
    [
      'player color in upper case',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'game', 'players', '0', 'color'], '#E53935'),
    ],
    [
      'player role spectator',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'game', 'players', '1', 'role'], 'spectator'),
    ],
    [
      'snapshot playerId is a number',
      isSnapshot,
      corrupt(snapshotGmNoGame, ['d', 'me', 'playerId'], 7),
    ],
    [
      'snapshot game is missing',
      isSnapshot,
      corrupt(snapshotGmNoGame, ['d', 'game'], undefined, true),
    ],
    ['presence seq is 0', isPresence, corrupt(presenceNewPlayer, ['seq'], 0)],
    [
      'presence player is damaged',
      isPresence,
      corrupt(presenceNewPlayer, ['d', 'player', 'online'], 'yes'),
    ],
    [
      'ack resumed is missing',
      isHelloAck,
      corrupt(helloAckResumed, ['d', 'resumed'], undefined, true),
    ],
    ['error code is unknown', isErrorMessage, corrupt(errorBadRequest, ['d', 'code'], 'oops')],
    [
      'scene grid type is unknown',
      isSceneChanged,
      corrupt(sceneChanged, ['d', 'scene', 'grid', 'type'], 'octagon'),
    ],
    [
      'scene grid color in upper case',
      isSceneChanged,
      corrupt(sceneChanged, ['d', 'scene', 'grid', 'color'], '#9AA8B3'),
    ],
    [
      'scene fog ops is not an array',
      isSceneActivated,
      corrupt(sceneActivated, ['d', 'scene', 'fog', 'ops'], {}),
    ],
    [
      'snapshot of a player lacks the scene',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'game', 'scene'], undefined, true),
    ],
    [
      'snapshot scenes is not an array',
      isSnapshot,
      corrupt(snapshotGmScenes, ['d', 'game', 'scenes'], null),
    ],
    [
      'scenes_changed sort is negative',
      isScenesChanged,
      corrupt(scenesChanged, ['d', 'scenes', '0', 'sort'], -1),
    ],
    ['noop seq is 0', isNoop, corrupt(noop, ['seq'], 0)],
    ['noop has data', isNoop, corrupt(noop, ['d'], {})],
    ['isEnvelope(null)', isEnvelope, null],
    ['isEnvelope([])', isEnvelope, []],
    ['isEnvelope({ t: 1 })', isEnvelope, { t: 1 }],
  ]
  it.each(cases)('%s', (_name, guard, value) => {
    expect(guard(value)).toBe(false)
  })
})
