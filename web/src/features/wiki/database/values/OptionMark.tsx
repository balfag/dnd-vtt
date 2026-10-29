import type { SelectOption } from '../../../../shared/api/wiki'
import { colorValue } from '../model'

/** OptionMark is a variant of a select: a square of its color and the word. The color never stands alone. */
export function OptionMark({ option }: { option: Pick<SelectOption, 'name' | 'color'> }) {
  return (
    <span className="dbopt">
      <span
        className="dbopt__mark"
        aria-hidden="true"
        style={{ backgroundColor: colorValue[option.color] }}
      />
      <span className="dbopt__name">{option.name}</span>
    </span>
  )
}
