import type { TFunction } from 'i18next'
import type { PageNode } from '../../shared/api/wiki'
import type { PanelDef } from '../workspace/panels'
import { PagePanel } from './page/PagePanel'
import { pagePanelTitle, PAGE_PANEL_PREFIX } from './panelIds'
import { TreePanel } from './tree/TreePanel'
import './wiki.css'

/** wikiPanels are the rows of the panel catalogue that belong to the wiki: the tree and a page. */
export function wikiPanels(t: TFunction, tree: PageNode[] | null): PanelDef[] {
  return [
    {
      kind: 'wiki.tree',
      title: t('wiki.tree.tab'),
      closable: true,
      popout: true,
      gmOnly: true,
      render: () => <TreePanel />,
    },
    {
      kind: 'wiki.page',
      idPrefix: PAGE_PANEL_PREFIX,
      title: t('wiki.page.tab'),
      titleFor: (params) => pagePanelTitle(tree, params.pageId, t('wiki.untitled')),
      closable: true,
      popout: true,
      gmOnly: true,
      render: (params) => <PagePanel key={String(params.pageId)} pageId={String(params.pageId)} />,
    },
  ]
}
