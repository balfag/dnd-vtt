import { describe, expect, it } from 'vitest'
import chatHistoryAck from '../../../../internal/ws/protocol/testdata/chat_history_ack.json'
import chatHistoryCommandExample from '../../../../internal/ws/protocol/testdata/chat_history_command.json'
import chatMessagePrivate from '../../../../internal/ws/protocol/testdata/chat_message_private.json'
import chatMessageRoll from '../../../../internal/ws/protocol/testdata/chat_message_roll.json'
import chatMessageRollBlindAuthor from '../../../../internal/ws/protocol/testdata/chat_message_roll_blind_author.json'
import chatMessageText from '../../../../internal/ws/protocol/testdata/chat_message_text.json'
import chatSendExample from '../../../../internal/ws/protocol/testdata/chat_send_command.json'
import combatChangedEnded from '../../../../internal/ws/protocol/testdata/combat_changed_ended.json'
import combatChangedGm from '../../../../internal/ws/protocol/testdata/combat_changed_gm.json'
import combatChangedPlayer from '../../../../internal/ws/protocol/testdata/combat_changed_player.json'
import combatAddExample from '../../../../internal/ws/protocol/testdata/combat_add_command.json'
import combatEndExample from '../../../../internal/ws/protocol/testdata/combat_end_command.json'
import combatNextExample from '../../../../internal/ws/protocol/testdata/combat_next_command.json'
import combatPrevExample from '../../../../internal/ws/protocol/testdata/combat_prev_command.json'
import combatRemoveExample from '../../../../internal/ws/protocol/testdata/combat_remove_command.json'
import combatRollInitiativeExample from '../../../../internal/ws/protocol/testdata/combat_roll_initiative_command.json'
import combatSetHiddenExample from '../../../../internal/ws/protocol/testdata/combat_set_hidden_command.json'
import combatSetInitiativeExample from '../../../../internal/ws/protocol/testdata/combat_set_initiative_command.json'
import combatStartExample from '../../../../internal/ws/protocol/testdata/combat_start_command.json'
import errorBadRequest from '../../../../internal/ws/protocol/testdata/error_bad_request.json'
import fogChangedGm from '../../../../internal/ws/protocol/testdata/fog_changed_gm.json'
import fogChangedPlayer from '../../../../internal/ws/protocol/testdata/fog_changed_player.json'
import fogChangedPlayerEmpty from '../../../../internal/ws/protocol/testdata/fog_changed_player_empty.json'
import fogHidePolygon from '../../../../internal/ws/protocol/testdata/fog_hide_polygon_command.json'
import fogRevealCircle from '../../../../internal/ws/protocol/testdata/fog_reveal_circle_command.json'
import fogRevealRect from '../../../../internal/ws/protocol/testdata/fog_reveal_rect_command.json'
import fogSet from '../../../../internal/ws/protocol/testdata/fog_set_command.json'
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
import rollRequestExample from '../../../../internal/ws/protocol/testdata/roll_request_command.json'
import snapshotGmNoGame from '../../../../internal/ws/protocol/testdata/snapshot_gm_no_game.json'
import snapshotGmScenes from '../../../../internal/ws/protocol/testdata/snapshot_gm_scenes.json'
import snapshotPlayer from '../../../../internal/ws/protocol/testdata/snapshot_player.json'
import tokenChanged from '../../../../internal/ws/protocol/testdata/token_changed.json'
import tokenChangedPlayer from '../../../../internal/ws/protocol/testdata/token_changed_player.json'
import tokenCreateExample from '../../../../internal/ws/protocol/testdata/token_create_command.json'
import tokenDragExample from '../../../../internal/ws/protocol/testdata/token_drag_command.json'
import tokenDragging from '../../../../internal/ws/protocol/testdata/token_dragging.json'
import tokenMoveExample from '../../../../internal/ws/protocol/testdata/token_move_command.json'
import tokenRemoved from '../../../../internal/ws/protocol/testdata/token_removed.json'
import tokenUpdateExample from '../../../../internal/ws/protocol/testdata/token_update_command.json'
import {
  chatHistoryCommand,
  chatSendCommand,
  combatAddCommand,
  combatEndCommand,
  combatNextCommand,
  combatPrevCommand,
  combatRemoveCommand,
  combatRollInitiativeCommand,
  combatSetHiddenCommand,
  combatSetInitiativeCommand,
  combatStartCommand,
  fogHideCommand,
  fogResetCommand,
  fogRevealCommand,
  fogSetCommand,
  helloCommand,
  isChatHistoryAck,
  isChatMessage,
  isCombatChanged,
  isEnvelope,
  isFogChanged,
  isErrorMessage,
  isHelloAck,
  isNoop,
  isPong,
  isPresence,
  isShape,
  isSceneActivated,
  isSceneChanged,
  isSceneViewAck,
  isScenesChanged,
  isSnapshot,
  isTokenChanged,
  isTokenCreateAck,
  isTokenDragging,
  isTokenRemoved,
  pingCommand,
  rollRequestCommand,
  sceneCreateCommand,
  sceneReorderCommand,
  sceneRefCommand,
  sceneUpdateCommand,
  tokenCreateCommand,
  tokenDeleteCommand,
  tokenDragMessage,
  tokenMoveCommand,
  tokenUpdateCommand,
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
    ['fog_changed_gm', fogChangedGm, isFogChanged],
    ['fog_changed_player', fogChangedPlayer, isFogChanged],
    ['fog_changed_player_empty', fogChangedPlayerEmpty, isFogChanged],
    ['combat_changed_gm', combatChangedGm, isCombatChanged],
    ['combat_changed_player', combatChangedPlayer, isCombatChanged],
    ['combat_changed_ended', combatChangedEnded, isCombatChanged],
    ['token_changed', tokenChanged, isTokenChanged],
    ['token_changed_player', tokenChangedPlayer, isTokenChanged],
    ['token_removed', tokenRemoved, isTokenRemoved],
    ['token_dragging', tokenDragging, isTokenDragging],
    ['chat_message_text', chatMessageText, isChatMessage],
    ['chat_message_private', chatMessagePrivate, isChatMessage],
    ['chat_message_roll', chatMessageRoll, isChatMessage],
    ['chat_message_roll_blind_author', chatMessageRollBlindAuthor, isChatMessage],
    ['chat_history_ack', chatHistoryAck, isChatHistoryAck],
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
      tokenCreateExample,
      tokenUpdateExample,
      tokenMoveExample,
      tokenDragExample,
      fogSet,
      fogRevealRect,
      fogHidePolygon,
      fogRevealCircle,
      chatSendExample,
      rollRequestExample,
      chatHistoryCommandExample,
      combatAddExample,
      combatEndExample,
      combatNextExample,
      combatPrevExample,
      combatRemoveExample,
      combatRollInitiativeExample,
      combatSetHiddenExample,
      combatSetInitiativeExample,
      combatStartExample,
    ]) {
      expect(isEnvelope(example)).toBe(true)
    }
  })
})

