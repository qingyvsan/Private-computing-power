<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { postSetupConfig, getSetupCheck } from '../../api/client'
import StepWelcome from './StepWelcome.vue'
import StepScheduler from './StepScheduler.vue'
import StepIdentity from './StepIdentity.vue'
import StepResources from './StepResources.vue'
import StepCheck from './StepCheck.vue'
import StepWindows from './StepWindows.vue'
import StepMacOS from './StepMacOS.vue'
import StepComplete from './StepComplete.vue'

const router = useRouter()

const currentStep = ref(0)
const loading = ref(false)
const config = ref({
  scheduler: { address: '8.138.108.183:9090' },
  agent: { name: '', data_dir: './data/cpstart' },
  resources: { max_cpu_cores: 0, max_memory_mb: 0, report_gpu: true },
  invite_code: '',
})
const checkResults = ref<Record<string, any>>({})
const wslSetupBusy = ref(false)
const macosSetupBusy = ref(false)

// Windows 步骤仅在 Windows + containerd 不可用时显示
const showWindowsStep = computed(() => {
  return checkResults.value.os === 'windows' && !checkResults.value.containerd
})

// macOS 步骤仅在 macOS + containerd 不可用时显示
const showMacOSStep = computed(() => {
  return checkResults.value.os === 'darwin' && !checkResults.value.containerd
})

// 步骤列表动态包含平台相关步骤
const steps = computed(() => {
  const base = ['欢迎', '调度器', '身份', '资源', '检查']
  if (showWindowsStep.value) {
    base.push('Windows')
  }
  if (showMacOSStep.value) {
    base.push('macOS')
  }
  base.push('完成')
  return base
})

const isLast = computed(() => currentStep.value === steps.value.length - 1)

async function next() {
  if (currentStep.value === 3) {
    // 运行环境检查
    loading.value = true
    try {
      checkResults.value = await getSetupCheck()
    } catch {}
    loading.value = false
  }

  if (currentStep.value >= steps.value.length - 1) {
    return
  }
  currentStep.value++
}

function prev() {
  if (currentStep.value <= 0) return
  currentStep.value--
}

async function finish() {
  loading.value = true
  try {
    await postSetupConfig(config.value)
    router.push('/dashboard')
  } catch (e: any) {
    console.error('Setup failed:', e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div style="min-height: 100vh; display: flex; flex-direction: column; align-items: center; justify-content: center; background: #f5f7fa; padding: 40px;">
    <el-card style="width: 600px; max-width: 90vw;">
      <template #header>
        <div style="text-align: center;">
          <h2 style="margin-bottom: 8px;">Computing Power 首次配置</h2>
          <el-steps :active="currentStep" align-center finish-status="success" style="margin-top: 16px;">
            <el-step v-for="(label, i) in steps" :key="i" :title="label" />
          </el-steps>
        </div>
      </template>

      <div style="min-height: 250px; padding: 16px 0;">
        <StepWelcome v-if="currentStep === 0" />
        <StepScheduler v-else-if="currentStep === 1" v-model="config.scheduler.address" />
        <StepIdentity v-else-if="currentStep === 2" v-model:name="config.agent.name" v-model:code="config.invite_code" v-model:data-dir="config.agent.data_dir" />
        <StepResources v-else-if="currentStep === 3" v-model:maxCpu="config.resources.max_cpu_cores" v-model:maxMem="config.resources.max_memory_mb" v-model:reportGpu="config.resources.report_gpu" />
        <StepCheck v-else-if="currentStep === 4" :results="checkResults" :loading="loading" />
        <StepWindows v-else-if="currentStep === 5 && showWindowsStep" :results="checkResults" v-model:busy="wslSetupBusy" @done="next" />
        <StepMacOS v-else-if="currentStep === 5 && showMacOSStep" :results="checkResults" v-model:busy="macosSetupBusy" @done="next" />
        <StepComplete v-else-if="currentStep === steps.length - 1" />
      </div>

      <div style="display: flex; justify-content: space-between; margin-top: 16px;">
        <el-button v-if="currentStep > 0 && currentStep < steps.length - 1" @click="prev">上一步</el-button>
        <span v-else />
        <el-button v-if="!isLast" type="primary" @click="next" :loading="loading || wslSetupBusy || macosSetupBusy">下一步</el-button>
        <el-button v-else type="primary" @click="finish" :loading="loading">进入控制台</el-button>
      </div>
    </el-card>
  </div>
</template>