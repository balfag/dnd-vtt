import { useTranslation } from 'react-i18next'
import type { ChatRoll, DiceDie, DiceTerm } from '../../shared/protocol'

function Die({ die, sides }: { die: DiceDie; sides: number }) {
  const { t } = useTranslation()
  const classes = ['die']
  if (die.dropped === true) classes.push('die--dropped')
  if (die.value === sides) classes.push('die--max')
  return (
    <span
      className={classes.join(' ')}
      title={
        die.rerolledFrom === undefined
          ? undefined
          : t('chat.die.rerolled', { from: die.rerolledFrom })
      }
    >
      {die.rerolledFrom !== undefined && <s className="die__was">{die.rerolledFrom}</s>}
      {die.value}
      {die.exploded === true && <span className="die__boom">!</span>}
    </span>
  )
}

function Term({ term, first }: { term: DiceTerm; first: boolean }) {
  // The minus is U+2212, not a hyphen.
  const sign = term.sign === -1 ? '−' : first ? '' : '+'
  if (term.dice === undefined) {
    return (
      <span className="roll__term">
        {sign !== '' && `${sign} `}
        {term.value}
      </span>
    )
  }
  const { dice, sides } = term.dice
  return (
    <span className="roll__term">
      {sign !== '' && <span className="roll__sign">{sign}</span>}
      {dice.map((die, i) => (
        <Die key={i} die={die} sides={sides} />
      ))}
    </span>
  )
}

/** RollView shows the result of a roll: the total, the formula and the dice by terms. */
export function RollView({ roll }: { roll: ChatRoll }) {
  const { t } = useTranslation()
  const result = roll.result
  if (result === undefined) {
    return (
      <div className="roll">
        <span className="roll__total roll__total--hidden" aria-hidden="true">
          ?
        </span>
        <div className="roll__body">
          <span className="roll__formula">{roll.formula}</span>
          <span className="roll__hidden">{t('chat.roll.hidden')}</span>
        </div>
      </div>
    )
  }
  return (
    <div className="roll">
      <strong className="roll__total">{result.total}</strong>
      <div className="roll__body">
        <span className="roll__formula">{roll.formula}</span>
        <span className="roll__terms">
          {result.terms.map((term, i) => (
            <Term key={i} term={term} first={i === 0} />
          ))}
        </span>
      </div>
    </div>
  )
}
