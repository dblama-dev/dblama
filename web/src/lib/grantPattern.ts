const any = Symbol('%')
const one = Symbol('_')
type Token = string | typeof any | typeof one

export function usernamePrefixPattern(username: string): string {
  return username.replace(/[\\_%]/g, '\\$&') + '\\_%'
}

function tokenize(pattern: string): Token[] | null {
  const tokens: Token[] = []
  let escaped = false
  let wildcard = false
  for (const ch of pattern) {
    if (ch === '\0') return null
    if (escaped) {
      if (ch !== '\\' && ch !== '%' && ch !== '_') return null
      tokens.push(ch)
      escaped = false
    } else if (ch === '\\') {
      escaped = true
    } else if (ch === '%' || ch === '_') {
      wildcard = true
      if (ch === '_') tokens.push(one)
      else if (tokens.at(-1) !== any) tokens.push(any)
    } else {
      tokens.push(ch)
    }
  }
  return wildcard && !escaped ? tokens : null
}

export function validGrantPattern(pattern: string): boolean {
  return tokenize(pattern) !== null
}

// Match Unicode code points with bounded work per token and name character.
// A row holds the matches for every prefix of this name after the last token.
export function compileGrantPattern(pattern: string): ((name: string) => boolean) | null {
  const tokens = tokenize(pattern)
  if (!tokens) return null
  const minimumLength = tokens.filter(t => t !== any).length
  return (name: string) => {
    const chars = Array.from(name)
    if (chars.length < minimumLength) return false
    const row = new Uint8Array(chars.length + 1)
    row[0] = 1
    for (const token of tokens) {
      if (token === any) {
        for (let i = 1; i <= chars.length; i++) row[i] |= row[i - 1]
      } else {
        for (let i = chars.length; i > 0; i--) {
          row[i] = token === one || token === chars[i - 1] ? row[i - 1] : 0
        }
        row[0] = 0
      }
    }
    return row[chars.length] === 1
  }
}
