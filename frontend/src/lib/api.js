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

// 保存类响应必须是带 id 的账号对象，否则说明请求被网关/重定向吞掉。
// 抛出错误时附上实际返回内容预览，便于定位问题发生在哪一层。
function guardSavedResponse(data) {
  if (!data || typeof data.id !== 'string' || !data.id) {
    let preview
    try {
      preview = JSON.stringify(data)
    } catch {
      preview = String(data)
    }
    if (preview && preview.length > 120) preview = preview.slice(0, 120) + '…'
    throw new Error(`保存响应异常，实际返回内容：${preview || '(空)'}`)
  }
  return data
}

export const api = {
  health: () => request('/health'),
  listAccounts: () => request('/accounts'),
  createAccount: async (payload) =>
    guardSavedResponse(await request('/accounts', { method: 'POST', body: payload })),
  updateAccount: async (id, payload) =>
    guardSavedResponse(await request(`/accounts/${id}`, { method: 'PUT', body: payload })),
  deleteAccount: (id) => request(`/accounts/${id}`, { method: 'DELETE' }),
  testAccount: (id) => request(`/accounts/${id}/test`, { method: 'POST' }),
  testConnection: (payload) => request('/test-connection', { method: 'POST', body: payload }),
  checkAccount: (id) => request(`/accounts/${id}/check`, { method: 'POST' }),
  listEvents: (limit = 50) => request(`/events?limit=${limit}`),
}
