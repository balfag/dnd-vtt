export type ParsedInput =
  | { kind: 'text'; text: string }
  | { kind: 'private'; to: string[]; text: string } // player ids
  | {
      kind: 'roll'
      formula: string
      label: string
      visibility: 'public' | 'gm' | 'self' | 'blind'
    }
  | { kind: 'error'; key: string; params?: Record<string, string> } // i18n key

const maxLabel = 100

const rollCommands = new Map<string, 'public' | 'gm' | 'self' | 'blind'>([
  ['r', 'public'],
  ['gmr', 'gm'],
  ['sr', 'self'],
  ['br', 'blind'],
])

function fail(key: string, params?: Record<string, string>): ParsedInput {
  return params === undefined ? { kind: 'error', key } : { kind: 'error', key, params }
}

function parseRoll(rest: string, visibility: 'public' | 'gm' | 'self' | 'blind'): ParsedInput {
  const body = rest.trim()
  if (body === '') return fail('chat.error.noFormula')
  const space = body.search(/\s/)
  const formula = space === -1 ? body : body.slice(0, space)
  const label = space === -1 ? '' : body.slice(space).trim()
  if ([...label].length > maxLabel) return fail('chat.error.labelTooLong')
  return { kind: 'roll', formula, label, visibility }
}

function parseWhisper(
  rest: string,
  players: { id: string; displayName: string }[],
  meId: string,
): ParsedInput {
  const colon = rest.indexOf(':')
  const text = colon === -1 ? '' : rest.slice(colon + 1).trim()
  const names =
    colon === -1
      ? []
      : rest
          .slice(0, colon)
          .split(',')
          .map((n) => n.trim())
          .filter((n) => n !== '')
  if (text === '' || names.length === 0) return fail('chat.error.whisperFormat')

  const to: string[] = []
  for (const name of names) {
    const wanted = name.toLowerCase()
    const found = players.filter((p) => p.displayName.trim().toLowerCase() === wanted)
    if (found.length === 0) return fail('chat.error.noPlayer', { name })
    if (found.some((p) => p.id === meId)) return fail('chat.error.self')
    for (const p of found) {
      if (!to.includes(p.id)) to.push(p.id)
    }
  }
  return { kind: 'private', to, text }
}

/** parseInput turns the composer text into a command; players are the game's participants. */
export function parseInput(
  input: string,
  players: { id: string; displayName: string }[],
  meId: string,
): ParsedInput {
  const trimmed = input.trim()
  if (trimmed === '') return fail('chat.error.empty')
  if (!trimmed.startsWith('/')) return { kind: 'text', text: trimmed }
  if (trimmed.startsWith('//')) return { kind: 'text', text: trimmed.slice(1) }

  const space = trimmed.search(/\s/)
  const command = (space === -1 ? trimmed.slice(1) : trimmed.slice(1, space)).toLowerCase()
  const rest = space === -1 ? '' : trimmed.slice(space)
  const visibility = rollCommands.get(command)
  if (visibility !== undefined) return parseRoll(rest, visibility)
  if (command === 'w') return parseWhisper(rest, players, meId)
  return fail('chat.error.unknownCommand')
}
