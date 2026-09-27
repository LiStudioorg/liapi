import { Message, Dialog } from 'fuxsto-design'

/** Thin wrappers so pages get consistent feedback + a promise-based confirm. */
export function useUi() {
  const router = useRouter()

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

  /** Handle API errors; on 401 route to the login page (no browser dialog). */
  function handleError(e: unknown) {
    const err = e as Error & { unauthorized?: boolean }
    if (err.unauthorized) {
      router.push({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })
      return
    }
    toast.error(err.message || String(e))
  }

  return { toast, confirm, handleError }
}
