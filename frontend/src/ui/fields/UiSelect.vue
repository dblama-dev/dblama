<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import type { CSSProperties, InputHTMLAttributes } from 'vue'
import UiIcon from '../../components/UiIcon.vue'
import i18n from '../../i18n'
import { containerAttrs, controlAttrs, type ControlSize, type OptionValue, type SelectOption } from './attributes'

defineOptions({ inheritAttrs: false })
const props = withDefaults(defineProps<{
  value?: OptionValue | OptionValue[] | null
  options?: SelectOption[]
  filterable?: boolean
  remote?: boolean
  tag?: boolean
  multiple?: boolean
  clearable?: boolean
  loading?: boolean
  disabled?: boolean
  placeholder?: string
  inputProps?: InputHTMLAttributes
  size?: ControlSize
}>(), { options: () => [] })
const emit = defineEmits(['update:value', 'search'])
const { t } = i18n.global
const custom = computed(() => props.filterable || props.multiple || props.remote || props.tag)
const selected = computed<OptionValue[]>(() => Array.isArray(props.value) ? props.value : props.value == null ? [] : [props.value])
const selectedText = computed(() => selected.value.map(labelFor).join(', '))
const root = ref<HTMLElement>()
const field = ref<HTMLInputElement | HTMLSelectElement>()
const menu = ref<HTMLElement>()
const menuId = useId()
const listLabel = ref('')
const listLabelledBy = ref('')
const open = ref(false)
const query = ref('')
const activeIndex = ref(-1)
let navigated = false
const popupParent = ref<Element | string>('body')
const position = ref<CSSProperties>({})

function labelFor(value: OptionValue) {
  return props.options.find(option => option.value === value)?.label ?? String(value)
}
const matches = computed<SelectOption[]>(() => {
  const term = query.value.toLocaleLowerCase()
  const remoteTag = props.remote && props.tag
  const options = !term || (props.remote && !props.tag) ? props.options : props.options.filter(option =>
    option.label.toLocaleLowerCase().includes(term) || (remoteTag ? String(option.value) === query.value : String(option.value).toLocaleLowerCase().includes(term)),
  )
  if (props.tag && query.value && !options.some(option => String(option.value) === query.value || option.label === query.value)) {
    return [...options, { label: query.value, value: query.value }]
  }
  return options
})
function change(value: OptionValue | OptionValue[] | null) {
  emit('update:value', value)
}
function pick(option: SelectOption) {
  if (props.disabled || option.disabled) return
  if (props.multiple) {
    const previous = selected.value
    change(previous.includes(option.value) ? previous.filter(value => value !== option.value) : [...previous, option.value])
    search('')
    field.value?.focus()
  } else {
    change(option.value)
    close()
  }
}
function clear() {
  if (props.disabled) return
  change(props.multiple ? [] : null)
  search('')
  field.value?.focus()
}
function nativeChange(event: Event) {
  const select = event.target as HTMLSelectElement
  const index = select.selectedOptions[0]?.dataset.index
  if (index !== undefined) change(props.options[Number(index)]!.value)
  else if (props.clearable) change(null)
  // Some consumers treat a selection as an action and leave value unchanged.
  void nextTick(() => syncNative(select))
}
function syncNative(select: HTMLSelectElement) {
  const wanted = props.options.findIndex(option => option.value === props.value)
  const index = Array.from(select.options).findIndex(option => wanted < 0 ? option.dataset.index === undefined : option.dataset.index === String(wanted))
  select.selectedIndex = index
}
function firstMatch(choices: SelectOption[]) {
  if (props.remote && props.tag && query.value) {
    const exact = choices.findIndex(option => !option.disabled && String(option.value) === query.value &&
      (props.options.includes(option) || /^[+-]?\d+$/.test(query.value)))
    if (exact >= 0) return exact
  }
  return choices.findIndex(option => !option.disabled)
}
function search(value: string) {
  query.value = value
  navigated = false
  activeIndex.value = firstMatch(matches.value)
  emit('search', value)
}
function reposition() {
  if (!open.value || !root.value) return
  const rect = root.value.getBoundingClientRect()
  const gap = 4
  const margin = 8
  const below = window.innerHeight - rect.bottom - margin - gap
  const above = rect.top - margin - gap
  const upwards = below < 200 && above > below
  const width = Math.min(Math.max(rect.width, 150), window.innerWidth - 2 * margin)
  position.value = {
    left: Math.max(margin, Math.min(rect.left, window.innerWidth - width - margin)) + 'px',
    width: width + 'px',
    maxHeight: Math.max(40, Math.min(300, upwards ? above : below)) + 'px',
    ...(upwards ? { bottom: window.innerHeight - rect.top + gap + 'px' } : { top: rect.bottom + gap + 'px' }),
  }
}
function labelList() {
  const input = field.value
  if (!input) return
  listLabelledBy.value = input.getAttribute('aria-labelledby') || ''
  listLabel.value = input.getAttribute('aria-label') || ''
  if (listLabelledBy.value || listLabel.value) return
  listLabelledBy.value = Array.from(input.labels ?? [], (label, index) => {
    if (!label.id) label.id = `${menuId}-label-${index}`
    return label.id
  }).join(' ')
  if (!listLabelledBy.value) listLabel.value = props.placeholder || t('controls.choose')
}
function openMenu() {
  if (!custom.value || open.value || props.disabled) return
  labelList()
  popupParent.value = root.value?.closest('.ui-modal, dialog[open]') ?? 'body'
  query.value = ''
  navigated = false
  open.value = true
  activeIndex.value = matches.value.findIndex(option => !option.disabled && selected.value.includes(option.value))
  if (activeIndex.value < 0) activeIndex.value = matches.value.findIndex(option => !option.disabled)
  reposition()
  if (props.remote) emit('search', '')
}
function close() {
  open.value = false
  query.value = ''
}
function outside(event: Event) {
  const target = event.target as Node
  if (!root.value?.contains(target) && !menu.value?.contains(target)) close()
}
function listen(enabled: boolean) {
  if (enabled) {
    document.addEventListener('pointerdown', outside, true)
    document.addEventListener('focusin', outside, true)
    window.addEventListener('resize', reposition)
    document.addEventListener('scroll', reposition, true)
  } else {
    document.removeEventListener('pointerdown', outside, true)
    document.removeEventListener('focusin', outside, true)
    window.removeEventListener('resize', reposition)
    document.removeEventListener('scroll', reposition, true)
  }
}
function move(delta: number) {
  const choices = matches.value
  for (let step = 1; step <= choices.length; step++) {
    const index = (activeIndex.value + delta * step + choices.length * 2) % choices.length
    if (!choices[index]?.disabled) {
      navigated = true
      activeIndex.value = index
      void nextTick(() => document.getElementById(menuId + '-' + index)?.scrollIntoView?.({ block: 'nearest' }))
      return
    }
  }
}
function point(index: number, option: SelectOption) {
  if (!option.disabled) { navigated = true; activeIndex.value = index }
}
function handleKey(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    event.stopPropagation()
    close()
  } else if (event.key === 'Tab') {
    close()
  } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    if (open.value) move(event.key === 'ArrowDown' ? 1 : -1)
    else openMenu()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    if (!open.value) openMenu()
    else if (matches.value[activeIndex.value]) pick(matches.value[activeIndex.value]!)
  } else if (event.key === 'Backspace' && props.multiple && !query.value) {
    change(selected.value.slice(0, -1))
  }
}
watch(matches, choices => {
  if ((!navigated && props.remote && props.tag && query.value) || activeIndex.value < 0 || activeIndex.value >= choices.length || choices[activeIndex.value]?.disabled) {
    activeIndex.value = firstMatch(choices)
  }
})
watch(() => props.disabled, disabled => { if (disabled) close() })
watch(open, listen, { flush: 'sync' })
watch([() => props.value, () => props.options, field], () => {
  if (field.value instanceof HTMLSelectElement) syncNative(field.value)
}, { flush: 'post' })
onBeforeUnmount(() => listen(false))
defineExpose({ focus: () => field.value?.focus(), blur: () => field.value?.blur() })
</script>

