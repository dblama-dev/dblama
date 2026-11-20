import { describe, expect, it } from 'vitest'
import { compileGrantPattern, usernamePrefixPattern, validGrantPattern } from './grantPattern'

describe('grant name patterns', () => {
  it('escapes every wildcard and backslash in a username and the prefix separator', () => {
    const pattern = usernamePrefixPattern('д_%\\')
    expect(pattern).toBe('д\\_\\%\\\\\\_%')
    const matches = compileGrantPattern(pattern)!
    expect(matches('д_%\\_данные')).toBe(true)
    expect(matches('д_%\\Xданные')).toBe(false)
  })

  it.each([
    ['a%_b', true], ['a\\%_b', true], ['a\\\\%b', true],
    ['a\\%b\\_', false], ['', false], ['exact', false],
    ['bad%\\', false], ['bad%\\x', false], ['bad%\0', false],
  ] as const)('validates %j', (pattern, valid) => {
    expect(validGrantPattern(pattern)).toBe(valid)
  })

  it.each([
    ['a🪐z', 'a_z', true], ['a🪐🪐z', 'a_z', false], ['az', 'a_z', false],
    ['za', '%a%', true], ['a', '%a%', true], ['', '%', true],
    ['B', 'b%', false], ['exact_tail', 'act%', false], ['x\nz', 'x%z', true],
    ['a%b_c\\d', 'a\\%b\\_c\\\\%', true], ['abc', '%\\q', false],
    ['a'.repeat(64), '%a'.repeat(30) + 'b%', false],
  ] as const)('matches the full name %j against %j', (name, pattern, matches) => {
    expect(compileGrantPattern(pattern)?.(name) ?? false).toBe(matches)
  })
})
