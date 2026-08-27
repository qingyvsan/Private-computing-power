<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { listJobs, type Job } from '../api/client'
import { statusType, formatTime } from '../utils/format'

const router = useRouter()
const jobs = ref<Job[]>([])
const totalCount = ref(0)
const loading = ref(true)
const statusFilter = ref('')

onMounted(async () => {
  try {
    const resp = await listJobs()
    jobs.value = resp.jobs
    totalCount.value = resp.total_count
  } catch {}
  loading.value = false
})

const filteredJobs = computed(() => {
  if (!statusFilter.value) return jobs.value
  return jobs.value.filter(j => j.status === statusFilter.value)
})
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <h2>作业管理</h2>
      <div style="display: flex; gap: 12px;">
        <el-select v-model="statusFilter" placeholder="筛选状态" clearable style="width: 140px;">
          <el-option label="全部" value="" />
          <el-option label="等待中" value="pending" />
          <el-option label="运行中" value="running" />
          <el-option label="已完成" value="completed" />
          <el-option label="失败" value="failed" />
          <el-option label="已取消" value="cancelled" />
        </el-select>
        <el-button type="primary" @click="router.push('/jobs/submit')">提交作业</el-button>
      </div>
    </div>

    <el-card>
      <el-table :data="filteredJobs" stripe v-loading="loading" empty-text="暂无作业" @row-click="(row: Job) => router.push('/jobs/' + row.id)">
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column prop="type" label="类型" width="100" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="owner_id" label="所有者" width="140" show-overflow-tooltip />
        <el-table-column prop="image" label="镜像" width="160" show-overflow-tooltip />
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="更新时间" width="180">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click.stop="router.push('/jobs/' + row.id)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>