<template>
  <span ref="root" v-bind="containerAttrs($attrs)" class="ui-select" :data-size="size" :data-disabled="disabled || undefined"
    @click="custom && (field?.focus(), openMenu())">
    <select v-if="!custom" ref="field" v-bind="{ ...controlAttrs($attrs), ...inputProps }" class="ui-select__control"
      :disabled="disabled" :aria-busy="loading || undefined" @change="nativeChange">
      <option v-if="value == null || clearable || !options.some(option => option.value === value)"
        value="" :hidden="!clearable" :disabled="!clearable">{{ value != null && !options.some(option => option.value === value) ? selectedText : placeholder || t('controls.choose') }}</option>
      <option v-for="(option, index) in options" :key="option.value" :value="option.value" :data-index="index"
        :disabled="option.disabled" :selected="option.value === value">{{ option.label }}</option>
    </select>
    <template v-else>
      <span v-if="multiple && selected.length" class="ui-select__choices">
        <span v-for="item in selected" :key="item" class="ui-select__choice">{{ labelFor(item) }}</span>
      </span>
      <input ref="field" v-bind="{ ...controlAttrs($attrs), ...inputProps }"
        class="ui-select__control" role="combobox" aria-haspopup="listbox" aria-autocomplete="list"
        :name="undefined" :value="open ? query : multiple ? '' : selectedText"
        :placeholder="open && !multiple && selected.length ? selectedText : multiple && selected.length ? '' : placeholder || t('controls.choose')"
        :disabled="disabled" :aria-controls="open ? menuId : undefined" :aria-expanded="open" :aria-busy="loading || undefined"
        :aria-activedescendant="open && activeIndex >= 0 ? menuId + '-' + activeIndex : undefined"
        autocomplete="off" @click="openMenu" @input="openMenu(); search(($event.target as HTMLInputElement).value)" @keydown="handleKey">
      <template v-if="$attrs.name">
        <input v-for="item in selected" :key="item" type="hidden" :name="String($attrs.name)" :value="item" :disabled="disabled">
      </template>
    </template>
    <button v-if="clearable && selected.length && !disabled" class="ui-select__clear" type="button" :aria-label="t('controls.clear')"
      @mousedown.prevent @click.prevent.stop="clear"><UiIcon name="x" :size="13" /></button>
    <UiIcon class="ui-select__arrow" name="chevron-down" :size="13" />
    <Teleport v-if="open" :to="popupParent">
      <ul :id="menuId" ref="menu" class="ui-select__list" :style="position" role="listbox"
        :aria-label="listLabel || undefined" :aria-labelledby="listLabelledBy || undefined" :aria-multiselectable="multiple || undefined">
        <li v-for="(option, index) in matches" :id="menuId + '-' + index" :key="option.value" class="ui-select__option" role="option"
          :aria-selected="selected.includes(option.value)" :aria-disabled="option.disabled || undefined" :data-active="activeIndex === index || undefined"
          @pointermove="point(index, option)" @mousedown.prevent @click="pick(option)">
          <span>{{ option.label }}</span><UiIcon v-if="selected.includes(option.value)" name="check" :size="14" />
        </li>
        <li v-if="!matches.length" class="ui-select__empty" role="presentation">{{ t(loading ? 'shell.loading' : 'shell.noResults') }}</li>
      </ul>
    </Teleport>
  </span>
</template>
