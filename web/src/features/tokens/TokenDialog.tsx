import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageType, type PlayerInfo, type Token, type TokenBar } from '../../shared/protocol'
import { commandErrorKey } from '../../shared/ws/errors'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Dialog'
import { ErrorNote } from '../../ui/ErrorNote'
import { Field } from '../../ui/Field'
import { AssetPicker } from '../assets/AssetPicker'
import { CONDITIONS } from './conditions'
import {
  buildTokenUpdate,
  formFromToken,
  limitsOf,
  maxBars,
  newBar,
  ownerChoices,
  sizePresets,
  validateTokenForm,
  type BarForm,
  type SizePreset,
  type TokenForm,
} from './tokenForm'
import './tokens.css'

interface TokenDialogProps {
  open: boolean
  /** original is the token as it was when the dialog was opened. */
  original: Token
  players: PlayerInfo[]
  campaignId: string
  command: (type: string, data?: unknown) => Promise<unknown>
  /** onSaved is called after the server took the changes (also when there were none). */
  onSaved: () => void
  onClose: () => void
}

const nameVisibilities: Token['nameVisibility'][] = ['all', 'hover', 'gm']
const layers: Token['layer'][] = ['map', 'tokens', 'gm']
const barVisibilities: TokenBar['visibility'][] = ['all', 'owners', 'gm']
const sizeChoices: SizePreset[] = [...sizePresets.map((p) => p.preset), 'custom']

/**
 * TokenDialog edits every property of a token. Its form starts from `original` when the
 * component mounts: remount it (a new key) per opening.
 */
