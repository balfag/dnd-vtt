import { describe, expect, it } from 'vitest'
import en from '../../i18n/en.json'
import ru from '../../i18n/ru.json'
import { ApiError } from './client'
import { errorCodes, errorKey } from './errors'

describe('errorKey', () => {
  it.each(errorCodes)('%s has its own key', (code) => {
    expect(errorKey(new ApiError(400, code))).toBe(`errors.${code}`)
  })

  it('falls back to errors.unknown', () => {
    expect(errorKey(new ApiError(500, 'unknown'))).toBe('errors.unknown')
    expect(errorKey(new ApiError(418, 'teapot'))).toBe('errors.unknown')
    expect(errorKey(new Error('boom'))).toBe('errors.unknown')
    expect(errorKey('boom')).toBe('errors.unknown')
  })

  it.each([
    ['ru', ru.errors],
    ['en', en.errors],
  ])('%s has a text for every key', (_lang, errors) => {
    const texts: Record<string, string> = errors
    for (const code of [...errorCodes, 'unknown']) {
      expect(texts[code], code).toBeTruthy()
    }
  })
})
