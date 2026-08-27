<script setup lang="ts">
const address = defineModel<string>({ required: true })
</script>

<template>
  <div style="padding: 40px 20px;">
    <h3 style="margin-bottom: 20px;">连接到调度器</h3>
    <el-form label-width="120px">
      <el-form-item label="调度器地址">
        <el-input v-model="address" placeholder="localhost:9090">
          <template #prefix>
            <el-icon><Connection /></el-icon>
          </template>
        </el-input>
      </el-form-item>
      <p style="color: #909399; font-size: 13px; margin-top: 8px;">
        输入您的调度器 gRPC 地址（IP:端口）。如果在本机运行，使用 localhost:9090。
      </p>
    </el-form>
  </div>
</template>