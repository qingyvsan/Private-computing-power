<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getTrustGraph, declareTrust, revokeTrust, type TrustEdge } from '../api/client'
import { formatTime } from '../utils/format'
import { ElMessage, ElMessageBox } from 'element-plus'

const edges = ref<TrustEdge[]>([])
const loading = ref(true)
const targetNodeID = ref('')
const declaring = ref(false)

onMounted(async () => {
  try {
    edges.value = await getTrustGraph()
  } catch {}
  loading.value = false
})

async function handleDeclare() {
  if (!targetNodeID.value.trim()) return
  declaring.value = true
  try {
    await declareTrust(targetNodeID.value.trim())
    ElMessage.success('信任声明已发送')
    targetNodeID.value = ''
    edges.value = await getTrustGraph()
  } catch (e: any) {
    ElMessage.error(e.message || '声明失败')
  }
  declaring.value = false
}

async function handleRevoke(edge: TrustEdge) {
  try {
    await ElMessageBox.confirm(`确定要撤销对节点 ${edge.to_node} 的信任吗？`, '确认撤销', { type: 'warning' })
    await revokeTrust(edge.to_node)
    ElMessage.success('信任已撤销')
    edges.value = await getTrustGraph()
  } catch {}
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <h2>信任管理</h2>
    </div>

    <el-row :gutter="16">
      <el-col :span="16">
        <el-card>
          <template #header>
            <span>信任关系</span>
          </template>
          <el-table :data="edges" stripe v-loading="loading" empty-text="暂无信任关系">
            <el-table-column prop="from_node" label="发起节点" min-width="180" show-overflow-tooltip />
            <el-table-column prop="to_node" label="目标节点" min-width="180" show-overflow-tooltip />
            <el-table-column label="过期时间" width="180">
              <template #default="{ row }">{{ formatTime(row.expires_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="80" fixed="right">
              <template #default="{ row }">
                <el-button type="danger" link size="small" @click="handleRevoke(row)">撤销</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card>
          <template #header>
            <span>声明信任</span>
          </template>
          <el-form @submit.prevent="handleDeclare">
            <el-form-item label="目标节点 ID">
              <el-input v-model="targetNodeID" placeholder="输入节点 ID" />
            </el-form-item>
            <el-button type="primary" :loading="declaring" :disabled="!targetNodeID.trim()" @click="handleDeclare" style="width: 100%;">声明信任</el-button>
          </el-form>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>