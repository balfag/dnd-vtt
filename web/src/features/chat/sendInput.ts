import { errorCodes } from '../../shared/api/errors'
import { MessageType } from '../../shared/protocol'
import { CommandError } from '../../shared/ws/errors'
import type { ParsedInput } from './parseInput'

export type Command = (type: string, data?: unknown) => Promise<unknown>

export interface Problem {
  key: string
  params?: Record<string, string>
}

/** SendState is what the composer remembers between sends. */
export interface SendState {
  inFlight: string | null
  lastSent: string
}

export interface SendHooks {
  setText: (update: (current: string) => string) => void
  setProblem: (problem: Problem | null) => void
}

function commandProblem(err: unknown, formula: string | null): Problem {
  if (err instanceof CommandError) {
    if (err.code === 'rate_limited') return { key: 'chat.error.tooFast' }
    if (err.code === 'bad_request' && formula !== null) {
      return { key: 'chat.error.formula', params: { formula } }
    }
    const known = errorCodes.find((code) => code === err.code)
    if (known !== undefined) return { key: `errors.${known}` }
  }
  return { key: 'errors.unknown' }
}

function send(command: Command, parsed: Exclude<ParsedInput, { kind: 'error' }>): Promise<unknown> {
  switch (parsed.kind) {
    case 'text':
      return command(MessageType.ChatSend, { text: parsed.text })
    case 'private':
      return command(MessageType.ChatSend, { text: parsed.text, to: parsed.to })
    case 'roll':
      return command(MessageType.RollRequest, {
        formula: parsed.formula,
        visibility: parsed.visibility,
        ...(parsed.label === '' ? {} : { label: parsed.label }),
      })
  }
}

/**
 * sendInput sends a parsed line. With clearInput the field is emptied at once;
 * if the server refuses, the sent text comes back unless a new one is already typed.
 */
export function sendInput(
  command: Command,
  state: SendState,
  hooks: SendHooks,
  input: string,
  parsed: ParsedInput,
  clearInput: boolean,
): void {
  if (parsed.kind === 'error') {
    hooks.setProblem({
      key: parsed.key,
      ...(parsed.params === undefined ? {} : { params: parsed.params }),
    })
    return
  }
  // The same text is not sent again until the server has answered.
  if (state.inFlight === input) return
  state.inFlight = input
  hooks.setProblem(null)
  if (clearInput) hooks.setText(() => '')
  const formula = parsed.kind === 'roll' ? parsed.formula : null
  send(command, parsed)
    .then(() => {
      state.lastSent = input
    })
    .catch((err: unknown) => {
      if (clearInput) hooks.setText((current) => (current === '' ? input : current))
      hooks.setProblem(commandProblem(err, formula))
    })
    .finally(() => {
      if (state.inFlight === input) state.inFlight = null
    })
}
