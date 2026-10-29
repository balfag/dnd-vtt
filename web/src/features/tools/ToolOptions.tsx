import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TemplateShape } from '../../shared/protocol'
import { clampTextSize } from './DrawTool'
import {
  defaultTemplateSize,
  DRAW_COLORS,
  TEMPLATE_SHAPES,
  TEMPLATE_SIZE_MAX,
  TEMPLATE_SIZE_MIN,
  TEXT_SIZE_MAX,
  TEXT_SIZE_MIN,
  type DrawKind,
  type DrawOptions,
  type TemplateOptions,
} from './toolState'
import { useToolAnchor } from './useToolAnchor'
import './tools.css'

/** The tools whose options are shown here. */
export type OptionsTool = 'ruler' | 'ping' | 'draw' | 'template'

const drawKinds: DrawKind[] = ['freehand', 'rect', 'circle', 'text']

interface NumberFieldProps {
  label: string
  value: number
  min: number
  max: number
  onChange: (value: number) => void
}

/** NumberField is a whole number in a range: a wrong entry is not taken and the last good value comes back on leaving. */
function NumberField({ label, value, min, max, onChange }: NumberFieldProps) {
  const id = useId()
  // What the user is typing; null while the field shows the value.
  const [draft, setDraft] = useState<string | null>(null)
  return (
    <div className="toolbar__row">
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        className="toolbar__number"
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        step={1}
        value={draft ?? String(value)}
        onChange={(e) => {
          setDraft(e.target.value)
          const n = Number(e.target.value)
          if (e.target.value !== '' && Number.isInteger(n) && n >= min && n <= max) onChange(n)
        }}
        onBlur={() => {
          setDraft(null)
        }}
      />
    </div>
  )
}

interface ToolOptionsProps {
  tool: OptionsTool
  isGm: boolean
  draw: DrawOptions
  onDraw: (next: DrawOptions) => void
  template: TemplateOptions
  onTemplate: (next: TemplateOptions) => void
  /** The colour of the viewer: the draw tool's default. */
  myColor: string
}

/** ToolOptions is the panel of the tool in hand, beside its button on the tool strip. */
export function ToolOptions({
  tool,
  isGm,
  draw,
  onDraw,
  template,
  onTemplate,
  myColor,
}: ToolOptionsProps) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDivElement>(null)
  useToolAnchor(ref, tool)
  const groupId = useId()
  if (tool === 'ping' && !isGm) return null
  const colorNow = draw.color ?? myColor

  return (
    <div
      ref={ref}
      className="toolbar"
      role="group"
      aria-label={t(`tools.${tool}.name`)}
      data-tool-options={tool}
    >
      <h2 className="toolbar__title">{t(`tools.${tool}.name`)}</h2>
      {tool === 'ruler' && <p className="toolbar__hint">{t('tools.ruler.hint')}</p>}
      {tool === 'ping' && <p className="toolbar__hint">{t('tools.ping.hintGm')}</p>}
      {tool === 'draw' && (
        <>
          <fieldset className="toolbar__group">
            {drawKinds.map((kind) => (
              <label key={kind} className="toolbar__choice">
                <input
                  type="radio"
                  name={`${groupId}-kind`}
                  checked={draw.kind === kind}
                  onChange={() => {
                    onDraw({ ...draw, kind })
                  }}
                />
                <span>{t(`tools.draw.${kind}`)}</span>
              </label>
            ))}
          </fieldset>
          <fieldset className="toolbar__group">
            <legend className="toolbar__legend">{t('tools.draw.color')}</legend>
            <div className="toolbar__swatches">
              {DRAW_COLORS.map((color) => (
                <label key={color} className="toolbar__swatch" style={{ backgroundColor: color }}>
                  <input
                    type="radio"
                    name={`${groupId}-color`}
                    aria-label={color}
                    checked={colorNow.toLowerCase() === color}
                    onChange={() => {
                      onDraw({ ...draw, color })
                    }}
                  />
                </label>
              ))}
            </div>
          </fieldset>
          {draw.kind === 'text' && (
            <NumberField
              label={t('tools.draw.textSize')}
              value={draw.textSize}
              min={TEXT_SIZE_MIN}
              max={TEXT_SIZE_MAX}
              onChange={(textSize) => {
                onDraw({ ...draw, textSize: clampTextSize(textSize) })
              }}
            />
          )}
        </>
      )}
      {tool === 'template' && (
        <>
          <fieldset className="toolbar__group">
            {TEMPLATE_SHAPES.map((shape: TemplateShape) => (
              <label key={shape} className="toolbar__choice">
                <input
                  type="radio"
                  name={`${groupId}-shape`}
                  checked={template.shape === shape}
                  onChange={() => {
                    onTemplate({ shape, size: defaultTemplateSize(shape) })
                  }}
                />
                <span>{t(`tools.template.${shape}`)}</span>
              </label>
            ))}
          </fieldset>
          <NumberField
            label={t('tools.template.size')}
            value={template.size}
            min={TEMPLATE_SIZE_MIN}
            max={TEMPLATE_SIZE_MAX}
            onChange={(size) => {
              onTemplate({ ...template, size })
            }}
          />
        </>
      )}
    </div>
  )
}
