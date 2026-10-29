import { useTranslation } from 'react-i18next'
import { Button } from '../../../ui/Button'
import { Dialog } from '../../../ui/Dialog'

interface ConfirmDialogProps {
  title: string
  text: string
  /** confirm names the action; the button is red when danger is set. */
  confirm: string
  danger?: boolean
  open: boolean
  onConfirm: () => void
  onClose: () => void
}

/** ConfirmDialog asks before something that cannot be undone; "Cancel" has the focus first. */
export function ConfirmDialog(p: ConfirmDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog
      title={p.title}
      open={p.open}
      closeLabel={t('ui.dialog.cancel')}
      onClose={p.onClose}
      actions={
        <Button
          variant={p.danger === true ? 'danger' : 'primary'}
          fullWidth={false}
          onClick={() => {
            p.onClose()
            p.onConfirm()
          }}
        >
          {p.confirm}
        </Button>
      }
    >
      <p>{p.text}</p>
    </Dialog>
  )
}