describe('isShape follows the limits of the server', () => {
  const rect = { type: 'rect', x: 0, y: 0, w: 1, h: 1 }
  const triangle = [
    [0, 0],
    [1, 0],
    [0, 1],
  ]
  const points = (n: number) => Array.from({ length: n }, (_, i) => [i, i % 7])
  it.each([
    ['rect', rect, true],
    ['rect with w = 0', { ...rect, w: 0 }, false],
    ['rect with an extra field', { ...rect, r: 1 }, false],
    ['rect without h', { type: 'rect', x: 0, y: 0, w: 1 }, false],
    ['circle', { type: 'circle', x: 0, y: 0, r: 1 }, true],
    ['circle with r = 0', { type: 'circle', x: 0, y: 0, r: 0 }, false],
    ['coordinate over the limit', { type: 'circle', x: 100001, y: 0, r: 1 }, false],
    ['coordinate at the limit', { type: 'circle', x: -100000, y: 0, r: 1 }, true],
    ['NaN is not JSON but still not a coordinate', { type: 'circle', x: NaN, y: 0, r: 1 }, false],
    ['polygon of 3', { type: 'polygon', points: triangle }, true],
    ['polygon of 2', { type: 'polygon', points: triangle.slice(0, 2) }, false],
    ['polygon of 200', { type: 'polygon', points: points(200) }, true],
    ['polygon of 201', { type: 'polygon', points: points(201) }, false],
    ['polygon point of three', { type: 'polygon', points: [[0, 0, 0], ...triangle] }, false],
    ['unknown type', { type: 'star' }, false],
  ])('%s', (_name, shape, ok) => {
    expect(isShape(shape)).toBe(ok)
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

  it('token commands', () => {
    expect(
      tokenCreateCommand('c-6', '01J00000000000000000000SC2', {
        x: 350,
        y: 210.5,
        name: 'Гоблин',
        nameVisibility: 'gm',
        imageAssetId: '01J00000000000000000000AS2',
        owners: ['01J00000000000000000000PL2'],
        bars: [{ key: 'hp', value: 7, max: 7, visibility: 'owners' }],
      }),
    ).toStrictEqual(tokenCreateExample)
    expect(
      tokenUpdateCommand('c-7', '01J00000000000000000000TK1', {
        hidden: true,
        imageAssetId: null,
        owners: [],
      }),
    ).toStrictEqual(tokenUpdateExample)
    expect(tokenMoveCommand('c-8', '01J00000000000000000000TK1', 420, 260)).toStrictEqual(
      tokenMoveExample,
    )
    expect(tokenDragMessage('01J00000000000000000000TK1', 400.5, 250)).toStrictEqual(
      tokenDragExample,
    )
    expect(tokenDeleteCommand('c-9', 'T1')).toStrictEqual({
      t: 'token.delete',
      id: 'c-9',
      d: { tokenId: 'T1' },
    })
  })

  it('fog commands', () => {
    const scene = '01J00000000000000000000SC2'
    expect(fogSetCommand('c-9', scene, true)).toStrictEqual(fogSet)
    expect(
      fogRevealCommand('c-10', scene, { type: 'rect', x: 0, y: 0, w: 1400, h: 1050 }),
    ).toStrictEqual(fogRevealRect)
    expect(
      fogHideCommand('c-11', scene, {
        type: 'polygon',
        points: [
          [300, 200],
          [700, 200],
          [500, 600.5],
        ],
      }),
    ).toStrictEqual(fogHidePolygon)
    expect(
      fogRevealCommand('c-12', scene, { type: 'circle', x: 500, y: 400.5, r: 120 }),
    ).toStrictEqual(fogRevealCircle)
    expect(fogResetCommand('c-13', 'S1')).toStrictEqual({
      t: 'fog.reset',
      id: 'c-13',
      d: { sceneId: 'S1' },
    })
  })

  it('chat commands', () => {
    expect(
      chatSendCommand('c-13', 'Давай обойдём сзади', ['01J00000000000000000000PL3']),
    ).toStrictEqual(chatSendExample)
    expect(chatSendCommand('c-1', 'hi')).toStrictEqual({
      t: 'chat.send',
      id: 'c-1',
      d: { text: 'hi' },
    })
    expect(rollRequestCommand('c-14', '2d20kh1+5', 'blind', 'Скрытность')).toStrictEqual(
      rollRequestExample,
    )
    expect(rollRequestCommand('c-2', '1d20', 'public')).toStrictEqual({
      t: 'roll.request',
      id: 'c-2',
      d: { formula: '1d20', visibility: 'public' },
    })
    expect(chatHistoryCommand('c-15', '01J00000000000000000000CM3', 20)).toStrictEqual(
      chatHistoryCommandExample,
    )
    expect(chatHistoryCommand('c-3', 'M1')).toStrictEqual({
      t: 'chat.history',
      id: 'c-3',
      d: { before: 'M1' },
    })
  })

  it('combat commands', () => {
    const scene = '01J00000000000000000000SC2'
    const goblin = '01J00000000000000000000CB1'
    const shadow = '01J00000000000000000000CB2'
    expect(combatStartCommand('c-16', scene)).toStrictEqual(combatStartExample)
    expect(combatEndCommand('c-17', scene)).toStrictEqual(combatEndExample)
    expect(
      combatAddCommand('c-18', scene, ['01J00000000000000000000TK1', '01J00000000000000000000TK2']),
    ).toStrictEqual(combatAddExample)
    expect(combatRemoveCommand('c-19', goblin)).toStrictEqual(combatRemoveExample)
    expect(combatSetInitiativeCommand('c-20', goblin, 17)).toStrictEqual(combatSetInitiativeExample)
    expect(combatRollInitiativeCommand('c-21', [goblin, shadow])).toStrictEqual(
      combatRollInitiativeExample,
    )
    expect(combatSetHiddenCommand('c-22', shadow, true)).toStrictEqual(combatSetHiddenExample)
    expect(combatNextCommand('c-23', scene)).toStrictEqual(combatNextExample)
    expect(combatPrevCommand('c-24', scene)).toStrictEqual(combatPrevExample)
  })

  it('a null initiative is sent, not left out', () => {
    expect(combatSetInitiativeCommand('c-1', 'C1', null)).toStrictEqual({
      t: 'combat.setInitiative',
      id: 'c-1',
      d: { combatantId: 'C1', value: null },
    })
  })

  it('the tracker frames differ for the GM and the player', () => {
    expect(combatChangedGm.d.combat.turnCombatantId).toBe('01J00000000000000000000CB2')
    expect(combatChangedPlayer.d.combat.turnCombatantId).toBeNull()
    expect(combatChangedEnded.d.combat).toBeNull()
  })

  it('a blind roll of its author has no result', () => {
    expect('result' in chatMessageRollBlindAuthor.d.roll).toBe(false)
    expect('result' in chatMessageRoll.d.roll).toBe(true)
  })

  it('isFogChanged tells the GM frame from the player frame', () => {
    expect('tokens' in fogChangedGm.d).toBe(false)
    expect(fogChangedPlayerEmpty.d.tokens).toStrictEqual([])
  })

  it('isTokenCreateAck', () => {
    expect(isTokenCreateAck({ t: 'ack', re: 'c-6', d: { tokenId: 'T1' } })).toBe(true)
    expect(isTokenCreateAck({ t: 'ack', re: 'c-6', d: {} })).toBe(false)
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
      'scene.changed carries tokens',
      isSceneChanged,
      corrupt(sceneChanged, ['d', 'scene', 'tokens'], []),
    ],
    [
      'fog op is unknown',
      isFogChanged,
      corrupt(fogChangedGm, ['d', 'fog', 'ops', '0', 'op'], 'toggle'),
    ],
    [
      'fog shape has an extra field',
      isFogChanged,
      corrupt(fogChangedGm, ['d', 'fog', 'ops', '0', 'shape', 'r'], 5),
    ],
    ['fog tokens is not an array', isFogChanged, corrupt(fogChangedPlayer, ['d', 'tokens'], null)],
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
    [
      'token layer is unknown',
      isTokenChanged,
      corrupt(tokenChanged, ['d', 'token', 'layer'], 'sky'),
    ],
    [
      'token bar visibility is unknown',
      isTokenChanged,
      corrupt(tokenChanged, ['d', 'token', 'bars', '0', 'visibility'], 'friends'),
    ],
    ['token owners is null', isTokenChanged, corrupt(tokenChanged, ['d', 'token', 'owners'], null)],
    ['token removed has no seq', isTokenRemoved, corrupt(tokenRemoved, ['seq'], undefined, true)],
    ['token dragging has a seq', isTokenDragging, corrupt(tokenDragging, ['seq'], 3)],
    [
      'scene tokens is null',
      isSceneActivated,
      corrupt(sceneActivated, ['d', 'scene', 'tokens'], null),
    ],
    ['chat message without seq', isChatMessage, corrupt(chatMessageText, ['seq'], undefined, true)],
    [
      'roll message without a roll',
      isChatMessage,
      corrupt(chatMessageRoll, ['d', 'roll'], undefined, true),
    ],
    ['text message with a roll', isChatMessage, corrupt(chatMessageText, ['d', 'roll'], {})],
    [
      'private message without recipients',
      isChatMessage,
      corrupt(chatMessagePrivate, ['d', 'to'], undefined, true),
    ],
    ['private message with empty to', isChatMessage, corrupt(chatMessagePrivate, ['d', 'to'], [])],
    [
      'public message with recipients',
      isChatMessage,
      corrupt(chatMessageText, ['d', 'to'], ['01J00000000000000000000PL3']),
    ],
    ['unknown visibility', isChatMessage, corrupt(chatMessageText, ['d', 'visibility'], 'friends')],
    ['unknown kind', isChatMessage, corrupt(chatMessageText, ['d', 'kind'], 'emote')],
    [
      'author is missing',
      isChatMessage,
      corrupt(chatMessageText, ['d', 'authorPlayerId'], undefined, true),
    ],
    [
      'die value is 0',
      isChatMessage,
      corrupt(
        chatMessageRoll,
        ['d', 'roll', 'result', 'terms', '0', 'dice', 'dice', '0', 'value'],
        0,
      ),
    ],
    [
      'history messages is null',
      isChatHistoryAck,
      corrupt(chatHistoryAck, ['d', 'messages'], null),
    ],
    [
      'history has a damaged message',
      isChatHistoryAck,
      corrupt(chatHistoryAck, ['d', 'messages', '0', 'kind'], 'emote'),
    ],
    ['snapshot chat is null', isSnapshot, corrupt(snapshotPlayer, ['d', 'game', 'chat'], null)],
    [
      'snapshot chat has a damaged message',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'game', 'chat', '0', 'visibility'], 'friends'),
    ],
    ['combat round is 0', isCombatChanged, corrupt(combatChangedGm, ['d', 'combat', 'round'], 0)],
    [
      'combatant initiative is a string',
      isCombatChanged,
      corrupt(combatChangedGm, ['d', 'combat', 'combatants', '0', 'initiative'], '17'),
    ],
    [
      'combat combatants is null',
      isCombatChanged,
      corrupt(combatChangedGm, ['d', 'combat', 'combatants'], null),
    ],
    [
      'snapshot combat is missing',
      isSnapshot,
      corrupt(snapshotPlayer, ['d', 'game', 'combat'], undefined, true),
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
