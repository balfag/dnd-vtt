function sameDay(a: Date, b: Date, timeZone?: string): boolean {
  const day = new Intl.DateTimeFormat('en-CA', { timeZone, dateStyle: 'short' })
  return day.format(a) === day.format(b)
}

/**
 * formatMoment shows a moment as HH:mm when it is today, otherwise as the date
 * and the time. lang is the interface language; timeZone is for tests.
 */
export function formatMoment(
  ms: number,
  lang: string,
  now = new Date(),
  timeZone?: string,
): string {
  const moment = new Date(ms)
  if (sameDay(moment, now, timeZone)) {
    return new Intl.DateTimeFormat(lang, { timeZone, hour: '2-digit', minute: '2-digit' }).format(
      moment,
    )
  }
  return new Intl.DateTimeFormat(lang, {
    timeZone,
    day: 'numeric',
    month: 'long',
    hour: '2-digit',
    minute: '2-digit',
  }).format(moment)
}

/** formatDate shows a date as "12 October", with the year when it is not this year. */
export function formatDate(ms: number, lang: string, now = new Date(), timeZone?: string): string {
  const moment = new Date(ms)
  const year = new Intl.DateTimeFormat('en', { timeZone, year: 'numeric' })
  const sameYear = year.format(moment) === year.format(now)
  return new Intl.DateTimeFormat(lang, {
    timeZone,
    day: 'numeric',
    month: 'long',
    year: sameYear ? undefined : 'numeric',
  }).format(moment)
}
