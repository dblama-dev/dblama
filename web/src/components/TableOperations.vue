<template>
  <section class="table-operations">
    <p>{{ db }} / {{ table }}</p>
    <n-alert v-if="error" type="error">{{ error }}</n-alert>
    <n-popconfirm @positive-click="run('truncate')">
      <template #trigger><n-button :disabled="busy || isView" :loading="busy">{{ t('navigationWorkflows.truncate') }}</n-button></template>
      {{ t('navigationWorkflows.confirmTruncate', {names:`${db}.${table}`}) }}
    </n-popconfirm>
    <n-popconfirm @positive-click="run('drop')">
      <template #trigger><n-button type="error" :disabled="busy" :loading="busy">{{ t('navigationWorkflows.drop') }}</n-button></template>
      {{ t('navigationWorkflows.confirmDrop', {names:`${db}.${table}`}) }}
    </n-popconfirm>
  </section>
</template>
<script setup lang="ts">
import {computed,ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {NAlert,NButton,NPopconfirm} from 'naive-ui'
import {api} from '../api/client'
import {useCatalogStore} from '../stores/catalog'
const props=defineProps<{db:string;table:string}>()
const emit=defineEmits<{changed:[];dropped:[]}>()
const {t}=useI18n()
const catalog=useCatalogStore()
const busy=ref(false)
const error=ref('')
const isView=computed(()=>catalog.tables.find(table=>table.name===props.table)?.isView)
async function run(action:'truncate'|'drop'):Promise<void>{
 if(busy.value || (action==='truncate' && isView.value))return
 busy.value=true;error.value=''
 try{
  const path=`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}`
  const suffix=action==='truncate'?'/truncate':isView.value?'?isView=true':''
  await api(action==='truncate'?'POST':'DELETE',catalog.path(path+suffix))
  await catalog.invalidate(props.db)
  if(action==='drop')emit('dropped');else emit('changed')
 }catch(err){error.value=err instanceof Error?err.message:String(err)}finally{busy.value=false}
}
</script>
<style scoped>.table-operations{display:flex;align-items:center;gap:12px;flex-wrap:wrap;}</style>
