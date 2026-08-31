<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getJob, cancelJob, type Job, type Stage, type Unit } from '../api/client'
import { statusType, formatTime, formatDuration } from '../utils/format'
import { usePolling } from '../utils/usePolling'
import { ElMessageBox, ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const job = ref<Job | null>(null)
const loading = ref(true)
const cancelling = ref(false)

async function fetchJob() {
  try {
    job.value = await getJob(route.params.id as string)
  } catch {}
}

usePolling(fetchJob, 5000, false)

onMounted(async () => {
  await fetchJob()
  loading.value = false
})

async function handleCancel() {
  if (!job.value) return
  try {
    await ElMessageBox.confirm('确定要取消此作业吗？', '确认取消', { type: 'warning' })
    cancelling.value = true
    await cancelJob(job.value.id)
    ElMessage.success('作业已取消')
    job.value = await getJob(job.value.id)
  } catch {}
  cancelling.value = false
}

function stageStatus(stage: Stage): string {
  if (stage.status === 'completed') return 'success'
  if (stage.status === 'failed') return 'danger'
  if (stage.status === 'running') return 'warning'
  return 'info'
}

function getUnits(stage: Stage): Unit[] {
  return stage.units || []
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <el-button text @click="router.push('/jobs')">← 返回作业列表</el-button>
      <h2 v-if="job">作业详情: {{ job.name }}</h2>
    </div>

    <el-card v-loading="loading" v-if="job" style="margin-bottom: 16px;">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="作业 ID">{{ job.id }}</el-descriptions-item>
        <el-descriptions-item label="名称">{{ job.name }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ job.type }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="statusType(job.status)" size="small">{{ job.status }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="镜像">{{ job.image || '-' }}</el-descriptions-item>
        <el-descriptions-item label="所有者">{{ job.owner_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(job.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{ formatTime(job.updated_at) }}</el-descriptions-item>
      </el-descriptions>

      <div style="margin-top: 16px; text-align: right;">
        <el-button @click="router.push('/jobs')">返回</el-button>
        <el-button v-if="job.status === 'running' || job.status === 'pending'" type="danger" :loading="cancelling" @click="handleCancel">取消作业</el-button>
      </div>
    </el-card>

    <el-card v-if="job?.stages?.length">
      <template #header>
        <span>执行阶段</span>
      </template>
      <div v-for="stage in job.stages" :key="stage.id" style="margin-bottom: 16px;">
        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
          <el-tag :type="stageStatus(stage)" size="small">{{ stage.status }}</el-tag>
          <strong>{{ stage.name }}</strong>
          <span style="color: #909399; font-size: 13px;">{{ stage.id }}</span>
        </div>
        <el-table :data="getUnits(stage)" stripe size="small" empty-text="无单元">
          <el-table-column prop="id" label="单元 ID" min-width="200" show-overflow-tooltip />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="assigned_node" label="分配节点" width="160" show-overflow-tooltip />
          <el-table-column prop="retry_count" label="重试" width="60" />
          <el-table-column label="耗时" width="100">
            <template #default="{ row }">
              {{ row.started_at ? formatDuration((row.completed_at || Date.now()/1000) - row.started_at) : '-' }}
            </template>
          </el-table-column>
          <el-table-column prop="exit_code" label="退出码" width="80" />
          <el-table-column prop="error_message" label="错误信息" min-width="200" show-overflow-tooltip />
        </el-table>
      </div>
    </el-card>
  </div>
</template>