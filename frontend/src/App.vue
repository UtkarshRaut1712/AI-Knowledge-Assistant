
<script setup>
import { ref } from 'vue'

const documents = ref([])
const loading = ref(false)
const error = ref('')

const selectedFile = ref(null)
const uploadMessage = ref('')
const uploading = ref(false)

const chatMessages = ref([])
const chatInput = ref('')
const chatLoading = ref(false)
const chatError = ref('')
const currentChatId = ref('')

async function fetchDocuments() {
  loading.value = true
  error.value = ''

  try {
    const response = await fetch('/api/documents')

    if (!response.ok) {
      throw new Error('Could not load documents')
    }

    documents.value = await response.json()
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function deleteDocument(id) {
  const confirmed = confirm('Are you sure you want to delete this document?')

  if (!confirmed) return

  try {
    const response = await fetch(`/api/documents/${id}`, {
      method: 'DELETE'
    })

    if (!response.ok) {
      throw new Error('Could not delete document')
    }

    await fetchDocuments()
    alert('Document deleted successfully')
  } catch (err) {
    error.value = err.message
  }
}

function handleFileChange(event) {
  selectedFile.value = event.target.files[0] || null
  uploadMessage.value = ''
}

async function uploadDocument() {
  if (!selectedFile.value) {
    uploadMessage.value = 'Please select a file first.'
    return
  }

  const formData = new FormData()
  formData.append('file', selectedFile.value)

  uploading.value = true
  uploadMessage.value = ''

  try {
    const response = await fetch('/api/documents/upload', {
      method: 'POST',
      body: formData
    })

    const data = await response.json()

    if (!response.ok) {
      throw new Error(data.error || 'Document upload failed')
    }

    uploadMessage.value = 'Document uploaded successfully!'
    selectedFile.value = null

    await fetchDocuments()
  } catch (err) {
    uploadMessage.value = err.message
  } finally {
    uploading.value = false
  }
}


async function sendMessage() {
  const question = chatInput.value.trim()

  if (!question || chatLoading.value) {
    return
  }

  chatMessages.value.push({
    role: 'user',
    content: question,
    sources: []
  })

  chatInput.value = ''
  chatLoading.value = true
  chatError.value = ''

  try {
    const response = await fetch('/api/chat', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        chat_id: currentChatId.value || undefined,
        message: question
      })
    })

    const data = await response.json().catch(() => ({}))

    if (!response.ok) {
      throw new Error(data.error || 'Could not generate AI response')
    }

    currentChatId.value = data.chat_id

    chatMessages.value.push({
      role: 'assistant',
      content: data.assistant_message.content,
      sources: data.sources || []
    })

  } catch (err) {
    chatError.value = err.message
  } finally {
    chatLoading.value = false
  }
}


fetchDocuments()
</script>

<template>
  <div class="dashboard">
    <header class="header">
      <div>
        <h1>AI Knowledge Assistant</h1>
        <p>Upload documents and ask questions using AI</p>
      </div>
      <span class="status">● AI Assistant</span>
    </header>

    <main class="content">
      <section class="panel">
        <h2>Document Management</h2>
        <p>Upload and manage your knowledge documents.</p>

        <div class="upload-area">
          <h3>Upload Documents</h3>
          <p>PDF and TXT files are supported.</p>
          <input
  type="file"
  accept=".pdf,.txt"
  @change="handleFileChange"
/>

<button
  @click="uploadDocument"
  :disabled="uploading || !selectedFile"
>
  {{ uploading ? 'Uploading...' : 'Upload Document' }}
</button>

<p v-if="uploadMessage">{{ uploadMessage }}</p>

        </div>

        <div class="document-heading">
          <h3>Uploaded Documents</h3>
          <button @click="fetchDocuments">Refresh</button>
        </div>

        <p v-if="loading">Loading documents...</p>
        <p v-else-if="error" class="error">{{ error }}</p>

        <div v-else-if="documents.length">
          <div
            v-for="document in documents"
            :key="document.id"
            class="document"
          >
          <button
  class="delete-button"
  @click="deleteDocument(document.id)"
>
  Delete
</button>
            <div>
              <strong>{{ document.name }}</strong>
              <small>ID: {{ document.id }}</small>
            </div>
            <span class="uploaded">{{ document.status }}</span>
          </div>
        </div>

        <p v-else>No documents uploaded yet.</p>
      </section>

      
