import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  deleteTemplate,
  listTemplates,
  renameTemplate,
  type Template,
} from '../../../shared/api/wiki'
import { wikiStore } from '../store'
import { ConfirmDialog } from '../tree/ConfirmDialog'
import { Section, SectionRow } from '../tree/Section'
import { useAsk } from '../tree/useAsk'
import { useToolsList } from '../tree/useToolsList'
import { useWikiActions } from '../wikiContext'

/** TemplatesSection is the fold "Templates": the GM's own page templates; rename and delete. */
export function TemplatesSection() {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const { items, failed } = useToolsList(() => listTemplates(actions.campaignId))
  const [renaming, setRenaming] = useState<string | null>(null)
  const removing = useAsk<Template>()

  async function run(job: () => Promise<unknown>) {
    try {
      await job()
    } catch {
      actions.notify('wiki.tools.templates.failed')
    }
    wikiStore.getState().bumpTools()
  }

  return (
    <Section name={t('wiki.tools.templates.title')} count={items.length}>
      {failed && <p className="tsec__note">{t('wiki.tools.templates.loadFailed')}</p>}
      {!failed && items.length === 0 && (
        <p className="tsec__note">{t('wiki.tools.templates.empty')}</p>
      )}
      <ul className="tsec__list">
        {items.map((tpl) => (
          <SectionRow
            key={tpl.id}
            icon={tpl.icon}
            label={tpl.name}
            menuLabel={t('wiki.tools.templates.rowMenu', { name: tpl.name })}
            renaming={renaming === tpl.id}
            renameLabel={t('wiki.tools.templates.renameLabel')}
            onRename={(name) => {
              setRenaming(null)
              const next = name.trim()
              if (next !== '' && next !== tpl.name) void run(() => renameTemplate(tpl.id, next))
            }}
            onCancelRename={() => {
              setRenaming(null)
            }}
            items={[
              {
                key: 'rename',
                label: t('wiki.tools.templates.rename'),
                onSelect: () => {
                  setRenaming(tpl.id)
                },
              },
              {
                key: 'delete',
                label: t('wiki.tools.templates.delete'),
                onSelect: () => {
                  removing.ask(tpl)
                },
              },
            ]}
          />
        ))}
      </ul>

      {removing.asked !== null && (
        <ConfirmDialog
          title={t('wiki.tools.templates.deleteTitle', { name: removing.asked.value.name })}
          text={t('wiki.tools.templates.deleteText')}
          confirm={t('wiki.tools.templates.delete')}
          danger
          open={removing.asked.open}
          onClose={removing.close}
          onConfirm={() => {
            if (removing.asked !== null) {
              const { id } = removing.asked.value
              void run(() => deleteTemplate(id))
            }
          }}
        />
      )}
    </Section>
  )
}
