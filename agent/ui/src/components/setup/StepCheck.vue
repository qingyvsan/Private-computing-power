<script setup lang="ts">
defineProps<{
  results: Record<string, boolean>
  loading: boolean
}>()
</script>

<template>
  <div style="padding: 40px 20px;">
    <h3 style="margin-bottom: 20px;">环境检查</h3>
    <div v-if="loading" style="text-align: center; padding: 40px;">
      <el-icon class="is-loading" :size="32"><Loading /></el-icon>
      <p style="margin-top: 12px; color: #909399;">正在检查环境...</p>
    </div>
    <div v-else>
      <el-table :data="Object.entries(results).map(([k, v]) => ({ check: k, passed: v }))" stripe>
        <el-table-column prop="check" label="检查项" />
        <el-table-column label="结果" width="120">
          <template #default="{ row }">
            <el-tag :type="row.passed ? 'success' : 'danger'" size="small">
              {{ row.passed ? '通过' : '未通过' }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>