<section class="panel">
  <h2>AI Chat Assistant</h2>
  <p>Ask questions about your uploaded documents.</p>

  <div class="chat-window">
    <div v-if="chatMessages.length === 0" class="chat-placeholder">
      <div class="chat-icon">✦</div>
      <h3>Your AI Assistant</h3>
      <p>Ask anything about your uploaded documents.</p>
    </div>

    <div
      v-for="(message, index) in chatMessages"
      :key="index"
      class="message"
      :class="message.role"
    >
      <strong>
        {{ message.role === 'user' ? 'You' : 'AI Assistant' }}
      </strong>

      <p>{{ message.content }}</p>

      <div v-if="message.sources && message.sources.length" class="sources">
        <strong>Sources:</strong>

        <div v-for="(source, sourceIndex) in message.sources" :key="sourceIndex">
          Document ID: {{ source.document_id }} |
          Chunk ID: {{ source.chunk_id }} |
          Similarity: {{ Number(source.similarity).toFixed(3) }}
        </div>
      </div>
    </div>

    <p v-if="chatLoading" class="loading-message">
      AI is thinking...
    </p>

    <p v-if="chatError" class="error">
      {{ chatError }}
    </p>
  </div>

  <form class="chat-form" @submit.prevent="sendMessage">
    <input
      v-model="chatInput"
      type="text"
      placeholder="Ask a question about your documents..."
      :disabled="chatLoading"
    />

    <button type="submit" :disabled="chatLoading || !chatInput.trim()">
      {{ chatLoading ? 'Sending...' : 'Send' }}
    </button>
  </form>
</section>

    </main>
  </div>
</template>

<style>
* {
  box-sizing: border-box;
}

body {
  margin: 0;
  font-family: Arial, sans-serif;
  background: #f3f5f9;
  color: #202b3c;
}

.dashboard {
  min-height: 100vh;
}

.header {
  background: #172554;
  color: white;
  padding: 25px 6%;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header h1 {
  margin: 0 0 8px;
}

.header p {
  margin: 0;
  color: #cbd5e1;
}

.status {
  color: #86efac;
}

.content {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
  padding: 35px 6%;
}

.panel {
  background: white;
  border-radius: 12px;
  padding: 25px;
  box-shadow: 0 3px 15px #0000000a;
}

.panel h2 {
  margin-top: 0;
}

.panel > p {
  color: #64748b;
}

.upload-area {
  margin: 25px 0;
  padding: 30px;
  border: 2px dashed #94a3b8;
  border-radius: 10px;
  text-align: center;
  background: #f8fafc;
}

.upload-area small,
.chat-placeholder small {
  display: block;
  margin-top: 15px;
  color: #64748b;
}

.document-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

button {
  background: #2563eb;
  color: white;
  border: none;
  border-radius: 6px;
  padding: 9px 15px;
  cursor: pointer;
}

.document {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 15px 0;
  border-bottom: 1px solid #e2e8f0;
}

.document small {
  display: block;
  margin-top: 6px;
  color: #64748b;
}

.uploaded {
  color: #15803d;
  font-size: 13px;
}

.error {
  color: #dc2626 !important;
}

.chat-placeholder {
  margin-top: 25px;
  min-height: 300px;
  background: #f8fafc;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  text-align: center;
  padding: 20px;
}

.chat-placeholder p {
  color: #64748b;
}

.chat-icon {
  font-size: 40px;
  color: #2563eb;
}

@media (max-width: 750px) {
  .content {
    grid-template-columns: 1fr;
  }

  .header {
    gap: 15px;
    flex-wrap: wrap;
  }
}


.chat-window {
  height: 350px;
  overflow-y: auto;
  padding: 15px;
  background: #f8fafc;
  border-radius: 10px;
  margin-top: 20px;
}

.message {
  padding: 12px;
  margin-bottom: 12px;
  border-radius: 8px;
  max-width: 90%;
  overflow-wrap: anywhere;
}

.message p {
  margin-bottom: 0;
  white-space: pre-wrap;
}

.message.user {
  background: #dbeafe;
  margin-left: auto;
}

.message.assistant {
  background: white;
  border: 1px solid #e2e8f0;
  margin-right: auto;
}

.sources {
  margin-top: 12px;
  padding-top: 8px;
  border-top: 1px solid #e2e8f0;
  font-size: 12px;
  color: #475569;
}

.chat-form {
  display: flex;
  gap: 10px;
  margin-top: 15px;
}

.chat-form input {
  flex: 1;
  min-width: 0;
  padding: 12px;
  border: 1px solid #cbd5e1;
  border-radius: 7px;
}

.chat-form button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.loading-message {
  color: #2563eb;
  font-style: italic;
}

.delete-button {
  background: #dc2626;
  color: white;
  border: none;
  padding: 8px 12px;
  border-radius: 6px;
  cursor: pointer;
}

.delete-button:hover {
  background: #b91c1c;
}
</style>
