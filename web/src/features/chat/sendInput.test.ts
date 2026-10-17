import { describe, expect, it } from 'vitest'
import { CommandError } from '../../shared/ws/errors'
import { parseInput } from './parseInput'
import { sendInput, type Problem, type SendState } from './sendInput'

interface Deferred {
  resolve: () => void
  reject: (err: unknown) => void
}

/** field is a fake composer: the input text, the problem and a command that answers when told to. */
function field() {
  const sent: unknown[] = []
  const pending: Deferred[] = []
  const state: SendState = { inFlight: null, lastSent: '' }
  const view = { text: '', problem: null as Problem | null }
  const hooks = {
    setText: (update: (current: string) => string) => {
      view.text = update(view.text)
    },
    setProblem: (problem: Problem | null) => {
      view.problem = problem
    },
  }
  const command = (type: string, data?: unknown) => {
    sent.push({ type, data })
    return new Promise<unknown>((resolve, reject) => {
      pending.push({
        resolve: () => {
          resolve(undefined)
        },
        reject,
      })
    })
  }
  function submit() {
    const input = view.text
    sendInput(command, state, hooks, input, parseInput(input, [], 'me'), true)
  }
  return { sent, pending, state, view, submit }
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('sendInput', () => {
  it('empties the field at once, before the server answers', () => {
    const f = field()
    f.view.text = 'hello'
    f.submit()
    expect(f.view.text).toBe('')
    expect(f.sent).toHaveLength(1)
  })

  it('does not glue a text typed while waiting to the sent one', async () => {
    const f = field()
    f.view.text = 'hello'
    f.submit()
    f.view.text = '/r 2d20kh1+5'
    f.pending[0]?.resolve()
    await settle()
    expect(f.view.text).toBe('/r 2d20kh1+5')
    expect(f.state.lastSent).toBe('hello')
  })

  it('returns the sent text when the server refuses and the field is still empty', async () => {
    const f = field()
    f.view.text = 'hello'
    f.submit()
    f.pending[0]?.reject(new CommandError('rate_limited'))
    await settle()
    expect(f.view.text).toBe('hello')
    expect(f.view.problem).toEqual({ key: 'chat.error.tooFast' })
  })

  it('keeps a new text when the server refuses and shows the error', async () => {
    const f = field()
    f.view.text = '/r 2d6+'
    f.submit()
    f.view.text = 'next'
    f.pending[0]?.reject(new CommandError('bad_request'))
    await settle()
    expect(f.view.text).toBe('next')
    expect(f.view.problem).toEqual({ key: 'chat.error.formula', params: { formula: '2d6+' } })
  })

  it('keeps the field on a parse error', () => {
    const f = field()
    f.view.text = '   '
    f.submit()
    expect(f.view.text).toBe('   ')
    expect(f.view.problem).toEqual({ key: 'chat.error.empty' })
    expect(f.sent).toHaveLength(0)
  })
})
