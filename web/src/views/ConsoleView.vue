<template>
  <section class="console">
    <div class="console__toolbar">
      <n-button size="small" type="primary" :loading="c.executing.value" @click="runAll">
        {{ t('console.executeAll') }}
      </n-button>
      <n-button size="small" :disabled="c.executing.value" @click="runSelection">
        {{ t('console.executeSelection') }}
      </n-button>
      <n-button size="small" :disabled="c.executing.value" @click="runExplain">
        {{ t('console.explain') }}
      </n-button>
      <n-button v-if="c.executing.value" size="small" type="error" @click="cancelRun">
        {{ t('console.cancel') }}
      </n-button>
      <!-- TODO(P5): SQL formatter button; the i18n key console.format is
           already reserved (no external formatter library in П3). -->
      <n-select
        class="console__history"
        size="small"
        clearable
        :value="null"
        :options="historyOptions"
        :placeholder="t('console.history')"
        @update:value="onHistoryPick"
      />
    </div>

    <div ref="editorHost" class="console__editor"></div>

    <n-alert v-if="c.error.value" type="error" class="console__error">
      {{ c.error.value }}
    </n-alert>

    <template v-if="c.results.value !== null">
      <p v-if="c.results.value.length === 0" class="console__empty">
        {{ t('console.results') }}: 0
      </p>
      <ResultGrid
        v-for="(res, i) in c.results.value"
        :key="i"
        :result="res"
        :index="i + 1"
      />
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { indentWithTab } from '@codemirror/commands'
import { sql as sqlLanguage } from '@codemirror/lang-sql'
import { EditorState, type EditorView as CMEditorView } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { basicSetup } from 'codemirror'
import type { SelectOption } from 'naive-ui'
import { NAlert, NButton, NSelect } from 'naive-ui'

import ResultGrid from '../components/ResultGrid.vue'
import { useConsole } from '../lib/useConsole'
import { useSessionStore } from '../stores/session'

const { t } = useI18n()
const route = useRoute()
const session = useSessionStore()

// History is scoped per connected server. The session carries the
// driver flavor and host (no port), which identifies the single live
// connection this app can have.
const serverKey = computed(() => {
  const driver = session.sessionInfo?.driver
  return driver ? `${driver.flavor}|${driver.host}` : 'unknown'
})

const c = useConsole({
  db: () => String(route.params.db ?? ''),
  serverKey: serverKey.value,
})

// --- CodeMirror editor -------------------------------------------------

const editorHost = ref<HTMLElement | null>(null)
let editor: EditorView | null = null

/** Cycles the history into the editor: Up walks to older entries, Down
 * back to newer ones; null (start of list) keeps the current text. */
function historyCommand(direction: 'older' | 'newer'): (target: CMEditorView) => boolean {
  return (target) => {
    const entry = direction === 'older' ? c.historyPrev() : c.historyNext()
    if (entry !== null) {
      replaceDoc(target, entry)
    }
    return true
  }
}

function replaceDoc(target: CMEditorView, text: string): void {
  target.dispatch({ changes: { from: 0, to: target.state.doc.length, insert: text } })
}

onMounted(() => {
  if (editorHost.value === null) return
  editor = new EditorView({
    parent: editorHost.value,
    state: EditorState.create({
      doc: c.sql.value,
      // basicSetup brings line numbers, undo history and the standard
      // keymap; lang-sql offers uppercase keyword completions (its own
      // default is lowercase).
      extensions: [
        keymap.of([indentWithTab]),
        basicSetup,
        sqlLanguage({ upperCaseKeywords: true }),
        keymap.of([
          { key: 'Alt-ArrowUp', run: historyCommand('older') },
          { key: 'Alt-ArrowDown', run: historyCommand('newer') },
        ]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) c.sql.value = update.state.doc.toString()
        }),
      ],
    }),
  })
})

onBeforeUnmount(() => {
  editor?.destroy()
  editor = null
})

function selection(): string | undefined {
  if (editor === null) return undefined
  const range = editor.state.selection.main
  return range.from === range.to ? undefined : editor.state.sliceDoc(range.from, range.to)
}

// --- Toolbar actions ---------------------------------------------------

function runAll(): void {
  void c.execute()
}

function runSelection(): void {
  void c.execute(selection())
}

function runExplain(): void {
  void c.explain(selection())
}

function cancelRun(): void {
  // A failed cancel (e.g. the run just finished) must not surface as an
  // unhandled rejection: the execute() promise reports the outcome.
  c.cancel().catch(() => {})
}

const historyOptions = computed<SelectOption[]>(() =>
  c.history.value.map((entry, i) => ({ label: entry, value: String(i) })),
)

function onHistoryPick(value: string | null): void {
  if (value === null || editor === null) return
  const entry = c.history.value[Number(value)]
  if (entry !== undefined) {
    // A pick replaces the doc from outside the Alt+Up/Down keys, so any
    // in-progress cycle (and its draft snapshot) is dropped first.
    c.resetHistoryCycle()
    replaceDoc(editor, entry)
  }
}
</script>

<style scoped>
.console {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.console__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.console__history {
  min-width: 200px;
  max-width: 360px;
  margin-left: auto;
}

.console__editor {
  border: 1px solid rgba(128, 128, 128, 0.35);
  border-radius: 3px;
  overflow: hidden;
}

.console__editor :deep(.cm-editor) {
  min-height: 180px;
  max-height: 40vh;
  font-size: 13px;
}

.console__editor :deep(.cm-editor.cm-focused) {
  outline: none;
}

.console__empty {
  opacity: 0.7;
}
</style>
