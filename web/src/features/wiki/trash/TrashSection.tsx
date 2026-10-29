import { useTranslation } from 'react-i18next'
import {
  emptyTrash,
  listTrash,
  purgeTrash,
  restoreTrash,
  type TrashItem,
} from '../../../shared/api/wiki'
import { wikiStore } from '../store'
import { ConfirmDialog } from '../tree/ConfirmDialog'
import { Section, SectionRow } from '../tree/Section'
import { useAsk } from '../tree/useAsk'
import { useToolsList } from '../tree/useToolsList'
import { useWikiActions } from '../wikiContext'

/** TrashSection is the fold "Trash": the roots of what was deleted, with a way back. */
export function TrashSection() {
  const { t, i18n } = useTranslation()
  const actions = useWikiActions()
  const { items, failed } = useToolsList(() => listTrash(actions.campaignId))
  const purging = useAsk<TrashItem>()
  const emptying = useAsk<null>()

  // The browser's time zone, in the language of the interface; only the date.
  const date = (ms: number) =>
    new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium' }).format(ms)
  const nameOf = (item: TrashItem) => (item.title === '' ? t('wiki.untitled') : item.title)

  async function run(job: () => Promise<void>, reloadTree: boolean) {
    try {
      await job()
    } catch {
      actions.notify('wiki.tools.trash.failed')
    }
    wikiStore.getState().bumpTools()
    if (reloadTree) await actions.reload().catch(() => undefined)
  }

  return (
    <Section name={t('wiki.tools.trash.title')} count={items.length}>
      {failed && <p className="tsec__note">{t('wiki.tools.trash.loadFailed')}</p>}
      {!failed && items.length === 0 && <p className="tsec__note">{t('wiki.tools.trash.empty')}</p>}
      <ul className="tsec__list">
        {items.map((item) => (
          <SectionRow
            key={item.id}
            icon={item.icon}
            label={nameOf(item)}
            meta={`${t('wiki.tools.trash.deleted', { date: date(item.deletedAt) })}\n${t('wiki.tools.trash.purgeAt', { date: date(item.purgeAt) })}`}
            menuLabel={t('wiki.tools.trash.rowMenu', { name: nameOf(item) })}
            extra={
              <button
                type="button"
                className="srow__action"
                aria-label={t('wiki.tools.trash.restoreLabel', { name: nameOf(item) })}
                onClick={() => {
                  void run(() => restoreTrash(item.id), true)
                }}
              >
                {t('wiki.tools.trash.restore')}
              </button>
            }
            items={[
              {
                key: 'purge',
                label: t('wiki.tools.trash.purge'),
                onSelect: () => {
                  purging.ask(item)
                },
              },
            ]}
          />
        ))}
      </ul>
      {items.length > 0 && (
        <div className="tsec__foot">
          <button
            type="button"
            className="srow__action"
            onClick={() => {
              emptying.ask(null)
            }}
          >
            {t('wiki.tools.trash.emptyAll')}
          </button>
        </div>
      )}

      {purging.asked !== null && (
        <ConfirmDialog
          title={t('wiki.tools.trash.purgeTitle', { name: nameOf(purging.asked.value) })}
          text={
            purging.asked.value.pageCount > 1
              ? t('wiki.tools.trash.purgeTextChildren', {
                  count: purging.asked.value.pageCount - 1,
                })
              : t('wiki.tools.trash.purgeText')
          }
          confirm={t('wiki.tools.trash.purge')}
          danger
          open={purging.asked.open}
          onClose={purging.close}
          onConfirm={() => {
            if (purging.asked !== null) {
              const { id } = purging.asked.value
              void run(() => purgeTrash(id), false)
            }
          }}
        />
      )}
      {emptying.asked !== null && (
        <ConfirmDialog
          title={t('wiki.tools.trash.emptyTitle')}
          text={t('wiki.tools.trash.emptyText')}
          confirm={t('wiki.tools.trash.emptyAll')}
          danger
          open={emptying.asked.open}
          onClose={emptying.close}
          onConfirm={() => {
            void run(() => emptyTrash(actions.campaignId), false)
          }}
        />
      )}
    </Section>
  )
}
