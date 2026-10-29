import type { TFunction } from 'i18next'

/** BuiltinTemplate is a page template defined in the client; its text comes from i18n when a page is made. */
export interface BuiltinTemplate {
  id: 'session' | 'npc' | 'location' | 'quest'
  icon: string
  title: (t: TFunction) => string
  content: (t: TFunction) => unknown
}

function heading(text: string) {
  return { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text }] }
}

const empty = () => ({ type: 'paragraph' })

/** sections makes a heading with an empty paragraph under it for each name. */
function sections(names: string[]) {
  return names.flatMap((name) => [heading(name), empty()])
}

function doc(content: unknown[]) {
  return { type: 'doc', content }
}

export const builtinTemplates: BuiltinTemplate[] = [
  {
    id: 'session',
    icon: '📓',
    title: (t) => t('wiki.templates.session.title'),
    content: (t) =>
      doc(
        sections(
          ['date', 'who', 'happened', 'rewards', 'hooks'].map((k) =>
            t(`wiki.templates.session.${k}`),
          ),
        ),
      ),
  },
  {
    id: 'npc',
    icon: '🧑',
    title: (t) => t('wiki.templates.npc.title'),
    content: (t) =>
      doc(
        sections(
          ['look', 'character', 'motive', 'knows', 'ties'].map((k) => t(`wiki.templates.npc.${k}`)),
        ),
      ),
  },
  {
    id: 'location',
    icon: '🏠',
    title: (t) => t('wiki.templates.location.title'),
    content: (t) =>
      doc(
        sections(
          ['entry', 'here', 'dwellers', 'secrets'].map((k) => t(`wiki.templates.location.${k}`)),
        ),
      ),
  },
  {
    id: 'quest',
    icon: '🗡',
    title: (t) => t('wiki.templates.quest.title'),
    content: (t) => {
      const item = () => ({
        type: 'taskItem',
        attrs: { checked: false },
        content: [empty()],
      })
      return doc([
        ...sections(['giver', 'goal', 'reward'].map((k) => t(`wiki.templates.quest.${k}`))),
        heading(t('wiki.templates.quest.steps')),
        { type: 'taskList', content: [item(), item(), item()] },
        ...sections([t('wiki.templates.quest.complications')]),
      ])
    },
  },
]
