<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAutoComplete, NInput, NInputNumber } from 'naive-ui'

import { useMetadataStore } from '../../stores/metadata'
import type { TableOptions } from '../../api/types'

// Table options editor: engine, collation, comment and the AUTO_INCREMENT
// counter. The state lives in the parent (StructureEditor) — everything is
// sent in the same PUT as the columns.

const props = defineProps<{ options: TableOptions }>()
const emit = defineEmits<{ 'update:options': [value: TableOptions] }>()

const { t } = useI18n()
const metadata = useMetadataStore()
void metadata.load()

// Local writable mirror so naive-ui inputs can bind directly; every change
// is re-emitted to the parent.
const model = computed(() => props.options)

function update(patch: Partial<TableOptions>): void {
  emit('update:options', { ...model.value, ...patch })
}
</script>

<template>
  <div class="struct-options">
    <label class="struct-field struct-field--engine">
      <span class="struct-field__label">{{ t('structure.engine') }}</span>
      <NInput
        :value="model.engine ?? ''"
        @update:value="update({ engine: $event })"
      />
    </label>
    <label class="struct-field struct-field--collation">
      <span class="struct-field__label">{{ t('structure.collation') }}</span>
      <NAutoComplete
        :options="metadata.data?.collations.map(c => c.name) ?? []"
        :value="model.collation ?? ''"
        @update:value="update({ collation: $event })"
      />
    </label>
    <label class="struct-field struct-field--comment">
      <span class="struct-field__label">{{ t('structure.comment') }}</span>
      <NInput
        :value="model.comment ?? ''"
        @update:value="update({ comment: $event })"
      />
    </label>
    <label class="struct-field struct-field--autoinc">
      <span class="struct-field__label">{{ t('structure.tableAutoIncrement') }}</span>
      <NInputNumber
        :value="model.autoIncrement ?? null"
        :min="0"
        @update:value="update({ autoIncrement: $event ?? undefined })"
      />
    </label>
  </div>
</template>

<style scoped>
.struct-options {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.struct-options .struct-field {
  min-width: 200px;
  flex: 1;
}
</style>
