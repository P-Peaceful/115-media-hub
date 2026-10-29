/* global document, localStorage, window */
// 在首屏绘制前应用已保存主题，避免 React 挂载后的主题闪烁。
(() => {
  const root = document.documentElement
  let theme = 'system'

  try {
    const saved = localStorage.getItem('heroui-theme')
    if (saved === 'light' || saved === 'dark' || saved === 'system') {
      theme = saved
    } else {
      const legacy = localStorage.getItem('115-media-hub-preferences')
      const preference = legacy ? JSON.parse(legacy) : undefined
      if (preference?.state?.theme === 'light' || preference?.state?.theme === 'dark') {
        theme = preference.state.theme
      }
      localStorage.setItem('heroui-theme', theme)
    }
  } catch {
    // 本地存储不可用或数据损坏时，跟随系统。
  }

  const resolved = theme === 'system'
    ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
    : theme
  root.classList.toggle('light', resolved === 'light')
  root.classList.toggle('dark', resolved === 'dark')
  root.dataset.theme = resolved
})()
