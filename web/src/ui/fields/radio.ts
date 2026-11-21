import type { InjectionKey } from 'vue'
import type { OptionValue } from './attributes'

export const radioGroupKey: InjectionKey<{
  name: string
  readonly value: OptionValue | undefined
  readonly disabled: boolean
  choose: (value: OptionValue) => void
}> = Symbol('radioGroup')
