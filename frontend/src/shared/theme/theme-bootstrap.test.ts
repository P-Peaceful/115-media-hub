import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { describe, expect, it } from 'vitest'

const script = readFileSync(new URL('../../../public/theme-bootstrap.js', import.meta.url), 'utf8')

function bootstrap(saved: Record<string, string> = {}, systemDark = false, storageBlocked = false) {
  const classes = new Set<string>()
  const root = {
    classList: {
      add: (value: string) => classes.add(value),
      remove: (value: string) => classes.delete(value),
      toggle: (value: string, enabled: boolean) => enabled ? classes.add(value) : classes.delete(value),
    },
    dataset: {} as Record<string, string>,
    style: {} as Record<string, string>,
  }
  const storage = new Map(Object.entries(saved))
  runInNewContext(script, {
    document: { documentElement: root },
    window: { matchMedia: () => ({ matches: systemDark }) },
    localStorage: {
      getItem: (key: string) => { if (storageBlocked) throw new Error('blocked'); return storage.get(key) ?? null },
      setItem: (key: string, value: string) => storage.set(key, value),
    },
  })
  return { root, classes, storage }
}

describe('theme before first paint', () => {
  it('follows the OS for a new visitor', () => {
    const { root, classes } = bootstrap({}, true)
    expect(root.dataset.theme).toBe('dark')
    expect(classes.has('dark')).toBe(true)
    expect(classes.has('light')).toBe(false)
  })

  it('uses the HeroUI preference before a legacy preference', () => {
    const { root } = bootstrap({ 'heroui-theme': 'dark', '115-media-hub-preferences': '{"state":{"theme":"light"}}' })
    expect(root.dataset.theme).toBe('dark')
  })

  it('migrates an existing explicit preference to HeroUI storage', () => {
    const { root, storage } = bootstrap({ '115-media-hub-preferences': '{"state":{"theme":"light"}}' }, true)
    expect(root.dataset.theme).toBe('light')
    expect(storage.get('heroui-theme')).toBe('light')
  })

  it('resolves the saved system preference before rendering', () => {
    expect(bootstrap({ 'heroui-theme': 'system' }, true).root.dataset.theme).toBe('dark')
  })

  it('ignores corrupt legacy data and follows the OS', () => {
    expect(bootstrap({ '115-media-hub-preferences': '{invalid' }, true).root.dataset.theme).toBe('dark')
  })

  it('still applies the OS theme if storage is unavailable', () => {
    expect(bootstrap({}, true, true).root.dataset.theme).toBe('dark')
  })
})
