import { Message, Dialog } from 'fuxsto-design'

/** Thin wrappers so pages get consistent feedback + a promise-based confirm. */
export function useUi() {
  const toast = {
    success: (m: string) => Message.success(m),
    error: (m: string) => Message.error(m),
    warning: (m: string) => Message.warning(m),
    info: (m: string) => Message.info(m),
  }

  function confirm(
    title: string,
    description: string,
    opts: { danger?: boolean; confirmText?: string } = {},
  ): Promise<boolean> {
    return new Promise((resolve) => {
      Dialog.confirm({
        title,
        description,
        danger: opts.danger,
        confirmText: opts.confirmText || '确认',
        cancelText: '取消',
        onConfirm: () => resolve(true),
        onCancel: () => resolve(false),
        onClose: () => resolve(false),
      })
    })
  }

  /** Handle API errors, auto-prompting for the admin token on 401. */
  function handleError(e: unknown, setToken: (v: string) => void) {
    const err = e as Error & { unauthorized?: boolean }
    if (err.unauthorized) {
      const t = window.prompt('请输入 admin token')
      if (t) setToken(t)
      return
    }
    toast.error(err.message || String(e))
  }

  return { toast, confirm, handleError }
}
