// Layout classes and test handles belong to the wrapper; form, keyboard and
// accessibility attributes belong to the actual focusable element.
export function containerAttrs(attrs: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(attrs).filter(([key]) =>
    key === 'class' || key === 'style' || key.startsWith('data-'),
  ))
}

export function controlAttrs(attrs: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(attrs).filter(([key]) =>
    key !== 'class' && key !== 'style' && !key.startsWith('data-'),
  ))
}

export type ControlSize = 'tiny' | 'small' | 'medium' | 'large'
export type OptionValue = string | number
export interface SelectOption {
  label: string
  value: OptionValue
  disabled?: boolean
}
