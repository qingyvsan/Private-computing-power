<script setup lang="ts">
import { computed } from 'vue'
import { statusColor } from '../../utils/format'

const props = defineProps<{
  status: string
  size?: 'small' | 'default'
}>()

const dotColor = computed(() => statusColor(props.status))
</script>

<template>
  <span style="display: inline-flex; align-items: center; gap: 4px;">
    <span :style="{ width: size === 'small' ? '6px' : '8px', height: size === 'small' ? '6px' : '8px', borderRadius: '50%', backgroundColor: dotColor, display: 'inline-block' }"></span>
    <span :style="{ fontSize: size === 'small' ? '12px' : '13px' }">{{ status }}</span>
  </span>
</template>