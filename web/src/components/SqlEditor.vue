<template><div ref="host" class="sql-editor"></div></template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { indentWithTab } from '@codemirror/commands'
import { sql, MySQL, MariaSQL, PostgreSQL, type SQLNamespace } from '@codemirror/lang-sql'
import { basicSetup } from 'codemirror'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { useThemeVars } from 'naive-ui'
const props = withDefaults(defineProps<{modelValue:string;dialect?:string;schema?:SQLNamespace;defaultTable?:string;disabled?:boolean;label?:string}>(),{dialect:'mysql',label:'SQL'})
const emit = defineEmits<{'update:modelValue':[value:string];execute:[selection:string|undefined];history:[direction:'older'|'newer']}>()
const host = ref<HTMLElement|null>(null)
const themeVars = useThemeVars()
const dark = computed(() => themeVars.value.baseColor.toLowerCase() === '#000')
let editor: EditorView | null = null
const language = new Compartment(), editable = new Compartment(), attributes = new Compartment(), appearance = new Compartment()
function appearanceConfig() {
  const colors = themeVars.value
  const palette = dark.value
    ? { keyword: '#c792ea', string: '#a5d6a7', number: '#f7c783', comment: '#a9b1bb', type: '#89c7ee' }
    : { keyword: '#663399', string: '#116329', number: '#865200', comment: '#57606a', type: '#0550ae' }
  return [EditorView.theme({
    '&': { color: colors.textColor1, backgroundColor: colors.bodyColor },
    '.cm-content': { caretColor: colors.textColor1 },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: colors.textColor1 },
    '.cm-gutters': { backgroundColor: colors.cardColor, color: colors.textColor2, borderColor: colors.borderColor },
    '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: colors.hoverColor },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': { backgroundColor: dark.value ? '#355568' : '#cce2f5' },
    '.cm-tooltip': { backgroundColor: colors.popoverColor, color: colors.textColor1, borderColor: colors.borderColor },
  }, { dark: dark.value }), syntaxHighlighting(HighlightStyle.define([
    { tag: tags.keyword, color: palette.keyword },
    { tag: [tags.string, tags.special(tags.string)], color: palette.string },
    { tag: [tags.number, tags.bool, tags.null], color: palette.number },
    { tag: tags.comment, color: palette.comment, fontStyle: 'italic' },
    { tag: [tags.typeName, tags.namespace], color: palette.type },
  ]))]
}
function languageConfig() { return sql({dialect:props.dialect === 'postgres' ? PostgreSQL : props.dialect === 'maria' ? MariaSQL : MySQL,schema:props.schema,defaultTable:props.defaultTable,upperCaseKeywords:true}) }
function selection(): string | undefined {
  if (!editor) return undefined
  const range = editor.state.selection.main
  return range.empty ? undefined : editor.state.sliceDoc(range.from,range.to)
}
function replace(text:string) { if (editor && editor.state.doc.toString() !== text) editor.dispatch({changes:{from:0,to:editor.state.doc.length,insert:text}}) }
defineExpose({selection,focus:()=>editor?.focus()})
onMounted(()=>{
  if (!host.value) return
  editor = new EditorView({parent:host.value,state:EditorState.create({doc:props.modelValue,extensions:[
    keymap.of([
      {key:'Mod-Enter',run:()=>{if (!props.disabled) emit('execute',selection());return true}},
      {key:'Alt-ArrowUp',run:()=>{emit('history','older');return true}},
      {key:'Alt-ArrowDown',run:()=>{emit('history','newer');return true}},
      indentWithTab,
    ]),basicSetup,language.of(languageConfig()),editable.of(EditorView.editable.of(!props.disabled)),appearance.of(appearanceConfig()),
    attributes.of(EditorView.contentAttributes.of({'aria-label':props.label,'data-source':''})),
    EditorView.updateListener.of(update=>{if(update.docChanged) emit('update:modelValue',update.state.doc.toString())}),
  ]})})
})
watch(()=>props.modelValue,replace)
watch(()=>[props.dialect,props.schema,props.defaultTable],()=>editor?.dispatch({effects:language.reconfigure(languageConfig())}),{deep:true})
watch(()=>props.disabled,()=>editor?.dispatch({effects:editable.reconfigure(EditorView.editable.of(!props.disabled))}))
watch(()=>props.label,()=>editor?.dispatch({effects:attributes.reconfigure(EditorView.contentAttributes.of({'aria-label':props.label,'data-source':''}))}))
watch(themeVars,()=>editor?.dispatch({effects:appearance.reconfigure(appearanceConfig())}))
onBeforeUnmount(()=>{editor?.destroy();editor=null})
</script>
<style scoped>
.sql-editor { border:1px solid #8885;border-radius:5px;overflow:hidden;min-width:0; }
.sql-editor :deep(.cm-editor) { min-height:180px;max-height:45vh;font-size:13px; }
.sql-editor :deep(.cm-scroller) { overflow:auto; }
.sql-editor :deep(.cm-editor.cm-focused) { outline:2px solid #18a05855;outline-offset:-2px; }
</style>
