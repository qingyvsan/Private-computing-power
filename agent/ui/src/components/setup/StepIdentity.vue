<script setup lang="ts">
const name = defineModel<string>('name', { required: true })
const code = defineModel<string>('code', { required: true })
</script>

<template>
  <div style="padding: 40px 20px;">
    <h3 style="margin-bottom: 20px;">节点身份</h3>
    <el-form label-width="120px">
      <el-form-item label="节点名称">
        <el-input v-model="name" placeholder="my-node">
          <template #prefix>
            <el-icon><User /></el-icon>
          </template>
        </el-input>
      </el-form-item>
      <el-form-item label="邀请码">
        <el-input v-model="code" placeholder="可选" show-password>
          <template #prefix>
            <el-icon><Key /></el-icon>
          </template>
        </el-input>
      </el-form-item>
      <p style="color: #909399; font-size: 13px; margin-top: 8px;">
        邀请码由调度器管理员提供。如果没有邀请码，可留空。
      </p>
    </el-form>
  </div>
</template>