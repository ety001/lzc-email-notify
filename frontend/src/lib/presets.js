/** 常用邮箱预设：选择后自动填 host/port/web_url，仍可手改 */
export const MAIL_PRESETS = [
  {
    key: 'custom',
    label: '自定义',
    imap: null,
    pop3: null,
  },
  {
    key: 'qq',
    label: 'QQ 邮箱',
    imap: { host: 'imap.qq.com', port: 993, ssl: true, web_url: 'https://mail.qq.com' },
    pop3: { host: 'pop.qq.com', port: 995, ssl: true, web_url: 'https://mail.qq.com' },
  },
  {
    key: '163',
    label: '163 邮箱',
    imap: { host: 'imap.163.com', port: 993, ssl: true, web_url: 'https://mail.163.com' },
    pop3: { host: 'pop.163.com', port: 995, ssl: true, web_url: 'https://mail.163.com' },
  },
  {
    key: '126',
    label: '126 邮箱',
    imap: { host: 'imap.126.com', port: 993, ssl: true, web_url: 'https://mail.126.com' },
    pop3: { host: 'pop.126.com', port: 995, ssl: true, web_url: 'https://mail.126.com' },
  },
  {
    key: 'gmail',
    label: 'Gmail',
    imap: { host: 'imap.gmail.com', port: 993, ssl: true, web_url: 'https://mail.google.com' },
    pop3: { host: 'pop.gmail.com', port: 995, ssl: true, web_url: 'https://mail.google.com' },
  },
  {
    key: 'outlook',
    label: 'Outlook',
    imap: {
      host: 'outlook.office365.com',
      port: 993,
      ssl: true,
      web_url: 'https://outlook.live.com',
    },
    pop3: {
      host: 'outlook.office365.com',
      port: 995,
      ssl: true,
      web_url: 'https://outlook.live.com',
    },
  },
]

export function findPreset(key) {
  return MAIL_PRESETS.find((p) => p.key === key) || MAIL_PRESETS[0]
}
