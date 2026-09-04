<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  results: Record<string, any>
  loading: boolean
}>()

// 将检查结果转换为表格行，字符串字段（如 container_backend）显示实际值而非通过/未通过
const rows = computed(() => {
  return Object.entries(props.results).map(([check, v]) => {
    const isBool = typeof v === 'boolean'
    return {
      check,
      passed: isBool ? v : v !== 'none' && v !== '',
      display: isBool ? (v ? '通过' : '未通过') : String(v),
      isBool,
    }
  })
})
</script>

<template>
  <div style="padding: 40px 20px;">
    <h3 style="margin-bottom: 20px;">环境检查</h3>
    <div v-if="loading" style="text-align: center; padding: 40px;">
      <el-icon class="is-loading" :size="32"><Loading /></el-icon>
      <p style="margin-top: 12px; color: #909399;">正在检查环境...</p>
    </div>
    <div v-else>
      <el-table :data="rows" stripe>
        <el-table-column prop="check" label="检查项" />
        <el-table-column label="结果" width="140">
          <template #default="{ row }">
            <el-tag :type="row.passed ? 'success' : 'danger'" size="small">
              {{ row.display }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>