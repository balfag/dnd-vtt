import { useTranslation } from 'react-i18next'
import type { DbRecord, Property } from '../../../shared/api/wiki'
import { isEmpty } from './filter'
import { ValueCell } from './values/ValueCell'

interface CardsViewProps {
  columns: Property[]
  records: DbRecord[]
  onOpen: (id: string) => void
}

/** CardsView is a grid of cards: cover, icon and title, and the visible properties that have a value. */
export function CardsView({ columns, records, onOpen }: CardsViewProps) {
  const { t } = useTranslation()
  return (
    <ul className="dbcards" aria-label={t('wiki.db.cards.label')}>
      {records.map((r) => {
        const name = r.title === '' ? t('wiki.untitled') : r.title
        const filled = columns.filter((c) => c.type === 'checkbox' || !isEmpty(r.props[c.id]))
        return (
          <li
            key={r.id}
            className="dbcard"
            onClick={() => {
              onOpen(r.id)
            }}
          >
            {r.coverAssetId !== null && (
              <div
                className="dbcard__cover"
                style={{ backgroundImage: `url(/assets/${r.coverAssetId})` }}
              />
            )}
            <div className="dbcard__body">
              <h3 className="dbcard__title">
                <button
                  type="button"
                  className="dbcard__open"
                  onClick={(e) => {
                    e.stopPropagation()
                    onOpen(r.id)
                  }}
                >
                  {r.icon !== '' && <span aria-hidden="true">{r.icon} </span>}
                  <span className={r.title === '' ? 'dbt__untitled' : undefined}>{name}</span>
                </button>
              </h3>
              {filled.length > 0 && (
                <dl className="dbcard__props">
                  {filled.map((c) => (
                    <div key={c.id} className="dbcard__prop">
                      <dt>{c.name}</dt>
                      <dd>
                        <ValueCell
                          prop={c}
                          value={r.props[c.id]}
                          editing={false}
                          readOnly
                          onEdit={noop}
                          onEnd={noop}
                          onChange={noop}
                        />
                      </dd>
                    </div>
                  ))}
                </dl>
              )}
            </div>
          </li>
        )
      })}
    </ul>
  )
}

function noop() {
  // Cards are for reading; a record is edited in its page or in the table.
}
