<template>
  <section class="table-operations">
    <ui-alert v-if="error" type="error">{{ error }}</ui-alert>
    <ui-alert v-if="isView === null" type="info">{{ t('navigationWorkflows.kindUnknown') }}</ui-alert>
    <div class="table-operation"><span>{{ t('rowWorkflows.cleanRows') }}</span>
    <ui-popconfirm @positive-click="run('truncate')">
      <template #trigger><ui-button :disabled="busy || isView !== false" :loading="busy">{{ t('navigationWorkflows.truncate') }}</ui-button></template>
      {{ t('navigationWorkflows.confirmTruncate', {names:`${db}.${table}`}) }}
    </ui-popconfirm>
    </div>
    <div class="table-operation"><span>{{ t(isView === true ? 'rowWorkflows.dropView' : isView === false ? 'rowWorkflows.dropTable' : 'rowWorkflows.dropObject') }}</span>
    <ui-popconfirm @positive-click="run('drop')">
      <template #trigger><ui-button type="error" :disabled="busy || isView === null" :loading="busy">{{ t('navigationWorkflows.drop') }}</ui-button></template>
      {{ t('navigationWorkflows.confirmDrop', {names:`${db}.${table}`}) }}
    </ui-popconfirm>
    </div>
  </section>
</template>
<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {UiAlert,UiButton,UiPopconfirm} from '../ui'
import {api} from '../api/client'
import {useCatalogStore} from '../stores/catalog'
import type {TableInfo} from '../api/types'
const props=defineProps<{db:string;table:string}>()
const emit=defineEmits<{changed:[];dropped:[]}>()
const {t}=useI18n()
const catalog=useCatalogStore()
const busy=ref(false)
const error=ref('')
const isView=ref<boolean|null>(null)
let resolvedPath:string|null=null
const objectPath=computed(()=>catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}`))
let generation=0
watch([objectPath,()=>catalog.schemaAware],async()=>{
 const current=++generation
 const scope=objectPath.value
 const path=catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables`)
 const table=props.table
 isView.value=null;resolvedPath=null;error.value='';busy.value=false
 try{
  const body=await api<{tables:TableInfo[]}>('GET',path)
  if(current!==generation || scope!==objectPath.value)return
  const object=body.tables.find(item=>item.name===table)
  isView.value=typeof object?.isView==='boolean'?object.isView:null
  resolvedPath=scope
 }catch(err){if(current===generation)error.value=err instanceof Error?err.message:String(err)}
},{immediate:true})
onBeforeUnmount(()=>{generation++})
async function run(action:'truncate'|'drop'):Promise<void>{
 if(busy.value || resolvedPath!==objectPath.value || isView.value===null || (action==='truncate' && isView.value))return
 const current=generation
 const scope=objectPath.value
 const db=props.db
 const path=`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}`
 busy.value=true;error.value=''
 try{
  const suffix=action==='truncate'?'/truncate':isView.value?'?isView=true':''
  await api(action==='truncate'?'POST':'DELETE',catalog.path(path+suffix))
  if(current!==generation || scope!==objectPath.value)return
  await catalog.invalidate(db)
  if(current!==generation || scope!==objectPath.value)return
  if(action==='drop')emit('dropped');else emit('changed')
 }catch(err){if(current===generation)error.value=err instanceof Error?err.message:String(err)}finally{if(current===generation)busy.value=false}
}
</script>
<style scoped>
.table-operations { border:1px solid var(--db-line); border-radius:10px; overflow:hidden; }
.table-operation { display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; padding:14px 16px; color:var(--db-ink); }
.table-operation + .table-operation { border-top:1px solid var(--db-line); }
</style>
