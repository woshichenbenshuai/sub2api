<template>
  <BaseDialog
    :show="show"
    :title="`${t('admin.accounts.testConnection')} (${accounts.length})`"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-700 dark:border-dark-500 dark:bg-dark-700/40 dark:text-gray-200">
        {{ t('admin.accounts.bulkActions.selected', { count: accounts.length }) }}
      </div>

      <div class="space-y-1.5">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.selectTestModel') }}
        </label>
        <Select
          v-model="selectedModelId"
          :options="availableModels"
          value-key="id"
          label-key="display_name"
          :disabled="loadingModels || status === 'testing'"
          :placeholder="loadingModels ? `${t('common.loading')}...` : t('admin.accounts.selectTestModel')"
        />
      </div>

      <div
        ref="terminalRef"
        class="max-h-[320px] min-h-[180px] overflow-y-auto rounded-xl border border-gray-700 bg-gray-900 p-4 font-mono text-sm dark:border-gray-800 dark:bg-black"
      >
        <div v-if="status === 'idle'" class="text-gray-500">
          {{ t('admin.accounts.readyToTest') }}
        </div>
        <div v-else-if="status === 'testing'" class="mb-2 flex items-center gap-2 text-yellow-400">
          <Icon name="refresh" size="sm" class="animate-spin" :stroke-width="2" />
          <span>{{ t('admin.accounts.testing') }}</span>
        </div>

        <div v-for="(line, index) in outputLines" :key="index" :class="line.class">
          {{ line.text }}
        </div>
      </div>

      <div v-if="failedResults.length > 0" class="space-y-2">
        <div class="text-sm font-semibold text-red-600 dark:text-red-400">Failed Account Logs</div>
        <div
          v-for="item in failedResults"
          :key="item.id"
          class="rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800/60 dark:bg-red-900/20"
        >
          <div class="text-sm font-medium text-red-700 dark:text-red-300">
            {{ item.name }} (#{{ item.id }})
          </div>
          <div class="mt-1 text-xs text-red-600 dark:text-red-400">{{ item.error }}</div>
          <pre class="mt-2 whitespace-pre-wrap break-words rounded bg-white/80 p-2 text-xs text-gray-700 dark:bg-black/40 dark:text-gray-300">{{ item.logs.join('\n') || '-' }}</pre>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
          :disabled="status === 'testing'"
        >
          {{ t('common.close') }}
        </button>
        <button
          @click="startTest"
          :disabled="status === 'testing' || loadingModels || !selectedModelId || accounts.length === 0"
          class="rounded-lg bg-primary-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {{ status === 'testing' ? t('admin.accounts.testing') : t('admin.accounts.startTest') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { Icon } from '@/components/icons'
import { adminAPI } from '@/api/admin'
import type { Account, ClaudeModel } from '@/types'

interface OutputLine {
  text: string
  class: string
}

interface FailedResult {
  id: number
  name: string
  error: string
  logs: string[]
}

const props = defineProps<{
  show: boolean
  accounts: Account[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'completed', payload: { failedIds: number[]; successCount: number; failedCount: number }): void
}>()

const { t } = useI18n()
const terminalRef = ref<HTMLElement | null>(null)
const status = ref<'idle' | 'testing' | 'done'>('idle')
const loadingModels = ref(false)
const availableModels = ref<ClaudeModel[]>([])
const selectedModelId = ref('')
const outputLines = ref<OutputLine[]>([])
const failedResults = ref<FailedResult[]>([])
const successCount = ref(0)

const addLine = (text: string, className: string = 'text-gray-300') => {
  outputLines.value.push({ text, class: className })
  scrollToBottom()
}

const scrollToBottom = async () => {
  await nextTick()
  if (terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight
  }
}

const resetState = () => {
  status.value = 'idle'
  loadingModels.value = false
  availableModels.value = []
  selectedModelId.value = ''
  outputLines.value = []
  failedResults.value = []
  successCount.value = 0
}

const pickDefaultModelId = (models: ClaudeModel[]): string => {
  if (models.length === 0) return ''
  const sonnetModel = models.find((model) => model.id.includes('sonnet'))
  return sonnetModel?.id || models[0].id
}

const loadModels = async () => {
  if (props.accounts.length === 0) return

  loadingModels.value = true
  try {
    const settled = await Promise.allSettled(
      props.accounts.map((account) => adminAPI.accounts.getAvailableModels(account.id))
    )

    const successfulLists = settled
      .filter((item): item is PromiseFulfilledResult<ClaudeModel[]> => item.status === 'fulfilled')
      .map((item) => item.value)
      .filter((list) => list.length > 0)

    if (successfulLists.length === 0) {
      availableModels.value = []
      selectedModelId.value = ''
      addLine('No available models found for selected accounts.', 'text-red-400')
      return
    }

    const allModelMap = new Map<string, ClaudeModel>()
    let intersectionIds: Set<string> | null = null

    for (const models of successfulLists) {
      const currentIds = new Set(models.map((model) => model.id))
      models.forEach((model) => {
        if (!allModelMap.has(model.id)) {
          allModelMap.set(model.id, model)
        }
      })
      if (intersectionIds === null) {
        intersectionIds = currentIds
      } else {
        intersectionIds = new Set([...intersectionIds].filter((id) => currentIds.has(id)))
      }
    }

    const sourceIds = intersectionIds && intersectionIds.size > 0
      ? [...intersectionIds]
      : [...allModelMap.keys()]

    availableModels.value = sourceIds
      .map((id) => allModelMap.get(id))
      .filter((model): model is ClaudeModel => Boolean(model))
    selectedModelId.value = pickDefaultModelId(availableModels.value)
  } catch (error) {
    console.error('Failed to load bulk test models:', error)
    addLine('Failed to load models.', 'text-red-400')
  } finally {
    loadingModels.value = false
  }
}

const runSingleTest = async (account: Account, modelId: string): Promise<{ success: boolean; error: string; logs: string[] }> => {
  const token = localStorage.getItem('auth_token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  const response = await fetch(`/api/v1/admin/accounts/${account.id}/test`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ model_id: modelId })
  })

  if (!response.ok) {
    return { success: false, error: `HTTP ${response.status}`, logs: [`HTTP ${response.status}`] }
  }

  const reader = response.body?.getReader()
  if (!reader) {
    return { success: false, error: 'No response body', logs: ['No response body'] }
  }

  const decoder = new TextDecoder()
  const logs: string[] = []
  let buffer = ''
  let result: { success: boolean; error: string } | null = null

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      const jsonStr = line.slice(6).trim()
      if (!jsonStr) continue

      try {
        const event = JSON.parse(jsonStr) as {
          type?: string
          text?: string
          model?: string
          success?: boolean
          error?: string
        }

        if (event.type === 'test_start') {
          logs.push(`model=${event.model || modelId}`)
        } else if (event.type === 'content' && event.text) {
          logs.push(event.text)
        } else if (event.type === 'error') {
          const errorMessage = event.error || 'Test failed'
          logs.push(`error: ${errorMessage}`)
          result = { success: false, error: errorMessage }
        } else if (event.type === 'test_complete') {
          if (event.success) {
            result = { success: true, error: '' }
          } else {
            const errorMessage = event.error || 'Test failed'
            logs.push(`error: ${errorMessage}`)
            result = { success: false, error: errorMessage }
          }
        }
      } catch {
        // ignore malformed event
      }
    }
  }

  if (!result) {
    return { success: false, error: 'No completion event', logs: logs.slice(-20) }
  }
  return { success: result.success, error: result.error, logs: logs.slice(-20) }
}

