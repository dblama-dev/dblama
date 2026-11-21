import { computed, shallowRef } from 'vue'

const dialogs = shallowRef<HTMLDialogElement[]>([])
let originalOverflow = ''
export const topDialog = computed(() => dialogs.value.at(-1))

export function addDialog(dialog: HTMLDialogElement) {
  if (!dialogs.value.length) {
    originalOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  }
  dialogs.value = [...dialogs.value, dialog]
}

export function removeDialog(dialog: HTMLDialogElement) {
  if (!dialogs.value.includes(dialog)) return false
  const wasTop = topDialog.value === dialog
  dialogs.value = dialogs.value.filter(item => item !== dialog)
  if (!dialogs.value.length) document.body.style.overflow = originalOverflow
  return wasTop
}

export function keepTabInside(event: KeyboardEvent, root: HTMLElement) {
  const candidates = root.querySelectorAll<HTMLElement>('button, input, select, textarea, a[href], [tabindex]')
  const controls = Array.from(candidates).filter(item => item.tabIndex >= 0 && !item.matches(':disabled') &&
    item.getClientRects().length && !item.closest('[hidden], [inert]'))
  const first = controls[0], last = controls.at(-1)
  if (!first || !last) {
    event.preventDefault()
    root.focus()
    return
  }
  if (event.shiftKey && (document.activeElement === first || document.activeElement === root)) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
