<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { postSetupConfig, getSetupCheck } from '../../api/client'
import StepWelcome from './StepWelcome.vue'
import StepScheduler from './StepScheduler.vue'
import StepIdentity from './StepIdentity.vue'
import StepResources from './StepResources.vue'
import StepCheck from './StepCheck.vue'
import StepComplete from './StepComplete.vue'

const router = useRouter()

const currentStep = ref(0)
const loading = ref(false)
const config = ref({
  scheduler: { address: 'localhost:9090' },
  agent: { name: '' },
  resources: { max_cpu_cores: 0, max_memory_mb: 0, report_gpu: true },
  invite_code: '',
})
const checkResults = ref<Record<string, boolean>>({})

const steps = ['欢迎', '调度器', '身份', '资源', '检查', '完成']
const isLast = computed(() => currentStep.value === steps.length - 1)

async function next() {
  if (currentStep.value === 3) {
    // 运行环境检查
    loading.value = true
    try {
      checkResults.value = await getSetupCheck()
    } catch {}
    loading.value = false
  }

  if (currentStep.value >= steps.length - 1) {
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
        <StepIdentity v-else-if="currentStep === 2" v-model:name="config.agent.name" v-model:code="config.invite_code" />
        <StepResources v-else-if="currentStep === 3" v-model:maxCpu="config.resources.max_cpu_cores" v-model:maxMem="config.resources.max_memory_mb" v-model:reportGpu="config.resources.report_gpu" />
        <StepCheck v-else-if="currentStep === 4" :results="checkResults" :loading="loading" />
        <StepComplete v-else-if="currentStep === 5" />
      </div>

      <div style="display: flex; justify-content: space-between; margin-top: 16px;">
        <el-button v-if="currentStep > 0 && currentStep < 5" @click="prev">上一步</el-button>
        <span v-else />
        <el-button v-if="!isLast" type="primary" @click="next" :loading="loading">
          {{ currentStep === 4 ? '完成' : '下一步' }}
        </el-button>
        <el-button v-else type="primary" @click="finish" :loading="loading">
          进入控制台
        </el-button>
      </div>
    </el-card>
  </div>
</template>