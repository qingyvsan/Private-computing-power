<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getLocalStatus, getSetupStatus, type LocalStatus } from '../api/client'
import { ElMessageBox, ElMessage } from 'element-plus'

const router = useRouter()
const localStatus = ref<LocalStatus | null>(null)
const setupStatus = ref<any>(null)
const loading = ref(true)

onMounted(async () => {
  try {
    localStatus.value = await getLocalStatus()
    setupStatus.value = await getSetupStatus()
  } catch {}
  loading.value = false
})

async function handleRerunSetup() {
  try {
    await ElMessageBox.confirm(
      '重新运行安装向导将保留现有配置，但允许您修改设置。要继续吗？',
      '重新运行向导',
      { type: 'info', confirmButtonText: '继续', cancelButtonText: '取消' }
    )
    router.push('/setup')
  } catch {}
}

function handleCopy(text: string) {
  navigator.clipboard?.writeText(text)
  ElMessage.success('已复制')
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <h2>设置</h2>
    </div>

    <el-card v-loading="loading" style="margin-bottom: 16px;">
      <template #header>
        <span>Agent 信息</span>
      </template>
      <el-descriptions :column="1" border v-if="localStatus">
        <el-descriptions-item label="节点 ID">
          <div style="display: flex; align-items: center; gap: 8px;">
            <code style="font-size: 12px;">{{ localStatus.node_id }}</code>
            <el-button link size="small" @click="handleCopy(localStatus.node_id)">复制</el-button>
          </div>
        </el-descriptions-item>
        <el-descriptions-item label="节点名称">{{ localStatus.agent_name }}</el-descriptions-item>
        <el-descriptions-item label="Agent 状态">{{ localStatus.agent_status }}</el-descriptions-item>
        <el-descriptions-item label="调度器地址">{{ localStatus.scheduler }}</el-descriptions-item>
      </el-descriptions>
      <el-empty v-else description="未获取到 Agent 信息" />
    </el-card>

    <el-row :gutter="16">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>配置</span>
          </template>
          <p style="color: #606266; margin-bottom: 16px; line-height: 1.6;">
            修改节点名称、资源限制、调度器地址等配置。
          </p>
          <el-button @click="handleRerunSetup">重新运行安装向导</el-button>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>数据目录</span>
          </template>
          <p style="color: #606266; margin-bottom: 16px; line-height: 1.6;">
            配置文件和 Agent 数据存储在本地数据目录中。
          </p>
          <el-button @click="handleCopy(setupStatus?.configured ? '已配置' : '未配置')">复制配置信息</el-button>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>