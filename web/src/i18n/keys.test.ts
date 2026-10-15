import { describe, expect, it } from 'vitest'
import en from './en.json'
import ru from './ru.json'

function flatten(tree: unknown, prefix = ''): Map<string, string> {
  if (typeof tree !== 'object' || tree === null || Array.isArray(tree)) {
    throw new Error(`translation node "${prefix}" is not an object`)
  }
  const out = new Map<string, string>()
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix === '' ? key : `${prefix}.${key}`
    if (typeof value === 'string') {
      out.set(path, value)
    } else {
      for (const [k, v] of flatten(value, path)) {
        out.set(k, v)
      }
    }
  }
  return out
}

const languages = { ru: flatten(ru), en: flatten(en) }

describe('i18n resources', () => {
  it('ru and en have the same keys', () => {
    expect([...languages.ru.keys()].sort()).toEqual([...languages.en.keys()].sort())
  })

  it.each(Object.entries(languages))('%s has no empty values', (_name, map) => {
    for (const [key, value] of map) {
      expect(value.trim(), key).not.toBe('')
    }
  })
})
