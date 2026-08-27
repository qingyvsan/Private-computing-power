<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getNode, type Node } from '../api/client'
import { statusType, formatBytes } from '../utils/format'

const route = useRoute()
const router = useRouter()
const node = ref<Node | null>(null)
const loading = ref(true)

onMounted(async () => {
  try {
    node.value = await getNode(route.params.id as string)
  } catch {}
  loading.value = false
})

function resourcePercent(used: number, total: number): number {
  if (!total) return 0
  return Math.round((used / total) * 100)
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <el-button text @click="router.push('/nodes')">← 返回节点列表</el-button>
      <h2 v-if="node">节点详情: {{ node.name }}</h2>
    </div>

    <el-card v-loading="loading" v-if="node">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="节点 ID">{{ node.id }}</el-descriptions-item>
        <el-descriptions-item label="名称">{{ node.name }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="statusType(node.status)" size="small">{{ node.status }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="版本">{{ node.version || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Overlay IP">{{ node.overlay_ip || '-' }}</el-descriptions-item>
        <el-descriptions-item label="信誉值">{{ (node.reputation * 100).toFixed(0) }}</el-descriptions-item>
        <el-descriptions-item label="当前任务">{{ node.current_tasks }} / {{ node.max_tasks }}</el-descriptions-item>
        <el-descriptions-item label="Phi 值">{{ node.phi_value?.toFixed(2) || '-' }}</el-descriptions-item>
      </el-descriptions>

      <h3 style="margin: 24px 0 16px;">资源使用</h3>
      <el-row :gutter="16" v-if="node.resources">
        <el-col :span="8">
          <el-card shadow="hover" class="resource-card">
            <div class="resource-label">CPU</div>
            <div class="resource-value">{{ node.resources.cpu_usage?.toFixed(1) }} / {{ node.resources.cpu_cores }} 核</div>
            <el-progress :percentage="resourcePercent(node.resources.cpu_usage || 0, node.resources.cpu_cores)" :stroke-width="6" />
          </el-card>
        </el-col>
        <el-col :span="8">
          <el-card shadow="hover" class="resource-card">
            <div class="resource-label">内存</div>
            <div class="resource-value">{{ formatBytes(node.resources.memory_used || 0) }} / {{ formatBytes(node.resources.memory_bytes || 0) }}</div>
            <el-progress :percentage="resourcePercent(node.resources.memory_used || 0, node.resources.memory_bytes)" :stroke-width="6" />
          </el-card>
        </el-col>
        <el-col :span="8">
          <el-card shadow="hover" class="resource-card">
            <div class="resource-label">磁盘</div>
            <div class="resource-value">{{ formatBytes(node.resources.disk_used || 0) }} / {{ formatBytes(node.resources.disk_bytes || 0) }}</div>
            <el-progress :percentage="resourcePercent(node.resources.disk_used || 0, node.resources.disk_bytes)" :stroke-width="6" />
          </el-card>
        </el-col>
      </el-row>
      <el-empty v-else description="暂无资源数据" />

      <h3 v-if="node.resources?.gpus?.length" style="margin: 24px 0 16px;">GPU</h3>
      <el-table :data="node.resources?.gpus || []" stripe v-if="node.resources?.gpus?.length">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="cores" label="核心数" width="100" />
        <el-table-column label="显存" width="200">
          <template #default="{ row }">
            {{ formatBytes(row.memory_used_mb * 1024 * 1024) }} / {{ formatBytes(row.memory_mb * 1024 * 1024) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>