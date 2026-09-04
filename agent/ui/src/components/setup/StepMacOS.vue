<script setup lang="ts">
import { ref, computed } from 'vue'
import { startMacOSSetup, getMacOSStatus } from '../../api/client'
import type { MacOSStatus } from '../../api/client'

const props = defineProps<{
  results: Record<string, any>
  busy?: boolean
}>()

const emit = defineEmits<{
  'update:busy': [value: boolean]
  done: []
}>()

const isMacOS = computed(() => props.results.os === 'darwin')
const containerBackend = computed(() => props.results.container_backend || 'none')
const containerdAvailable = computed(() => props.results.containerd === true)
const brewAvailable = computed(() => props.results.brew_available === true)

// macOS 配置状态
const setupStatus = ref<MacOSStatus | null>(null)
const setupRunning = ref(false)
const setupDone = ref(false)
const setupError = ref('')
const pollTimer = ref<ReturnType<typeof setInterval> | null>(null)

// 整体进度
const progressPercent = computed(() => {
  if (!setupStatus.value) return 0
  const steps = setupStatus.value.steps
  if (steps.length === 0) return 0
  const done = steps.filter(s => s.status === 'done' || s.status === 'skipped').length
  return Math.round((done / steps.length) * 100)
})

// 后端类型标签
const backendLabel = computed(() => {
  const map: Record<string, string> = {
    'colima': 'Colima',
    'docker-desktop': 'Docker Desktop',
    'orbstack': 'OrbStack',
    'containerd': 'containerd',
    'none': '未检测到',
  }
  return map[containerBackend.value] || containerBackend.value
})

// 开始一键配置
async function startSetup() {
  setupRunning.value = true
  setupDone.value = false
  setupError.value = ''
  emit('update:busy', true)

  try {
    await startMacOSSetup()
    // 开始轮询状态
    pollTimer.value = setInterval(pollStatus, 2000)
    // 立即轮询一次
    await pollStatus()
  } catch (e: any) {
    setupError.value = e.message || '启动配置失败'
    setupRunning.value = false
    emit('update:busy', false)
  }
}

// 轮询配置状态
async function pollStatus() {
  try {
    const status = await getMacOSStatus()
    setupStatus.value = status

    if (!status.running) {
      // 配置完成（成功或失败）
      if (pollTimer.value) {
        clearInterval(pollTimer.value)
        pollTimer.value = null
      }
      setupRunning.value = false
      emit('update:busy', false)

      if (status.error) {
        setupError.value = status.error
      } else {
        setupDone.value = true
        emit('done')
      }
    }
  } catch (e: any) {
    // 轮询失败时忽略，下次继续
  }
}

// 重试
function retry() {
  setupError.value = ''
  startSetup()
}

// 步骤状态对应的颜色
function stepStatusType(status: string): string {
  switch (status) {
    case 'done': return 'success'
    case 'failed': return 'danger'
    case 'skipped': return 'info'
    case 'running': return 'primary'
    default: return 'info'
  }
}
</script>

