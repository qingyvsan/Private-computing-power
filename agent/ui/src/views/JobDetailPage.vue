<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getJob, cancelJob, submitJob, type Job, type Stage, type Unit } from '../api/client'
import { statusType, formatTime, formatDuration } from '../utils/format'
import { usePolling } from '../utils/usePolling'
import { ElMessageBox, ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()
const job = ref<Job | null>(null)
const loading = ref(true)
const cancelling = ref(false)

// 输出查看
const outputDialogVisible = ref(false)
const outputUnit = ref<Unit | null>(null)

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

async function handleRetry() {
  if (!job.value) return
  try {
    await ElMessageBox.confirm('确定要重试此作业吗？\n将使用相同的参数创建一个新作业。', '重试作业', { type: 'info' })
    const newJob: any = {
      name: job.value.name + ' (retry)',
      type: job.value.type,
      image: job.value.image,
      allow_self_assignment: false,
    }
    await submitJob(newJob)
    ElMessage.success('新作业已创建')
    await fetchJob()
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error(err?.message || '重试失败')
    }
  }
}

function handleClone() {
  if (!job.value) return
  router.push(`/jobs/submit?clone=${job.value.id}`)
}

function showOutput(unit: Unit) {
  outputUnit.value = unit
  outputDialogVisible.value = true
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

function isTerminalStatus(status: string): boolean {
  return ['completed', 'failed', 'cancelled'].includes(status)
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
        <el-button v-if="isTerminalStatus(job.status)" @click="handleClone">克隆并重新提交</el-button>
        <el-button v-if="job.status === 'failed' || job.status === 'cancelled'" type="primary" @click="handleRetry">重试</el-button>
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
          <el-table-column prop="id" label="单元 ID" min-width="180" show-overflow-tooltip />
          <el-table-column prop="status" label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="assigned_node" label="分配节点" width="150" show-overflow-tooltip />
          <el-table-column prop="retry_count" label="重试" width="60" />
          <el-table-column label="耗时" width="100">
            <template #default="{ row }">
              {{ row.started_at ? formatDuration((row.completed_at || Date.now()/1000) - row.started_at) : '-' }}
            </template>
          </el-table-column>
          <el-table-column prop="exit_code" label="退出码" width="70" />
          <el-table-column prop="error_message" label="错误信息" min-width="160" show-overflow-tooltip />
          <el-table-column label="输出" width="60" fixed="right">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click="showOutput(row)" :disabled="!row.output">查看</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <!-- 单元输出弹窗 -->
    <el-dialog v-model="outputDialogVisible" title="单元输出" width="70%" top="5vh">
      <template v-if="outputUnit">
        <p style="margin: 0 0 12px; color: #909399; font-size: 13px;">
          单元 <code>{{ outputUnit.id }}</code> · 状态 <el-tag :type="statusType(outputUnit.status)" size="small">{{ outputUnit.status }}</el-tag>
        </p>
        <pre style="background: #1e1e1e; color: #d4d4d4; padding: 16px; border-radius: 6px; max-height: 60vh; overflow: auto; font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-all;">{{ outputUnit.output || '(无输出)' }}</pre>
      </template>
    </el-dialog>
  </div>
</template>