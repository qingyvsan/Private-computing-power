<script setup lang="ts">
import Sidebar from './Sidebar.vue'
</script>

<template>
  <el-container style="height: 100vh">
    <Sidebar />
    <el-container>
      <el-header height="50px" style="background: #fff; border-bottom: 1px solid #e4e7ed; display: flex; align-items: center; padding: 0 20px;">
        <span style="font-size: 14px; color: #606266;">Computing Power Console</span>
      </el-header>
      <el-main style="background: #f5f7fa; padding: 0;">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>