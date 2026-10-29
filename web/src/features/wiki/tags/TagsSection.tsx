import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from '../../../shared/api/client'
import { deleteTag, listTags, renameTag, type TagCount } from '../../../shared/api/wiki'
import { wikiStore } from '../store'
import { ConfirmDialog } from '../tree/ConfirmDialog'
import { Section, SectionRow } from '../tree/Section'
import { useAsk } from '../tree/useAsk'
import { useToolsList } from '../tree/useToolsList'
import { useWikiActions } from '../wikiContext'

const maxTagLength = 40

/** TagsSection is the fold "Tags" of the tree: every tag with its number of pages. */
export function TagsSection({ onSearch }: { onSearch: (query: string) => void }) {
  const { t } = useTranslation()
  const actions = useWikiActions()
  const { items, failed } = useToolsList(() => listTags(actions.campaignId))
  const [renaming, setRenaming] = useState<string | null>(null)
  const removing = useAsk<TagCount>()
  const merging = useAsk<{ tag: TagCount; name: string }>()

  async function rename(tag: TagCount, name: string, merge: boolean) {
    try {
      await renameTag(tag.id, name, merge ? true : undefined)
      wikiStore.getState().bumpTools()
    } catch (err) {
      if (!merge && err instanceof ApiError && err.status === 409) {
        merging.ask({ tag, name })
      } else {
        actions.notify('wiki.tools.tags.failed')
      }
    }
  }

  async function remove(tag: TagCount) {
    try {
      await deleteTag(tag.id)
      wikiStore.getState().bumpTools()
    } catch {
      actions.notify('wiki.tools.tags.failed')
    }
  }

  return (
    <Section name={t('wiki.tools.tags.title')} count={items.length}>
      {failed && <p className="tsec__note">{t('wiki.tools.tags.loadFailed')}</p>}
      {!failed && items.length === 0 && <p className="tsec__note">{t('wiki.tools.tags.empty')}</p>}
      <ul className="tsec__list" aria-label={t('wiki.tools.tags.list')}>
        {items.map((tag) => (
          <SectionRow
            key={tag.id}
            label={tag.name}
            count={tag.pageCount}
            menuLabel={t('wiki.tools.tags.rowMenu', { name: tag.name })}
            onClick={() => {
              onSearch(`#${tag.name}`)
            }}
            renaming={renaming === tag.id}
            renameLabel={t('wiki.tools.tags.renameLabel')}
            renameMax={maxTagLength}
            onRename={(name) => {
              setRenaming(null)
              const next = name.trim()
              if (next !== '' && next !== tag.name) void rename(tag, next, false)
            }}
            onCancelRename={() => {
              setRenaming(null)
            }}
            items={[
              {
                key: 'rename',
                label: t('wiki.tools.tags.rename'),
                onSelect: () => {
                  setRenaming(tag.id)
                },
              },
              {
                key: 'delete',
                label: t('wiki.tools.tags.delete'),
                onSelect: () => {
                  removing.ask(tag)
                },
              },
            ]}
          />
        ))}
      </ul>

      {removing.asked !== null && (
        <ConfirmDialog
          title={t('wiki.tools.tags.deleteTitle', { name: removing.asked.value.name })}
          text={t('wiki.tools.tags.deleteText', { count: removing.asked.value.pageCount })}
          confirm={t('wiki.tools.tags.delete')}
          danger
          open={removing.asked.open}
          onClose={removing.close}
          onConfirm={() => {
            if (removing.asked !== null) void remove(removing.asked.value)
          }}
        />
      )}
      {merging.asked !== null && (
        <ConfirmDialog
          title={t('wiki.tools.tags.mergeTitle', { name: merging.asked.value.name })}
          text={t('wiki.tools.tags.mergeText')}
          confirm={t('wiki.tools.tags.merge')}
          open={merging.asked.open}
          onClose={merging.close}
          onConfirm={() => {
            if (merging.asked !== null) {
              void rename(merging.asked.value.tag, merging.asked.value.name, true)
            }
          }}
        />
      )}
    </Section>
  )
}
