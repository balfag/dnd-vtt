import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { listTemplates, type Template } from '../../../shared/api/wiki'
import { PopupMenu, type MenuItem } from '../../workspace/PanelTab'
import { useWikiActions, type NewPageTemplate } from '../wikiContext'
import { builtinTemplates } from './builtin'
import { databaseTemplates, databaseTemplateTitle } from './databases'

interface NewPageMenuProps {
  doc: Document
  at: { x: number; y: number; align: 'start' | 'end' }
  /** onPick is called with nothing for a blank page. */
  onPick: (template?: NewPageTemplate) => void
  onClose: (restoreFocus: boolean) => void
}

/** NewPageMenu offers "Blank page", the built-in templates, and the GM's own ones below a line. */
export function NewPageMenu({ doc, at, onPick, onClose }: NewPageMenuProps) {
  const { t } = useTranslation()
  const { campaignId } = useWikiActions()
  const [own, setOwn] = useState<Template[]>([])

  useEffect(() => {
    let stale = false
    listTemplates(campaignId).then(
      (list) => {
        if (!stale) setOwn(list)
      },
      () => undefined,
    )
    return () => {
      stale = true
    }
  }, [campaignId])

  const items: MenuItem[] = [
    {
      key: 'blank',
      label: t('wiki.tools.templates.blank'),
      onSelect: () => {
        onPick()
      },
    },
    ...builtinTemplates.map((b, i) => ({
      key: `builtin:${b.id}`,
      label: `${b.icon} ${b.title(t)}`,
      separated: i === 0,
      onSelect: () => {
        onPick({ builtin: b.id })
      },
    })),
    ...own.map((tpl, i) => ({
      key: `custom:${tpl.id}`,
      label: [tpl.icon, tpl.name].filter((s) => s !== '').join(' '),
      separated: i === 0,
      onSelect: () => {
        onPick({ custom: tpl.id })
      },
    })),
    {
      key: 'database:empty',
      label: t('wiki.db.new.empty'),
      separated: true,
      onSelect: () => {
        onPick({ database: null })
      },
    },
    ...databaseTemplates.map((d) => ({
      key: `database:${d.id}`,
      label: t('wiki.db.new.template', { name: `${d.icon} ${databaseTemplateTitle(t, d)}` }),
      onSelect: () => {
        onPick({ database: d.id })
      },
    })),
  ]

  return (
    <PopupMenu
      doc={doc}
      at={at}
      label={t('wiki.tools.templates.menu')}
      items={items}
      onClose={onClose}
    />
  )
}
