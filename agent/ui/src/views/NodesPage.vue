<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessageBox, ElMessage } from 'element-plus'
import { listNodes, unregisterNode, type Node } from '../api/client'
import { statusType } from '../utils/format'
import { usePolling } from '../utils/usePolling'

const router = useRouter()
const nodes = ref<Node[]>([])
const loading = ref(true)
const searchQuery = ref('')

async function fetchNodes() {
  try {
    nodes.value = await listNodes()
  } catch {}
}

usePolling(fetchNodes, 10000)

onMounted(async () => {
  await fetchNodes()
  loading.value = false
})

const filteredNodes = computed(() => {
  if (!searchQuery.value) return nodes.value
  const q = searchQuery.value.toLowerCase()
  return nodes.value.filter(n => n.name.toLowerCase().includes(q) || n.id.toLowerCase().includes(q))
})

async function handleUnregister(node: Node) {
  try {
    await ElMessageBox.confirm(
      `确定要注销节点 "${node.name}" (${node.id}) 吗？\n此操作将从集群中移除该节点，其上的所有任务将被回收。`,
      '注销节点',
      { confirmButtonText: '确认注销', cancelButtonText: '取消', type: 'warning' }
    )
    await unregisterNode(node.id, 'user requested')
    ElMessage.success(`节点 ${node.name} 已注销`)
    await fetchNodes()
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error(err?.message || '注销失败')
    }
  }
}

</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <h2>节点管理</h2>
      <el-input
        v-model="searchQuery"
        placeholder="搜索节点名称或 ID..."
        clearable
        style="width: 280px;"
        prefix-icon="Search"
      />
    </div>

    <el-card>
      <el-table :data="filteredNodes" stripe v-loading="loading" empty-text="暂无节点" @row-click="(row: Node) => router.push('/nodes/' + row.id)">
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="id" label="ID" min-width="200" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="overlay_ip" label="Overlay IP" width="140" />
        <el-table-column prop="current_tasks" label="任务数" width="80" />
        <el-table-column prop="max_tasks" label="最大任务" width="90" />
        <el-table-column prop="reputation" label="信誉" width="80">
          <template #default="{ row }">
            <span>{{ (row.reputation * 100).toFixed(0) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="100" />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click.stop="router.push('/nodes/' + row.id)">详情</el-button>
            <el-button type="danger" link size="small" @click.stop="handleUnregister(row)">注销</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>