import { inject, onBeforeUnmount, onMounted } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, routerKey } from 'vue-router'

export function useDirtyGuard(dirty: () => boolean, message: () => string): () => boolean {
  const confirm = () => !dirty() || window.confirm(message())
  if (inject(routerKey,null)) {
    onBeforeRouteLeave(confirm)
    onBeforeRouteUpdate(confirm)
  }
  function beforeUnload(event: BeforeUnloadEvent) {
    if (dirty()) {event.preventDefault();event.returnValue=''}
  }
  onMounted(()=>window.addEventListener('beforeunload',beforeUnload))
  onBeforeUnmount(()=>window.removeEventListener('beforeunload',beforeUnload))
  return confirm
}
