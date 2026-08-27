<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getSetupStatus } from './api/client'

const router = useRouter()
const loading = ref(true)

onMounted(async () => {
  try {
    const status = await getSetupStatus()
    if (!status.configured) {
      router.replace('/setup')
    }
  } catch {
    // 如果 API 不可用，假设已配置并显示主界面
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div id="cp-app">
    <div v-if="loading" class="loading-screen">
      <el-icon class="is-loading" :size="32">
        <Loading />
      </el-icon>
      <p>Loading...</p>
    </div>
    <router-view v-else />
  </div>
</template>

<style scoped>
.loading-screen {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100vh;
  color: #909399;
}
</style>