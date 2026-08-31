<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { submitJob, uploadProject, type ProjectMeta } from '../api/client'
import { ElMessage } from 'element-plus'

const router = useRouter()

// 提交模式
const submitMode = ref<'normal' | 'project'>('normal')

// 普通模式字段
const name = ref('')
const type = ref('container')
const image = ref('')
const command = ref('')
const envVars = ref('')
const cpuCores = ref(1)
const memoryMB = ref(1024)
const allowSelfAssignment = ref(false)

// 项目模式字段
const projectFile = ref<File | null>(null)
const projectStartupCommand = ref('')
const projectBaseImage = ref('python:3.11-slim')
const projectUploading = ref(false)
const projectMeta = ref<ProjectMeta | null>(null)

const submitting = ref(false)

async function handleSubmit() {
  if (!name.value.trim()) {
    ElMessage.warning('请输入作业名称')
    return
  }

  if (submitMode.value === 'project') {
    // 项目模式：先上传项目，再提交作业
    if (!projectFile.value) {
      ElMessage.warning('请选择项目文件')
      return
    }
    if (!projectStartupCommand.value.trim()) {
      ElMessage.warning('请输入启动命令')
      return
    }

    projectUploading.value = true
    try {
      const meta = await uploadProject(projectFile.value, projectStartupCommand.value.trim(), projectBaseImage.value)
      projectMeta.value = meta
      ElMessage.success('项目上传成功')

      // 提交作业
      submitting.value = true
      const jobPayload: any = {
        name: name.value.trim(),
        type: 'container',
        allow_self_assignment: allowSelfAssignment.value,
        project_id: meta.project_id,
        startup_command: projectStartupCommand.value.trim(),
        base_image: projectBaseImage.value,
        resources: {
          cpu_cores: cpuCores.value,
          memory_mb: memoryMB.value,
        },
      }
      await submitJob(jobPayload)
      ElMessage.success('作业已提交')
      router.push('/jobs')
    } catch (e: any) {
      ElMessage.error(e.message || '提交失败')
    }
    projectUploading.value = false
    submitting.value = false
    return
  }

  // 普通模式
  submitting.value = true
  try {
    const jobPayload: any = {
      name: name.value.trim(),
      type: type.value,
      allow_self_assignment: allowSelfAssignment.value,
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

function onFileSelected(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    projectFile.value = target.files[0]
  }
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

        <!-- 提交模式切换 -->
        <el-form-item label="提交方式">
          <el-radio-group v-model="submitMode">
            <el-radio value="normal">普通提交</el-radio>
            <el-radio value="project">项目提交</el-radio>
          </el-radio-group>
        </el-form-item>

        <!-- 普通模式 -->
        <template v-if="submitMode === 'normal'">
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
        </template>

        <!-- 项目模式 -->
        <template v-if="submitMode === 'project'">
          <el-form-item label="项目文件" required>
            <input type="file" accept=".zip" @change="onFileSelected" style="width: 100%;" />
            <p style="color: #909399; font-size: 12px; margin-top: 4px;">上传 .zip 格式的项目文件夹</p>
          </el-form-item>
          <el-form-item label="启动命令" required>
            <el-input v-model="projectStartupCommand" placeholder="例如: python train.py --epochs 10" />
            <p style="color: #909399; font-size: 12px; margin-top: 4px;">项目挂载到 /workspace，在此目录下执行该命令</p>
          </el-form-item>
          <el-form-item label="基础镜像">
            <el-select v-model="projectBaseImage" style="width: 100%;" allow-create filterable>
              <el-option label="Python 3.11" value="python:3.11-slim" />
              <el-option label="Python 3.10" value="python:3.10-slim" />
              <el-option label="Node.js 20" value="node:20-slim" />
              <el-option label="Node.js 18" value="node:18-slim" />
              <el-option label="Ubuntu 22.04" value="ubuntu:22.04" />
              <el-option label="Alpine" value="alpine:latest" />
              <el-option label="Go 1.22" value="golang:1.22" />
            </el-select>
            <p style="color: #909399; font-size: 12px; margin-top: 4px;">项目将挂载到 /workspace，使用此镜像运行</p>
          </el-form-item>
        </template>

        <!-- 通用字段 -->
        <el-form-item label="CPU 核数">
          <el-input-number v-model="cpuCores" :min="0.1" :max="64" :step="0.5" />
        </el-form-item>
        <el-form-item label="内存 (MB)">
          <el-input-number v-model="memoryMB" :min="1" :max="65536" :step="256" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="allowSelfAssignment">允许分配到自己的节点</el-checkbox>
          <p style="color: #909399; font-size: 12px; margin-left: 8px;">默认不勾选时，调度器会优先分配到其他节点</p>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting || projectUploading" native-type="submit">
            {{ projectUploading ? '上传项目中...' : submitting ? '提交中...' : '提交' }}
          </el-button>
          <el-button @click="router.push('/jobs')">取消</el-button>
        </el-form-item>
      </el-form>

      <!-- 上传成功提示 -->
      <el-alert v-if="projectMeta" type="success" show-icon style="margin-top: 16px;">
        <template #title>
          项目已上传: {{ projectMeta.file_name }} ({{ (projectMeta.size / 1024 / 1024).toFixed(1) }} MB)
        </template>
      </el-alert>
    </el-card>
  </div>
</template>