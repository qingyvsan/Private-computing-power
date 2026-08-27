<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { submitJob } from '../api/client'
import { ElMessage } from 'element-plus'

const router = useRouter()
const name = ref('')
const type = ref('container')
const image = ref('')
const command = ref('')
const envVars = ref('')
const cpuCores = ref(1)
const memoryMB = ref(1024)
const submitting = ref(false)

async function handleSubmit() {
  if (!name.value.trim()) {
    ElMessage.warning('请输入作业名称')
    return
  }
  submitting.value = true
  try {
    const jobPayload: any = {
      name: name.value.trim(),
      type: type.value,
    }
    if (image.value) jobPayload.image = image.value.trim()
    if (command.value.trim()) jobPayload.command = command.value.trim()
    if (envVars.value.trim()) {
      jobPayload.env = envVars.value.split('\n').filter(Boolean).map((line: string) => {
        const idx = line.indexOf('=')
        if (idx > 0) return { key: line.slice(0, idx).trim(), value: line.slice(idx + 1).trim() }
        return { key: line.trim(), value: '' }
      })
    }
    jobPayload.resources = {
      cpu_cores: cpuCores.value,
      memory_mb: memoryMB.value,
    }
    await submitJob(jobPayload)
    ElMessage.success('作业已提交')
    router.push('/jobs')
  } catch (e: any) {
    ElMessage.error(e.message || '提交失败')
  }
  submitting.value = false
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <el-button text @click="router.push('/jobs')">← 返回作业列表</el-button>
      <h2>提交作业</h2>
    </div>

    <el-card style="max-width: 720px;">
      <el-form label-width="120px" @submit.prevent="handleSubmit">
        <el-form-item label="作业名称" required>
          <el-input v-model="name" placeholder="my-job" />
        </el-form-item>
        <el-form-item label="作业类型">
          <el-select v-model="type" style="width: 100%;">
            <el-option label="容器作业" value="container" />
            <el-option label="自定义作业" value="custom" />
          </el-select>
        </el-form-item>
        <el-form-item label="镜像">
          <el-input v-model="image" placeholder="docker.io/library/ubuntu:latest" />
        </el-form-item>
        <el-form-item label="启动命令">
          <el-input v-model="command" placeholder="echo hello" />
        </el-form-item>
        <el-form-item label="环境变量">
          <el-input v-model="envVars" type="textarea" :rows="4" placeholder="KEY=VALUE&#10;FOO=bar" />
          <p style="color: #909399; font-size: 12px; margin-top: 4px;">每行一个 KEY=VALUE</p>
        </el-form-item>
        <el-form-item label="CPU 核数">
          <el-input-number v-model="cpuCores" :min="0.1" :max="64" :step="0.5" />
        </el-form-item>
        <el-form-item label="内存 (MB)">
          <el-input-number v-model="memoryMB" :min="1" :max="65536" :step="256" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" native-type="submit">提交</el-button>
          <el-button @click="router.push('/jobs')">取消</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>