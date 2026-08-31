<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getLocalStatus, getSettings, updateResourceSettings, type LocalStatus, type SettingsConfig } from '../api/client'
import { ElMessageBox, ElMessage } from 'element-plus'

const router = useRouter()
const localStatus = ref<LocalStatus | null>(null)
const settings = ref<SettingsConfig | null>(null)
const loading = ref(true)
const saving = ref(false)

// 资源表单
const maxCpu = ref(0)
const maxMem = ref(0)
const reportGpu = ref(true)

onMounted(async () => {
  try {
    localStatus.value = await getLocalStatus()
    settings.value = await getSettings()
    maxCpu.value = settings.value.max_cpu_cores
    maxMem.value = settings.value.max_memory_mb
    reportGpu.value = settings.value.report_gpu
  } catch {}
  loading.value = false
})

async function handleSaveResources() {
  saving.value = true
  try {
    await updateResourceSettings({
      max_cpu_cores: maxCpu.value,
      max_memory_mb: maxMem.value,
      report_gpu: reportGpu.value,
    })
    ElMessage.success('资源配置已更新，Agent 已重启')
    // 刷新状态
    localStatus.value = await getLocalStatus()
    settings.value = await getSettings()
  } catch (e: any) {
    ElMessage.error('保存失败: ' + (e.message || '未知错误'))
  } finally {
    saving.value = false
  }
}

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

    <el-card style="margin-bottom: 16px;">
      <template #header>
        <span>资源限制</span>
      </template>
      <el-form label-width="140px" style="max-width: 520px;">
        <el-form-item label="最大 CPU 核数">
          <el-slider v-model="maxCpu" :min="0" :max="64" :step="1" style="width: 300px;">
            <template #append>
              <span style="margin-left: 12px; min-width: 60px; font-size: 13px; color: #606266;">
                {{ maxCpu ? maxCpu + ' 核' : '全部' }}
              </span>
            </template>
          </el-slider>
        </el-form-item>
        <el-form-item label="最大内存">
          <el-slider v-model="maxMem" :min="0" :max="131072" :step="1024" style="width: 300px;">
            <template #append>
              <span style="margin-left: 12px; min-width: 80px; font-size: 13px; color: #606266;">
                {{ maxMem ? (maxMem / 1024).toFixed(0) + ' GB' : '全部' }}
              </span>
            </template>
          </el-slider>
        </el-form-item>
        <el-form-item label="共享 GPU">
          <el-switch v-model="reportGpu" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="handleSaveResources">
            {{ saving ? '保存中...' : '保存资源配置' }}
          </el-button>
        </el-form-item>
      </el-form>
      <p style="color: #909399; font-size: 13px; margin: 0;">
        设置为 0 表示共享全部可用资源。修改后 Agent 将自动重启使配置生效。
      </p>
    </el-card>

    <el-row :gutter="16">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>向导配置</span>
          </template>
          <p style="color: #606266; margin-bottom: 16px; line-height: 1.6;">
            修改节点名称、调度器地址、邀请码等配置。
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
            配置文件和 Agent 数据存储在本地。<br>
            <code style="font-size: 12px;">{{ settings?.data_dir || './data/cpstart/' }}</code>
          </p>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>