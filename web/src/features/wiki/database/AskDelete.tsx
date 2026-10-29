import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '../tree/ConfirmDialog'
import { useRestoreFocus } from './useRestoreFocus'

interface AskDeleteProps {
  title: string
  text: string
  onConfirm: () => void
  onClose: () => void
}

/** AskDelete asks before a view, a property or a variant is deleted; the focus goes back to where it was. */
export function AskDelete(p: AskDeleteProps) {
  const { t } = useTranslation()
  useRestoreFocus()
  return (
    <ConfirmDialog
      title={p.title}
      text={p.text}
      confirm={t('wiki.db.delete')}
      danger
      open
      onConfirm={p.onConfirm}
      onClose={p.onClose}
    />
  )
}