<template>
  <div style="padding: 40px 20px;">
    <h3 style="margin-bottom: 20px;">macOS 容器运行时环境</h3>

    <!-- 检测状态 -->
    <el-descriptions :column="1" border style="margin-bottom: 24px;">
      <el-descriptions-item label="操作系统">
        <el-tag type="success" size="small">macOS</el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="容器后端">
        <el-tag :type="containerdAvailable ? 'success' : 'info'" size="small">
          {{ containerdAvailable ? backendLabel : '未检测到' }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="Homebrew">
        <el-tag :type="brewAvailable ? 'success' : 'info'" size="small">
          {{ brewAvailable ? '已安装' : '未安装' }}
        </el-tag>
      </el-descriptions-item>
    </el-descriptions>

    <!-- macOS 一键配置 -->
    <div v-if="!containerdAvailable && isMacOS">

      <!-- 未开始配置 -->
      <div v-if="!setupRunning && !setupDone && !setupError" style="text-align: center; padding: 20px;">
        <el-icon :size="48" color="#e6a23c"><WarningFilled /></el-icon>
        <p style="color: #606266; margin: 16px 0; line-height: 1.6;">
          macOS 原生不支持 containerd。点击下方按钮自动安装配置 Colima 容器运行时环境。
        </p>
        <el-button type="primary" size="large" @click="startSetup">
          一键配置 Colima 容器环境
        </el-button>
        <p style="color: #909399; font-size: 12px; margin-top: 12px;">
          将自动安装 Homebrew（如未安装）、Colima 并启动 containerd 容器运行时
        </p>
      </div>

      <!-- 配置中 -->
      <div v-if="setupRunning" style="margin-top: 16px;">
        <el-progress :percentage="progressPercent" :stroke-width="8" :status="'success'" style="margin-bottom: 20px;" />

        <el-steps direction="vertical" :space="60">
          <el-step
            v-for="(step, i) in setupStatus?.steps || []"
            :key="i"
            :title="step.name"
            :status="stepStatusType(step.status) as any"
          >
            <template #icon>
              <el-icon v-if="step.status === 'running'" class="is-loading" color="#409eff"><Loading /></el-icon>
              <el-icon v-else-if="step.status === 'done'" color="#67c23a"><Check /></el-icon>
              <el-icon v-else-if="step.status === 'failed'" color="#f56c6c"><Close /></el-icon>
              <el-icon v-else-if="step.status === 'skipped'" color="#909399"><Minus /></el-icon>
              <el-icon v-else color="#dcdfe6"><CircleCheck /></el-icon>
            </template>
            <template #description>
              <div v-if="step.log" style="margin-top: 4px;">
                <pre style="font-size: 12px; background: #f5f7fa; padding: 8px 12px; border-radius: 4px; max-height: 80px; overflow-y: auto; white-space: pre-wrap; word-break: break-all; color: #606266; margin: 0;">{{ step.log }}</pre>
              </div>
            </template>
          </el-step>
        </el-steps>
      </div>

      <!-- 配置完成 -->
      <div v-if="setupDone" style="text-align: center; padding: 20px;">
        <el-icon :size="48" color="#67c23a"><SuccessFilled /></el-icon>
        <p style="color: #67c23a; margin-top: 12px; font-weight: 600; font-size: 16px;">Colima 容器环境配置完成</p>
        <p style="color: #909399; font-size: 13px; margin: 8px 0 16px 0;">容器运行时就绪，可以继续下一步配置</p>

        <el-button type="primary" @click="$emit('done')">继续配置</el-button>
      </div>

      <!-- 配置失败 -->
      <div v-if="setupError && !setupRunning" style="background: #fef0f0; border: 1px solid #fde2e2; border-radius: 8px; padding: 20px; margin-top: 16px;">
        <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 12px;">
          <el-icon :size="20" color="#f56c6c"><WarningFilled /></el-icon>
          <span style="font-weight: 600; color: #f56c6c;">配置失败</span>
        </div>
        <pre style="font-size: 13px; background: #fff; padding: 12px; border-radius: 4px; white-space: pre-wrap; word-break: break-all; color: #606266; margin: 0 0 16px 0; max-height: 150px; overflow-y: auto;">{{ setupError }}</pre>
        <div style="display: flex; gap: 12px;">
          <el-button type="primary" @click="retry">重试</el-button>
          <el-button @click="setupError = ''; setupDone = false;">取消</el-button>
        </div>
      </div>

    </div>

    <!-- containerd 已可用 -->
    <div v-else-if="containerdAvailable" style="text-align: center; padding: 20px;">
      <el-icon :size="48" color="#67c23a"><SuccessFilled /></el-icon>
      <p style="color: #67c23a; margin-top: 12px; font-weight: 600;">容器运行时就绪</p>
      <p style="color: #909399; font-size: 13px;">{{ backendLabel }} 已可用，可以正常执行容器作业。</p>
    </div>

    <!-- 非 macOS 系统 -->
    <div v-else style="background: #f5f7fa; border-radius: 8px; padding: 20px;">
      <p style="color: #606266; margin: 0; line-height: 1.6;">
        当前操作系统为 <strong>{{ results.os }}</strong>，不需要 macOS 容器配置。
      </p>
    </div>
  </div>
</template>