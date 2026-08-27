<script setup lang="ts">
import { ref } from 'vue'
import { createInviteCode, redeemInviteCode } from '../api/client'
import { ElMessage } from 'element-plus'

const createdCode = ref('')
const createdCodeInput = ref('')
const creating = ref(false)
const redeemCode = ref('')
const redeemNodeID = ref('')
const redeeming = ref(false)

async function handleCreate() {
  creating.value = true
  createdCode.value = ''
  try {
    const resp: any = await createInviteCode()
    createdCode.value = resp.code || resp.invite_code || JSON.stringify(resp)
    ElMessage.success('邀请码已生成')
  } catch (e: any) {
    ElMessage.error(e.message || '生成失败')
  }
  creating.value = false
}

async function handleRedeem() {
  if (!redeemCode.value.trim() || !redeemNodeID.value.trim()) return
  redeeming.value = true
  try {
    await redeemInviteCode(redeemCode.value.trim(), redeemNodeID.value.trim())
    ElMessage.success('邀请码已兑换')
    redeemCode.value = ''
    redeemNodeID.value = ''
  } catch (e: any) {
    ElMessage.error(e.message || '兑换失败')
  }
  redeeming.value = false
}

function handleCopy(text: string) {
  navigator.clipboard?.writeText(text)
  ElMessage.success('已复制')
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <h2>邀请码管理</h2>
    </div>

    <el-row :gutter="16">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>创建邀请码</span>
          </template>
          <p style="color: #606266; margin-bottom: 16px; line-height: 1.6;">
            创建一个新的邀请码，其他节点可以使用此邀请码加入集群。
          </p>
          <el-button type="primary" :loading="creating" @click="handleCreate">创建邀请码</el-button>
          <div v-if="createdCode" style="margin-top: 16px;">
            <p style="margin-bottom: 8px; font-weight: 500;">邀请码:</p>
            <el-input ref="createdCodeInput" :model-value="createdCode" readonly>
              <template #append>
                <el-button @click="handleCopy(createdCode)">复制</el-button>
              </template>
            </el-input>
          </div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>兑换邀请码</span>
          </template>
          <el-form @submit.prevent="handleRedeem">
            <el-form-item label="邀请码">
              <el-input v-model="redeemCode" placeholder="输入邀请码" />
            </el-form-item>
            <el-form-item label="节点 ID">
              <el-input v-model="redeemNodeID" placeholder="输入节点 ID" />
            </el-form-item>
            <el-button type="primary" :loading="redeeming" :disabled="!redeemCode.trim() || !redeemNodeID.trim()" @click="handleRedeem" style="width: 100%;">兑换</el-button>
          </el-form>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>