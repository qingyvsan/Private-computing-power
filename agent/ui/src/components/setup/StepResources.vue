<script setup lang="ts">
const maxCpu = defineModel<number>('maxCpu', { required: true })
const maxMem = defineModel<number>('maxMem', { required: true })
const reportGpu = defineModel<boolean>('reportGpu', { required: true })
</script>

<template>
  <div style="padding: 40px 20px;">
    <h3 style="margin-bottom: 20px;">共享资源</h3>
    <el-form label-width="160px">
      <el-form-item label="最大 CPU 核数">
        <el-slider v-model="maxCpu" :min="0" :max="64" :step="1" style="width: 300px;">
          <template #append>
            <span style="margin-left: 12px; min-width: 60px;">{{ maxCpu || '全部' }}</span>
          </template>
        </el-slider>
      </el-form-item>
      <el-form-item label="最大内存">
        <el-slider v-model="maxMem" :min="0" :max="65536" :step="1024" style="width: 300px;">
          <template #append>
            <span style="margin-left: 12px; min-width: 80px;">{{ maxMem ? (maxMem / 1024).toFixed(0) + ' GB' : '全部' }}</span>
          </template>
        </el-slider>
      </el-form-item>
      <el-form-item label="共享 GPU">
        <el-switch v-model="reportGpu" />
      </el-form-item>
      <p style="color: #909399; font-size: 13px; margin-top: 8px;">
        设置为 0 表示共享全部可用资源。您随时可以在设置中调整这些限制。
      </p>
    </el-form>
  </div>
</template>