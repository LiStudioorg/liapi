import type { ApiError } from '~/types/api'

const TOKEN_KEY = 'liapi_admin_token'

export const useAdminToken = () =>
  useState<string>('admin-token', () => '')

export function useApi() {
  const token = useAdminToken()

  const apiBase = '/admin/api'

  function loadToken() {
    if (import.meta.client && !token.value) {
      token.value = localStorage.getItem(TOKEN_KEY) || ''
    }
  }

  function setToken(value: string) {
    token.value = value.trim()
    if (import.meta.client) {
      if (token.value) localStorage.setItem(TOKEN_KEY, token.value)
      else localStorage.removeItem(TOKEN_KEY)
    }
  }

  function clearToken() {
    setToken('')
  }

  /** Raw fetch that attaches the admin bearer token. */
  async function raw(path: string, opts: RequestInit = {}): Promise<Response> {
    loadToken()
    const headers = new Headers(opts.headers || {})
    if (!headers.has('Content-Type') && opts.body) {
      headers.set('Content-Type', 'application/json')
    }
    if (token.value) headers.set('Authorization', `Bearer ${token.value}`)
    return fetch(apiBase + path, { ...opts, headers })
  }

  async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
    let res: Response
    try {
      res = await raw(path, opts)
    } catch (e) {
      throw new Error('网络错误: ' + (e as Error).message)
    }
    const text = await res.text()
    let data: any = {}
    if (text) {
      try {
        data = JSON.parse(text)
      } catch {
        data = { raw: text }
      }
    }
    if (res.status === 401) {
      const err = new Error('unauthorized') as Error & { unauthorized?: boolean }
      err.unauthorized = true
      throw err
    }
    if (!res.ok) {
      const apiErr = data as ApiError
      const msg =
        apiErr.error?.message || text || 'HTTP ' + res.status
      throw new Error(msg)
    }
    return data as T
  }

  /** Download a file with auth applied (stats/config export). */
  async function download(path: string, filename: string): Promise<void> {
    const res = await raw(path)
    if (!res.ok) throw new Error('HTTP ' + res.status)
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
    URL.revokeObjectURL(url)
  }

  return { token, loadToken, setToken, clearToken, request, download }
}