const startTest = async () => {
  if (status.value === 'testing') return
  if (!selectedModelId.value || props.accounts.length === 0) return

  status.value = 'testing'
  outputLines.value = []
  failedResults.value = []
  successCount.value = 0
  addLine(`Start bulk test. model=${selectedModelId.value}`, 'text-blue-400')

  for (let index = 0; index < props.accounts.length; index += 1) {
    const account = props.accounts[index]
    addLine(`[${index + 1}/${props.accounts.length}] ${account.name} (#${account.id})`, 'text-cyan-300')

    try {
      const result = await runSingleTest(account, selectedModelId.value)
      if (result.success) {
        successCount.value += 1
        addLine('  OK success', 'text-green-400')
      } else {
        failedResults.value.push({
          id: account.id,
          name: account.name,
          error: result.error || 'Test failed',
          logs: result.logs
        })
        addLine(`  FAIL ${result.error || 'Test failed'}`, 'text-red-400')
      }
    } catch (error: any) {
      const message = error?.message || 'Unknown error'
      failedResults.value.push({
        id: account.id,
        name: account.name,
        error: message,
        logs: [message]
      })
      addLine(`  FAIL ${message}`, 'text-red-400')
    }
  }

  const failedCount = failedResults.value.length
  addLine(`Done. success=${successCount.value}, failed=${failedCount}`, failedCount > 0 ? 'text-yellow-300' : 'text-green-400')

  if (failedCount > 0) {
    addLine('Failed account logs:', 'text-red-300')
    failedResults.value.forEach((item) => {
      addLine(`- ${item.name} (#${item.id}): ${item.error}`, 'text-red-300')
    })
  }

  emit('completed', {
    failedIds: failedResults.value.map((item) => item.id),
    successCount: successCount.value,
    failedCount
  })
  status.value = 'done'
}

const handleClose = () => {
  if (status.value === 'testing') return
  emit('close')
}

watch(
  () => props.show,
  async (visible) => {
    if (visible) {
      resetState()
      await loadModels()
    }
  }
)
</script>
