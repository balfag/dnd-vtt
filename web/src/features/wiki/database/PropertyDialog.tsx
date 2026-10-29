import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  propertyTypes,
  type PageNode,
  type Property,
  type PropertyOptions,
  type PropertyType,
} from '../../../shared/api/wiki'
import { Button } from '../../../ui/Button'
import { Dialog } from '../../../ui/Dialog'
import { ErrorNote } from '../../../ui/ErrorNote'
import { useWiki } from '../store'
import { useRestoreFocus } from './useRestoreFocus'

const MAX_NAME = 100

interface PropertyDialogProps {
  open: boolean
  properties: Property[]
  onClose: () => void
  /** onAdd makes the property; it rejects with the i18n key of why not. */
  onAdd: (p: {
    name: string
    type: PropertyType
    options: PropertyOptions
  }) => Promise<string | null>
}

/** PropertyDialog asks the name and the type of a new property; a relation also needs the database it points at. */
export function PropertyDialog({ open, properties, onClose, onAdd }: PropertyDialogProps) {
  const { t } = useTranslation()
  useRestoreFocus()
  const tree = useWiki((s) => s.tree)
  const databases = useMemo(
    () => (tree ?? []).filter((n: PageNode) => n.kind === 'database'),
    [tree],
  )
  const [name, setName] = useState('')
  const [type, setType] = useState<PropertyType>('text')
  const [target, setTarget] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const nameId = useId()
  const typeId = useId()
  const targetId = useId()

  const trimmed = name.trim()
  const taken = properties.some((p) => p.name === trimmed)
  const targetOk = type !== 'relation' || (target !== '' && databases.some((d) => d.id === target))
  const valid = trimmed !== '' && trimmed.length <= MAX_NAME && !taken && targetOk

  async function submit() {
    if (!valid || busy) return
    setBusy(true)
    const options: PropertyOptions =
      type === 'relation'
        ? { targetDatabaseId: target }
        : type === 'number'
          ? { format: 'decimal' }
          : type === 'select' || type === 'multi_select'
            ? { options: [] }
            : {}
    const why = await onAdd({ name: trimmed, type, options })
    setBusy(false)
    if (why === null) {
      setName('')
      setType('text')
      setTarget('')
      setError(null)
      onClose()
    } else {
      setError(why)
    }
  }

  return (
    <Dialog
      title={t('wiki.db.property.new')}
      open={open}
      onClose={onClose}
      closeLabel={t('ui.dialog.cancel')}
      actions={
        <Button fullWidth={false} disabled={!valid || busy} onClick={() => void submit()}>
          {t('wiki.db.property.add')}
        </Button>
      }
    >
      <form
        className="dbform"
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <label className="dbform__label" htmlFor={nameId}>
          {t('wiki.db.property.name')}
        </label>
        <input
          id={nameId}
          className="field__input"
          value={name}
          maxLength={MAX_NAME}
          aria-invalid={taken}
          onChange={(e) => {
            setName(e.target.value)
            setError(null)
          }}
        />
        {taken && <p className="field__text field__text--error">{t('wiki.db.property.taken')}</p>}
        <label className="dbform__label" htmlFor={typeId}>
          {t('wiki.db.property.type')}
        </label>
        <select
          id={typeId}
          className="field__input"
          value={type}
          onChange={(e) => {
            setType(propertyTypes.find((x) => x === e.target.value) ?? 'text')
          }}
        >
          {propertyTypes.map((x) => (
            <option key={x} value={x}>
              {t(`wiki.db.type.${x}`)}
            </option>
          ))}
        </select>
        {type === 'relation' && (
          <>
            <label className="dbform__label" htmlFor={targetId}>
              {t('wiki.db.property.target')}
            </label>
            <select
              id={targetId}
              className="field__input"
              value={target}
              onChange={(e) => {
                setTarget(e.target.value)
              }}
            >
              <option value="">{t('wiki.db.property.targetPick')}</option>
              {databases.map((d) => (
                <option key={d.id} value={d.id}>
                  {[d.icon, d.title === '' ? t('wiki.untitled') : d.title]
                    .filter((s) => s !== '')
                    .join(' ')}
                </option>
              ))}
            </select>
          </>
        )}
        {error !== null && <ErrorNote>{t(error)}</ErrorNote>}
      </form>
    </Dialog>
  )
}
