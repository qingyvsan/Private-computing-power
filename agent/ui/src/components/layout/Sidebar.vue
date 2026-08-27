<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { getLocalStatus, type LocalStatus } from '../../api/client'

const router = useRouter()
const route = useRoute()
const localStatus = ref<LocalStatus | null>(null)

const menuItems = [
  { path: '/dashboard', label: '仪表盘', icon: 'Monitor' },
  { path: '/nodes', label: '节点', icon: 'Connection' },
  { path: '/jobs', label: '作业', icon: 'List' },
  { path: '/trust', label: '信任', icon: 'Link' },
  { path: '/invite', label: '邀请码', icon: 'Key' },
  { path: '/settings', label: '设置', icon: 'Setting' },
]

onMounted(async () => {
  try {
    localStatus.value = await getLocalStatus()
  } catch {}
})
</script>

<template>
  <el-aside width="200px" style="background: #304156;">
    <div style="height: 50px; display: flex; align-items: center; justify-content: center; color: #fff; font-size: 16px; font-weight: 600; border-bottom: 1px solid rgba(255,255,255,0.1);">
      CP Console
    </div>
    <el-menu
      :default-active="route.path"
      background-color="#304156"
      text-color="#bfcbd9"
      active-text-color="#409eff"
      style="border-right: none;"
      @select="(index: string) => router.push(index)"
    >
      <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">
        <el-icon><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
      </el-menu-item>
    </el-menu>
    <div style="position: absolute; bottom: 16px; left: 0; right: 0; padding: 0 20px; color: rgba(255,255,255,0.5); font-size: 12px;">
      <div v-if="localStatus" style="display: flex; align-items: center; gap: 6px; margin-bottom: 4px;">
        <span style="width: 8px; height: 8px; border-radius: 50%; background: #67c23a; display: inline-block;"></span>
        {{ localStatus.agent_name }}
      </div>
      <div v-if="localStatus" style="font-size: 11px;">
        {{ localStatus.node_id || 'Not registered' }}
      </div>
    </div>
  </el-aside>
</template>