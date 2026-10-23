import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const source = fileURLToPath(new URL('../frontend/dist/', import.meta.url))
const target = fileURLToPath(new URL('../backend/internal/webui/dist/', import.meta.url))
if (!existsSync(`${source}/index.html`)) throw new Error('Build frontend first: pnpm --dir frontend build')
rmSync(target, { recursive: true, force: true })
mkdirSync(target, { recursive: true })
cpSync(source, target, { recursive: true })
console.log('Frontend assets copied for Go embedding.')