export function TokenDialog({
  open,
  original,
  players,
  campaignId,
  command,
  onSaved,
  onClose,
}: TokenDialogProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState<TokenForm>(() => formFromToken(original, players))
  const [shown, setShown] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const formEl = useRef<HTMLFormElement>(null)
  const id = useId()

  const errors = validateTokenForm(form)
  const owners = ownerChoices(players)

  function patch(change: Partial<TokenForm>) {
    setForm((f) => ({ ...f, ...change }))
  }

  function errorText(field: string): string | undefined {
    const key = errors[field]
    if (!shown || key === undefined) return undefined
    return t(key, limitsOf(field))
  }

  function setBar(index: number, change: Partial<BarForm>) {
    setForm((f) => ({ ...f, bars: f.bars.map((b, i) => (i === index ? { ...b, ...change } : b)) }))
  }

  function toggle(list: string[], value: string, on: boolean): string[] {
    return on ? [...list.filter((v) => v !== value), value] : list.filter((v) => v !== value)
  }

  async function save() {
    if (busy) return
    setShown(true)
    if (Object.keys(errors).length > 0) {
      // The errors are drawn after this render: focus the first one then.
      requestAnimationFrame(() => {
        formEl.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
      })
      return
    }
    const update = buildTokenUpdate(original, form)
    if (update === null) {
      onClose()
      return
    }
    setBusy(true)
    setFailure(null)
    try {
      await command(MessageType.TokenUpdate, update)
      onSaved()
      onClose()
    } catch (err) {
      setFailure(commandErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  const title =
    original.name === ''
      ? t('tokens.dialog.titleUnnamed')
      : t('tokens.dialog.title', { name: original.name })

  return (
    <Dialog
      open={open}
      title={title}
      closeLabel={t('ui.dialog.cancel')}
      onClose={onClose}
      actions={
        <Button
          fullWidth={false}
          disabled={busy}
          onClick={() => {
            void save()
          }}
        >
          {t('tokens.save')}
        </Button>
      }
    >
      <form
        ref={formEl}
        className="token-form"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void save()
        }}
      >
        <Field
          id={`${id}-name`}
          label={t('tokens.field.name')}
          value={form.name}
          error={errorText('name')}
          autoComplete="off"
          onChange={(name) => {
            patch({ name })
          }}
        />
        <fieldset className="token-form__group token-form__group--plain">
          <legend className="token-form__label">{t('tokens.field.nameVisibility')}</legend>
          <div className="token-form__choices token-form__choices--row">
            {nameVisibilities.map((v) => (
              <label key={v} className="token-form__choice">
                <input
                  type="radio"
                  name={`${id}-name-vis`}
                  checked={form.nameVisibility === v}
                  onChange={() => {
                    patch({ nameVisibility: v })
                  }}
                />
                {t(`tokens.nameVis.${v}`)}
              </label>
            ))}
          </div>
        </fieldset>

        <fieldset className="token-form__group">
          <legend>{t('tokens.field.image')}</legend>
          <AssetPicker
            campaignId={campaignId}
            kind="token"
            value={form.imageAssetId}
            noneLabel={t('tokens.image.none')}
            onChange={(asset) => {
              patch({ imageAssetId: asset?.id ?? null })
            }}
          />
        </fieldset>

        <fieldset className="token-form__group">
          <legend>{t('tokens.field.size')}</legend>
          <div className="token-form__choices token-form__choices--columns">
            {sizeChoices.map((s) => (
              <label key={s} className="token-form__choice">
                <input
                  type="radio"
                  name={`${id}-size`}
                  checked={form.size === s}
                  onChange={() => {
                    patch({ size: s })
                  }}
                />
                {t(`tokens.size.${s}`)}
              </label>
            ))}
          </div>
          {form.size === 'custom' && (
            <div className="token-form__row">
              <Field
                id={`${id}-width`}
                label={t('tokens.field.width')}
                value={form.width}
                error={errorText('width')}
                autoComplete="off"
                onChange={(width) => {
                  patch({ width })
                }}
              />
              <Field
                id={`${id}-height`}
                label={t('tokens.field.height')}
                value={form.height}
                error={errorText('height')}
                autoComplete="off"
                onChange={(height) => {
                  patch({ height })
                }}
              />
            </div>
          )}
          <Field
            id={`${id}-rotation`}
            label={t('tokens.field.rotation')}
            value={form.rotation}
            error={errorText('rotation')}
            autoComplete="off"
            onChange={(rotation) => {
              patch({ rotation })
            }}
          />
        </fieldset>

        <fieldset className="token-form__group">
          <legend>{t('tokens.field.layer')}</legend>
          <div className="token-form__choices token-form__choices--row">
            {layers.map((l) => (
              <label key={l} className="token-form__choice">
                <input
                  type="radio"
                  name={`${id}-layer`}
                  checked={form.layer === l}
                  onChange={() => {
                    patch({ layer: l })
                  }}
                />
                {t(`tokens.layer.${l}`)}
              </label>
            ))}
          </div>
          <div className="token-form__choices token-form__choices--row">
            <label className="token-form__choice">
              <input
                type="checkbox"
                checked={form.hidden}
                onChange={(e) => {
                  patch({ hidden: e.target.checked })
                }}
              />
              {t('tokens.field.hidden')}
            </label>
            <label className="token-form__choice">
              <input
                type="checkbox"
                checked={form.locked}
                onChange={(e) => {
                  patch({ locked: e.target.checked })
                }}
              />
              {t('tokens.field.locked')}
            </label>
          </div>
        </fieldset>

        <fieldset className="token-form__group">
          <legend>{t('tokens.field.owners')}</legend>
          {owners.length === 0 ? (
            <p className="token-form__empty">{t('tokens.owners.empty')}</p>
          ) : (
            <div className="token-form__choices token-form__choices--columns">
              {owners.map((p) => (
                <label key={p.id} className="token-form__choice">
                  <input
                    type="checkbox"
                    checked={form.owners.includes(p.id)}
                    onChange={(e) => {
                      patch({ owners: toggle(form.owners, p.id, e.target.checked) })
                    }}
                  />
                  <span
                    className="token-form__dot"
                    style={{ background: p.color }}
                    aria-hidden="true"
                  />
                  {p.displayName}
                </label>
              ))}
            </div>
          )}
        </fieldset>

        <fieldset className="token-form__group">
          <legend>{t('tokens.field.bars')}</legend>
          {form.bars.map((bar, i) => {
            const name = bar.key === '' ? String(i + 1) : bar.key
            const at = `bar.${String(i)}`
            return (
              <div key={i} className="token-form__bar">
                <Field
                  id={`${id}-bar${String(i)}-key`}
                  label={t('tokens.bar.key')}
                  value={bar.key}
                  error={errorText(`${at}.key`)}
                  autoComplete="off"
                  onChange={(key) => {
                    setBar(i, { key })
                  }}
                />
                <Field
                  id={`${id}-bar${String(i)}-value`}
                  label={t('tokens.bar.value')}
                  value={bar.value}
                  error={errorText(`${at}.value`)}
                  autoComplete="off"
                  onChange={(value) => {
                    setBar(i, { value })
                  }}
                />
                <Field
                  id={`${id}-bar${String(i)}-max`}
                  label={t('tokens.bar.max')}
                  value={bar.max}
                  error={errorText(`${at}.max`)}
                  autoComplete="off"
                  onChange={(max) => {
                    setBar(i, { max })
                  }}
                />
                <div className="field">
                  <label className="field__label" htmlFor={`${id}-bar${String(i)}-vis`}>
                    {t('tokens.bar.visibility')}
                  </label>
                  <select
                    id={`${id}-bar${String(i)}-vis`}
                    className="field__input"
                    value={bar.visibility}
                    onChange={(e) => {
                      const v = barVisibilities.find((x) => x === e.target.value)
                      if (v !== undefined) setBar(i, { visibility: v })
                    }}
                  >
                    {barVisibilities.map((v) => (
                      <option key={v} value={v}>
                        {t(`tokens.barVis.${v}`)}
                      </option>
                    ))}
                  </select>
                </div>
                <button
                  type="button"
                  className="token-form__remove"
                  aria-label={t('tokens.bar.remove', { key: name })}
                  title={t('tokens.bar.remove', { key: name })}
                  onClick={() => {
                    setForm((f) => ({ ...f, bars: f.bars.filter((_, j) => j !== i) }))
                  }}
                >
                  <svg
                    viewBox="0 0 24 24"
                    width="20"
                    height="20"
                    aria-hidden="true"
                    focusable="false"
                  >
                    <path
                      d="M6 6l12 12M18 6L6 18"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.6"
                      strokeLinecap="round"
                    />
                  </svg>
                </button>
              </div>
            )
          })}
          <div>
            <Button
              variant="secondary"
              fullWidth={false}
              disabled={form.bars.length >= maxBars}
              onClick={() => {
                setForm((f) => ({ ...f, bars: [...f.bars, newBar(f.bars)] }))
              }}
            >
              {t('tokens.bar.add')}
            </Button>
          </div>
        </fieldset>

        <fieldset className="token-form__group">
          <legend>{t('tokens.field.conditions')}</legend>
          <div className="token-form__choices token-form__choices--columns">
            {CONDITIONS.map((c) => (
              <label key={c} className="token-form__choice">
                <input
                  type="checkbox"
                  checked={form.conditions.includes(c)}
                  onChange={(e) => {
                    patch({ conditions: toggle(form.conditions, c, e.target.checked) })
                  }}
                />
                {t(`conditions.${c}`)}
              </label>
            ))}
          </div>
        </fieldset>

        {failure !== null && <ErrorNote>{t(failure)}</ErrorNote>}
        {/* Enter in a field saves, as the button does. */}
        <button type="submit" hidden />
      </form>
    </Dialog>
  )
}
