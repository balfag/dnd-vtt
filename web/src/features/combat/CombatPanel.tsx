import { useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useTranslation } from 'react-i18next'
import { MessageType } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { readableColor } from '../chat/playerColor'
import { useGame } from '../game/store'
import { canvasBridge } from '../table/canvas/canvasBridge'
import { CombatMenu } from './CombatMenu'
import { CombatRow } from './CombatRow'
import { canEndTurn, canSetInitiative } from './combatRights'
import './combat.css'

interface CombatPanelProps {
  command: (type: string, data?: unknown) => Promise<unknown>
}

/** CombatPanel is the "Fight" tab: the turn order, the round and what the viewer may do with them. */
export function CombatPanel({ command }: CombatPanelProps) {
  const { t } = useTranslation()
  const combat = useGame((s) => s.game?.combat ?? null)
  const scene = useGame((s) => s.game?.scene ?? null)
  const activeId = useGame((s) => s.game?.activeSceneId ?? null)
  const players = useGame((s) => s.game?.players)
  const isGM = useGame((s) => s.me?.isGm === true)
  const playerId = useGame((s) => s.me?.playerId ?? null)
  const selectedId = useStore(canvasBridge, (s) => s.selectedTokenId)
  const [error, setError] = useState<string | null>(null)
  const [confirmEnd, setConfirmEnd] = useState(false)
  const list = useRef<HTMLUListElement>(null)

  const me = { isGM, playerId }
  const tokens = scene?.tokens ?? []
  const everyone = players ?? []

  function run(type: string, data: unknown): Promise<boolean> {
    setError(null)
    return command(type, data).then(
      () => true,
      (err: unknown) => {
        setError(commandErrorKey(err))
        return false
      },
    )
  }

  // The GM can look at another scene, but the fight is kept for the one the players see.
  if (scene === null || (isGM && scene.id !== activeId)) {
    return (
      <div className="combat">
        <p className="combat__empty">
          {scene === null ? t('combat.empty.player') : t('combat.notActive')}
        </p>
      </div>
    )
  }

  if (combat === null) {
    return (
      <div className="combat">
        {isGM ? (
          <>
            <p className="combat__empty">{t('combat.empty.gm')}</p>
            <div className="combat__foot">
              {error !== null && <ErrorNote>{t(error)}</ErrorNote>}
              <Button
                fullWidth={false}
                onClick={() => {
                  void run(MessageType.CombatStart, { sceneId: scene.id })
                }}
              >
                {t('combat.start')}
              </Button>
            </div>
          </>
        ) : (
          <p className="combat__empty">{t('combat.empty.player')}</p>
        )}
      </div>
    )
  }

  const inFight = new Set(combat.combatants.map((c) => c.tokenId))
  const canAdd =
    isGM &&
    selectedId !== null &&
    !inFight.has(selectedId) &&
    tokens.some((tk) => tk.id === selectedId)
  const unrolled = combat.combatants.filter((c) => c.initiative === null)
  const unrolledAll = unrolled.map((c) => c.id)
  const unrolledMine = unrolled.filter((c) => canSetInitiative(c, tokens, me)).map((c) => c.id)
  const ownsAny = combat.combatants.some((c) => canSetInitiative(c, tokens, me))

  function colorOf(tokenId: string): string | null {
    const owner = tokens.find((tk) => tk.id === tokenId)?.owners[0]
    const player = everyone.find((p) => p.id === owner)
    return player === undefined ? null : readableColor(player.color)
  }

  function rollFor(ids: string[]) {
    if (ids.length > 0) void run(MessageType.CombatRollInitiative, { combatantIds: ids })
  }

  const turnButtons = canEndTurn(combat, tokens, me)

  return (
    <div className="combat">
      <div className="combat__head">
        <h2 className="combat__round">{t('combat.round', { n: combat.round })}</h2>
        {isGM && (
          <Button
            variant="secondary"
            fullWidth={false}
            onClick={() => {
              void run(MessageType.CombatPrev, { sceneId: scene.id })
            }}
          >
            {t('combat.prev')}
          </Button>
        )}
        {turnButtons && (
          <Button
            fullWidth={false}
            onClick={() => {
              void run(MessageType.CombatNext, { sceneId: scene.id })
            }}
          >
            {t('combat.next')}
          </Button>
        )}
        {isGM && (
          <CombatMenu
            label={t('combat.tab')}
            items={[
              {
                id: 'end',
                label: t('combat.end'),
                danger: true,
                onSelect: () => {
                  setConfirmEnd(true)
                },
              },
            ]}
          />
        )}
      </div>

      <ul ref={list} className="combat__list" aria-label={t('combat.tab')} tabIndex={-1}>
        {combat.combatants.map((c) => (
          <CombatRow
            key={c.id}
            combatant={c}
            isTurn={c.id === combat.turnCombatantId}
            color={colorOf(c.tokenId)}
            isGM={isGM}
            canSetInitiative={canSetInitiative(c, tokens, me)}
            onSetInitiative={(value) =>
              run(MessageType.CombatSetInitiative, { combatantId: c.id, value })
            }
            onRoll={() => {
              rollFor([c.id])
            }}
            onLook={() => {
              canvasBridge.getState().focusToken(c.tokenId)
            }}
            onHide={(hidden) => {
              void run(MessageType.CombatSetHidden, { combatantId: c.id, hidden })
            }}
            onRemove={() => {
              void run(MessageType.CombatRemove, { combatantId: c.id }).then(() => {
                list.current?.focus()
              })
            }}
          />
        ))}
      </ul>

      <div className="combat__foot">
        {error !== null && <ErrorNote>{t(error)}</ErrorNote>}
        {isGM && (
          <Button
            variant="secondary"
            fullWidth={false}
            disabled={!canAdd}
            onClick={() => {
              if (selectedId !== null) {
                void run(MessageType.CombatAdd, { sceneId: scene.id, tokenIds: [selectedId] })
              }
            }}
          >
            {t('combat.addSelected')}
          </Button>
        )}
        {isGM && (
          <Button
            variant="secondary"
            fullWidth={false}
            disabled={unrolledAll.length === 0}
            onClick={() => {
              rollFor(unrolledAll)
            }}
          >
            {t('combat.rollAll')}
          </Button>
        )}
        {!isGM && ownsAny && (
          <Button
            variant="secondary"
            fullWidth={false}
            disabled={unrolledMine.length === 0}
            onClick={() => {
              rollFor(unrolledMine)
            }}
          >
            {t('combat.rollMine')}
          </Button>
        )}
      </div>

      <Dialog
        open={confirmEnd}
        title={t('combat.end')}
        closeLabel={t('ui.dialog.cancel')}
        onClose={() => {
          setConfirmEnd(false)
        }}
        actions={
          <Button
            variant="danger"
            fullWidth={false}
            onClick={() => {
              setConfirmEnd(false)
              void run(MessageType.CombatEnd, { sceneId: scene.id })
            }}
          >
            {t('combat.end')}
          </Button>
        }
      >
        <p>{t('combat.endConfirm')}</p>
      </Dialog>
    </div>
  )
}
