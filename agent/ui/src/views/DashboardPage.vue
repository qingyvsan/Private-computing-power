<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getLocalStatus, listNodes, listJobs, type LocalStatus, type Node, type Job } from '../api/client'

const router = useRouter()
const localStatus = ref<LocalStatus | null>(null)
const nodes = ref<Node[]>([])
const jobs = ref<Job[]>([])

onMounted(async () => {
  try {
    localStatus.value = await getLocalStatus()
    nodes.value = await listNodes()
    const resp = await listJobs()
    jobs.value = resp.jobs.slice(0, 5)
  } catch {}
})

const onlineCount = ref(0)
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <h2>仪表盘</h2>
    </div>

    <el-row :gutter="16" style="margin-bottom: 24px;">
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <div class="stat-value">{{ nodes.length }}</div>
            <div class="stat-label">集群节点</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <div class="stat-value">{{ jobs.length }}</div>
            <div class="stat-label">作业总数</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <div class="stat-value">{{ localStatus?.agent_status || '-' }}</div>
            <div class="stat-label">Agent 状态</div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <div class="stat-card">
            <div class="stat-value" style="color: #67c23a;">{{ onlineCount }}</div>
            <div class="stat-label">在线节点</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>最近作业</span>
          </template>
          <el-table :data="jobs" stripe size="small" empty-text="暂无作业" @row-click="(row: Job) => router.push('/jobs/' + row.id)">
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 'completed' ? 'success' : row.status === 'failed' ? 'danger' : 'warning'" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="type" label="类型" width="80" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>节点概览</span>
          </template>
          <el-table :data="nodes.slice(0, 5)" stripe size="small" empty-text="暂无节点" @row-click="(row: Node) => router.push('/nodes/' + row.id)">
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="status" label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.status === 'online' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="current_tasks" label="任务" width="60" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>