<template><div ref="host" class="sql-editor"></div></template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { indentWithTab } from '@codemirror/commands'
import { sql, MySQL, MariaSQL, PostgreSQL, type SQLNamespace } from '@codemirror/lang-sql'
import { basicSetup } from 'codemirror'
const props = withDefaults(defineProps<{modelValue:string;dialect?:string;schema?:SQLNamespace;defaultTable?:string;disabled?:boolean;label?:string}>(),{dialect:'mysql',label:'SQL'})
const emit = defineEmits<{'update:modelValue':[value:string];execute:[selection:string|undefined];history:[direction:'older'|'newer']}>()
const host = ref<HTMLElement|null>(null)
let editor: EditorView | null = null
const language = new Compartment(), editable = new Compartment(), attributes = new Compartment()
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
    ]),basicSetup,language.of(languageConfig()),editable.of(EditorView.editable.of(!props.disabled)),
    attributes.of(EditorView.contentAttributes.of({'aria-label':props.label,'data-source':''})),
    EditorView.updateListener.of(update=>{if(update.docChanged) emit('update:modelValue',update.state.doc.toString())}),
  ]})})
})
watch(()=>props.modelValue,replace)
watch(()=>[props.dialect,props.schema,props.defaultTable],()=>editor?.dispatch({effects:language.reconfigure(languageConfig())}),{deep:true})
watch(()=>props.disabled,()=>editor?.dispatch({effects:editable.reconfigure(EditorView.editable.of(!props.disabled))}))
watch(()=>props.label,()=>editor?.dispatch({effects:attributes.reconfigure(EditorView.contentAttributes.of({'aria-label':props.label,'data-source':''}))}))
onBeforeUnmount(()=>{editor?.destroy();editor=null})
</script>
<style scoped>
.sql-editor { border:1px solid #8885;border-radius:5px;overflow:hidden;min-width:0; }
.sql-editor :deep(.cm-editor) { min-height:180px;max-height:45vh;font-size:13px; }
.sql-editor :deep(.cm-scroller) { overflow:auto; }
.sql-editor :deep(.cm-editor.cm-focused) { outline:2px solid #18a05855;outline-offset:-2px; }
</style>
