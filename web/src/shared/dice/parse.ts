// A port of the formula parser of internal/dice/parse.go (docs/systems/vtt.md, "Формулы бросков").
// It only checks a formula, it does not roll; the server still parses every formula itself.

const maxFormulaLen = 200
const maxTerms = 20
const maxTotalDice = 200
const maxConstant = 1_000_000
const maxCount = 100
const minSides = 2
const maxSides = 1000
// A number above this stops growing and counts as overflowed (as in Go, without an int overflow).
const numberLimit = 2 ** 31

type Compare = '=' | '<' | '<=' | '>' | '>='

class ParseFailure extends Error {}

function fail(): never {
  throw new ParseFailure()
}

function lower(c: string): string {
  return c >= 'A' && c <= 'Z' ? c.toLowerCase() : c
}

function isDigit(c: string): boolean {
  return c >= '0' && c <= '9'
}

function matches(op: Compare, v: number, n: number): boolean {
  switch (op) {
    case '=':
      return v === n
    case '<':
      return v < n
    case '<=':
      return v <= n
    case '>':
      return v > n
    case '>=':
      return v >= n
  }
}

interface Group {
  count: number
  sides: number
  keep: boolean
  reroll: boolean
  explode: boolean
}

class Parser {
  // Code points, so that positions and the length limit count characters like Go counts runes.
  private readonly rs: string[]
  private pos = 0
  private terms = 0
  private totalDice = 0
  private overflowed = false

  constructor(s: string) {
    this.rs = Array.from(s)
  }

  private eof(): boolean {
    return this.pos >= this.rs.length
  }

  private peek(): string {
    return this.rs[this.pos] ?? ''
  }

  /** peekAt returns the character at pos+offset lower-cased, or '' past the end. */
  private peekAt(offset: number): string {
    return lower(this.rs[this.pos + offset] ?? '')
  }

  private skipSpace(): void {
    while (!this.eof() && (this.peek() === ' ' || this.peek() === '\t')) {
      this.pos++
    }
  }

  private number(): number {
    this.overflowed = false
    let n = 0
    while (!this.eof() && isDigit(this.peek())) {
      if (n < numberLimit) {
        n = n * 10 + (this.peek().charCodeAt(0) - 48)
      }
      if (n >= numberLimit) {
        this.overflowed = true
      }
      this.pos++
    }
    return n
  }

  parse(): void {
    if (this.rs.length > maxFormulaLen) {
      fail()
    }
    this.skipSpace()
    if (!this.eof() && (this.peek() === '+' || this.peek() === '-')) {
      this.pos++
    }
    for (;;) {
      this.term()
      this.skipSpace()
      if (this.eof()) {
        return
      }
      if (this.peek() !== '+' && this.peek() !== '-') {
        fail()
      }
      this.pos++
    }
  }

  private term(): void {
    this.skipSpace()
    if (this.terms >= maxTerms || this.eof()) {
      fail()
    }
    if (isDigit(this.peek())) {
      const n = this.number()
      const overflowed = this.overflowed
      this.skipSpace()
      if (this.peekAt(0) === 'd') {
        this.dice(n, overflowed)
        return
      }
      if (overflowed || n > maxConstant) {
        fail()
      }
      this.terms++
      return
    }
    if (lower(this.peek()) === 'd') {
      this.dice(1, false)
      return
    }
    fail()
  }

  private dice(count: number, countOverflowed: boolean): void {
    if (countOverflowed || count < 1 || count > maxCount) {
      fail()
    }
    this.pos++ // the "d"
    this.skipSpace()

    const g: Group = { count, sides: 0, keep: false, reroll: false, explode: false }
    if (this.eof()) {
      fail()
    } else if (this.peek() === '%') {
      g.sides = 100
      this.pos++
    } else if (isDigit(this.peek())) {
      const n = this.number()
      if (this.overflowed || n < minSides || n > maxSides) {
        fail()
      }
      g.sides = n
    } else {
      fail()
    }

    this.totalDice += count
    if (this.totalDice > maxTotalDice) {
      fail()
    }
    this.modifiers(g)
    this.terms++
  }

  private modifiers(g: Group): void {
    for (;;) {
      this.skipSpace()
      if (this.eof()) {
        return
      }
      switch (lower(this.peek())) {
        case 'k':
        case 'd':
          this.keepModifier(g)
          break
        case 'r':
          this.rerollModifier(g)
          break
        case '!':
          if (g.explode) {
            fail()
          }
          g.explode = true
          this.pos++
          break
        default:
          return
      }
    }
  }

  private keepModifier(g: Group): void {
    const kind = this.peekAt(0) + this.peekAt(1)
    if (kind !== 'kh' && kind !== 'kl' && kind !== 'dh' && kind !== 'dl') {
      fail()
    }
    if (g.keep) {
      fail()
    }
    this.pos += 2
    this.skipSpace()
    let n = 1
    if (!this.eof() && isDigit(this.peek())) {
      n = this.number()
      if (this.overflowed) {
        n = 0
      }
    }
    if (n < 1 || n > g.count) {
      fail()
    }
    g.keep = true
  }

  private rerollModifier(g: Group): void {
    if (g.reroll) {
      fail()
    }
    this.pos++ // the "r"
    this.skipSpace()

    let op: Compare = '='
    if (!this.eof() && (this.peek() === '<' || this.peek() === '>')) {
      op = this.peek() as '<' | '>'
      this.pos++
      if (!this.eof() && this.peek() === '=') {
        op = `${op}=`
        this.pos++
      }
    }
    this.skipSpace()
    if (this.eof() || !isDigit(this.peek())) {
      fail()
    }
    const n = this.number()
    if (this.overflowed || n < 1 || n > g.sides) {
      fail()
    }

    let covered = 0
    for (let v = 1; v <= g.sides; v++) {
      if (matches(op, v, n)) {
        covered++
      }
    }
    if (covered === g.sides) {
      fail()
    }
    g.reroll = true
  }
}

/** isValidFormula mirrors internal/dice.Parse: true exactly when the server accepts the formula. */
export function isValidFormula(s: string): boolean {
  try {
    new Parser(s).parse()
    return true
  } catch (e) {
    if (e instanceof ParseFailure) {
      return false
    }
    throw e
  }
}
