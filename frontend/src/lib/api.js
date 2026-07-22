/**
 * 同源 /api 的 fetch 封装。
 * - 非 2xx：若 body 含 {"error": "..."} 则抛出该中文错误信息
 * - test 端点特殊：HTTP 恒为 200，以 {"ok":false,"error":"..."} 表示失败，调用方自行判断
 */

const BASE = '/api'

async function request(path, { method = 'GET', body } = {}) {
  let res
  try {
    res = await fetch(`${BASE}${path}`, {
      method,
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new Error('网络请求失败，请检查连接后重试')
  }

  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  if (!res.ok) {
    const msg = data && typeof data.error === 'string' && data.error
      ? data.error
      : `请求失败（HTTP ${res.status}）`
    throw new Error(msg)
  }
  return data
}

export const api = {
  listAccounts: () => request('/accounts'),
  createAccount: (payload) => request('/accounts', { method: 'POST', body: payload }),
  updateAccount: (id, payload) => request(`/accounts/${id}`, { method: 'PUT', body: payload }),
  deleteAccount: (id) => request(`/accounts/${id}`, { method: 'DELETE' }),
  testAccount: (id) => request(`/accounts/${id}/test`, { method: 'POST' }),
  checkAccount: (id) => request(`/accounts/${id}/check`, { method: 'POST' }),
  listEvents: (limit = 50) => request(`/events?limit=${limit}`),
}
