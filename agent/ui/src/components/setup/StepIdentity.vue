<script setup lang="ts">
const name = defineModel<string>('name', { required: true })
const code = defineModel<string>('code', { required: true })
const dataDir = defineModel<string>('dataDir', { required: true })
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
      <el-form-item label="数据目录">
        <el-input v-model="dataDir" placeholder="./data/cpstart">
          <template #prefix>
            <el-icon><FolderOpened /></el-icon>
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
        数据目录用于存储配置文件、Agent 数据和下载内容。邀请码由调度器管理员提供，没有可留空。
      </p>
    </el-form>
  </div>
</template>