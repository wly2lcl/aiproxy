const API_BASE = ''

const app = Vue.createApp({
  data() {
    return {
      currentView: 'dashboard',
      adminKey: '',
      hasStoredKey: false,
      accounts: [],
      providers: [],
      stats: {},
      health: { status: '', timestamp: '', checks: {} },
      healthStatus: '',
      loading: false,
      toasts: [],
      toastId: 0,
      charts: {},
      lang: localStorage.getItem('aiproxy_lang') || 'zh',
      autoRefresh: false,
      autoRefreshTimer: null,
      version: { version: 'unknown', build_time: 'unknown' },
      modelMapping: {},
      logs: [],
      accountFilter: { provider: '', status: '', available: '' },
      selectedAccounts: [],
      timeSeries: [],
      timeSeriesRange: '24',
      accountStats: [],
      modelStats: [],
      apiKeys: [],
      accountModelStats: {},
      confirmModal: { show: false, title: '', message: '', onConfirm: () => {}, onCancel: () => {} },
      accountModal: { show: false, isEdit: false, form: { id: '', provider_id: '', api_key: '', weight: 1, priority: 1, enabled: true } },
      accountLimitsModal: { show: false, account: null, limits: [] },
      providerModal: { show: false, provider: null },
      logDetailModal: { show: false, log: null },
      accountModelsModal: { show: false, accountId: '', models: {} },
      apiKeyModal: { show: false, newKey: null }
    }
  },
  computed: {
    t() { return (key) => translations[this.lang][key] || key },
    navItems() {
      return [
        { id: 'dashboard', label: this.t('dashboard'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"/></svg>' },
        { id: 'accounts', label: this.t('accounts'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"/></svg>' },
        { id: 'providers', label: this.t('providers'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>' },
        { id: 'logs', label: this.t('logs'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>' },
        { id: 'stats', label: this.t('stats'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>' },
        { id: 'apikeys', label: this.t('apiKeys'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>' },
        { id: 'settings', label: this.t('settings'), icon: '<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>' }
      ]
    },
    currentViewTitle() { return this.navItems.find(i => i.id === this.currentView)?.label || '' },
    errorRate() { return this.stats.total_requests ? (this.stats.total_errors / this.stats.total_requests) * 100 : 0 },
    topModels() { return Object.entries(this.stats.by_model || {}).sort((a, b) => b[1] - a[1]).slice(0, 10).reduce((acc, [k, v]) => { acc[k] = v; return acc }, {}) },
    filteredAccounts() {
      return this.accounts.filter(a => {
        if (this.accountFilter.provider && a.provider_id !== this.accountFilter.provider) return false
        if (this.accountFilter.status === 'enabled' && !a.is_enabled) return false
        if (this.accountFilter.status === 'disabled' && a.is_enabled) return false
        if (this.accountFilter.available === 'true' && !a.available) return false
        if (this.accountFilter.available === 'false' && a.available) return false
        return true
      })
    },
    providerList() { return this.providers.map(p => p.name || p.id) }
  },
  mounted() {
    this.loadAdminKey()
    this.fetchHealth()
    if (this.adminKey) this.fetchAll()
  },
  watch: {
    currentView(v) {
      if (v === 'stats') {
        this.$nextTick(() => {
          this.initCharts()
          this.initTrendChart()
        })
      }
    },
    autoRefresh(v) {
      if (v) {
        this.autoRefreshTimer = setInterval(() => this.fetchAll(), 5000)
      } else {
        clearInterval(this.autoRefreshTimer)
      }
    },
    timeSeriesRange() {
      this.fetchTimeSeries()
    }
  },
  methods: {
    toggleLang() { this.lang = this.lang === 'zh' ? 'en' : 'zh'; localStorage.setItem('aiproxy_lang', this.lang) },
    getHeaders() { return { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + this.adminKey } },
    async apiCall(endpoint, options = {}) {
      const res = await fetch(`${API_BASE}${endpoint}`, { ...options, headers: { ...this.getHeaders(), ...options.headers } })
      if (!res.ok) { const e = await res.json().catch(() => ({ error: 'Request failed' })); throw new Error(e.error || `HTTP ${res.status}`) }
      return res.json()
    },
    async fetchAll() { await Promise.all([this.fetchAccounts(), this.fetchProviders(), this.fetchStats(), this.fetchLogs(), this.fetchVersion(), this.fetchModelMapping(), this.fetchTimeSeries(), this.fetchAccountStats(), this.fetchModelStats(), this.fetchAPIKeys()]) },
    async fetchAccounts() {
      try {
        const data = await this.apiCall('/admin/accounts')
        this.accounts = (data.accounts || []).map(a => ({ ...a, provider: a.provider_id, enabled: a.is_enabled }))
      } catch (e) { this.showToast(this.t('noAccounts') + ': ' + e.message, 'error') }
    },
    async fetchProviders() {
      try {
        const data = await this.apiCall('/admin/providers/stats')
        this.providers = (data.providers || []).map(p => ({ ...p, id: p.name, enabled: true }))
      } catch (e) { this.showToast(this.t('noProviders') + ': ' + e.message, 'error') }
    },
    async fetchStats() {
      try {
        const d = await this.apiCall('/admin/stats')
        this.stats = {
          total_requests: d.total_requests || 0, total_tokens: d.total_tokens || 0, total_errors: d.total_errors || 0,
          by_provider: d.requests_by_provider || {}, by_model: d.requests_by_model || {}, tokens_by_provider: d.tokens_by_provider || {},
          latency: { avg_ms: d.avg_latency_ms || 0, p50_ms: d.latency_percentiles?.p50 || 0, p95_ms: d.latency_percentiles?.p95 || 0, p99_ms: d.latency_percentiles?.p99 || 0 },
          ttft: { avg_ms: d.avg_ttft_ms || 0, p50_ms: d.ttft_percentiles?.p50 || 0, p95_ms: d.ttft_percentiles?.p95 || 0, p99_ms: d.ttft_percentiles?.p99 || 0 }
        }
      } catch (e) {}
    },
    async fetchHealth() { try { const h = await this.apiCall('/admin/health'); this.health = h; this.healthStatus = h.status } catch (e) { this.healthStatus = 'unhealthy' } },
    async fetchLogs() { try { const d = await this.apiCall('/admin/logs?limit=100'); this.logs = d.logs || [] } catch (e) {} },
    async fetchVersion() { try { const d = await this.apiCall('/admin/version'); this.version = d } catch (e) {} },
    async fetchModelMapping() { try { const d = await this.apiCall('/admin/model-mapping'); this.modelMapping = d.model_mapping || {} } catch (e) {} },
    async fetchTimeSeries() {
      try { const d = await this.apiCall(`/admin/stats/timeseries?hours=${this.timeSeriesRange}`); this.timeSeries = d.timeseries || [] }
      catch (e) {}
    },
    async fetchAccountStats() {
      try { const d = await this.apiCall('/admin/stats/accounts?hours=24'); this.accountStats = d.account_stats || [] }
      catch (e) {}
    },
    async fetchModelStats() {
      try { const d = await this.apiCall('/admin/stats/models?hours=24'); this.modelStats = d.model_stats || [] }
      catch (e) {}
    },
    async fetchAccountLimits(id) {
      try { const d = await this.apiCall(`/admin/accounts/${id}/limits`); return d.limits || [] }
      catch (e) { return [] }
    },
    openAddAccountModal() { this.accountModal = { show: true, isEdit: false, form: { id: '', provider_id: this.providerList[0] || '', api_key: '', weight: 1, priority: 1, enabled: true } } },
    openEditAccountModal(a) { this.accountModal = { show: true, isEdit: true, form: { id: a.id, provider_id: a.provider_id, api_key: '', weight: a.weight, priority: a.priority, enabled: a.is_enabled } } },
    async openAccountLimitsModal(a) {
      const limits = await this.fetchAccountLimits(a.id)
      this.accountLimitsModal = { show: true, account: a, limits }
    },
    openLogDetailModal(log) { this.logDetailModal = { show: true, log } },
    async saveAccount() {
      this.loading = true
      try {
        if (this.accountModal.isEdit) {
          await this.apiCall(`/admin/accounts/${this.accountModal.form.id}`, { method: 'PUT', body: JSON.stringify({ weight: this.accountModal.form.weight, priority: this.accountModal.form.priority, is_enabled: this.accountModal.form.enabled }) })
          this.showToast(this.t('accountUpdated'), 'success')
        } else {
          await this.apiCall('/admin/accounts', { method: 'POST', body: JSON.stringify(this.accountModal.form) })
          this.showToast(this.t('accountCreated'), 'success')
        }
        this.accountModal.show = false; await this.fetchAccounts()
      } catch (e) { this.showToast(e.message, 'error') } finally { this.loading = false }
    },
    deleteAccount(id) {
      this.showConfirm(this.t('deleteAccount'), this.t('deleteAccountMsg'), async () => {
        try { await this.apiCall(`/admin/accounts/${id}`, { method: 'DELETE' }); this.showToast(this.t('accountDeleted'), 'success'); await this.fetchAccounts() }
        catch (e) { this.showToast(e.message, 'error') }
      })
    },
    resetAccountLimits(id) {
      this.showConfirm(this.t('resetLimits'), this.t('resetLimitsMsg'), async () => {
        try { await this.apiCall(`/admin/accounts/${id}/reset`, { method: 'POST' }); this.showToast(this.t('limitsReset'), 'success'); await this.fetchAccounts() }
        catch (e) { this.showToast(e.message, 'error') }
      })
    },
    toggleSelectAccount(id) {
      const idx = this.selectedAccounts.indexOf(id)
      if (idx > -1) { this.selectedAccounts.splice(idx, 1) } else { this.selectedAccounts.push(id) }
    },
    toggleSelectAll() {
      if (this.selectedAccounts.length === this.filteredAccounts.length) { this.selectedAccounts = [] }
      else { this.selectedAccounts = this.filteredAccounts.map(a => a.id) }
    },
    async batchOperation(action) {
      if (!this.selectedAccounts.length) return
      this.showConfirm(this.t('confirm'), `${this.t(action)} ${this.selectedAccounts.length} ${this.t('accountsCount')}?`, async () => {
        try {
          await this.apiCall('/admin/accounts/batch', { method: 'POST', body: JSON.stringify({ action, account_ids: this.selectedAccounts }) })
          this.showToast('Success', 'success')
          this.selectedAccounts = []
          await this.fetchAccounts()
        } catch (e) { this.showToast(e.message, 'error') }
      })
    },
    exportData(type) {
      fetch(`${API_BASE}/admin/export/${type}`, {
        headers: this.getHeaders()
      })
      .then(res => res.text())
      .then(csv => {
        const blob = new Blob([csv], { type: 'text/csv' })
        const url = window.URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `${type}.csv`
        a.click()
        window.URL.revokeObjectURL(url)
      })
      .catch(e => this.showToast(e.message, 'error'))
    },
    openProviderModal(p) { this.providerModal = { show: true, provider: p } },
    async reloadConfig() {
      this.showConfirm(this.t('reloadConfigTitle'), this.t('reloadConfigMsg'), async () => {
        this.loading = true
        try { await this.apiCall('/admin/reload', { method: 'POST' }); this.showToast(this.t('configReloaded'), 'success'); await this.fetchAll() }
        catch (e) { this.showToast(e.message, 'error') } finally { this.loading = false }
      })
    },
    loadAdminKey() { const s = localStorage.getItem('aiproxy_admin_key'); if (s) { this.adminKey = s; this.hasStoredKey = true } },
    saveAdminKey() { if (this.adminKey) { localStorage.setItem('aiproxy_admin_key', this.adminKey); this.hasStoredKey = true; this.fetchAll() } else { localStorage.removeItem('aiproxy_admin_key'); this.hasStoredKey = false } },
    showToast(message, type = 'info') { const id = ++this.toastId; this.toasts.push({ id, message, type }); setTimeout(() => this.removeToast(id), 5000) },
    removeToast(id) { this.toasts = this.toasts.filter(t => t.id !== id) },
    showConfirm(title, message, onConfirm) { this.confirmModal = { show: true, title, message, onConfirm: () => { this.confirmModal.show = false; onConfirm() }, onCancel: () => { this.confirmModal.show = false } } },
    initCharts() {
      if (this.charts.requests) this.charts.requests.destroy()
      if (this.charts.tokens) this.charts.tokens.destroy()
      const rCtx = document.getElementById('requestsChart'), tCtx = document.getElementById('tokensChart')
      if (!rCtx || !tCtx) return
      const colors = ['#3b82f6', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#84cc16']
      const opts = { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true, grid: { color: '#374151' }, ticks: { color: '#9ca3af' } }, x: { grid: { display: false }, ticks: { color: '#9ca3af' } } } }
      this.charts.requests = new Chart(rCtx, { type: 'bar', data: { labels: Object.keys(this.stats.by_provider || {}), datasets: [{ data: Object.values(this.stats.by_provider || {}), backgroundColor: colors }] }, options: opts })
      this.charts.tokens = new Chart(tCtx, { type: 'bar', data: { labels: Object.keys(this.stats.tokens_by_provider || {}), datasets: [{ data: Object.values(this.stats.tokens_by_provider || {}), backgroundColor: colors }] }, options: opts })
    },
    initTrendChart() {
      if (this.charts.trend) this.charts.trend.destroy()
      const ctx = document.getElementById('trendChart')
      if (!ctx || !this.timeSeries.length) return
      const labels = this.timeSeries.map(p => p.Timestamp ? new Date(p.Timestamp).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'}) : '')
      this.charts.trend = new Chart(ctx, {
        type: 'line',
        data: {
          labels,
          datasets: [
            { label: this.t('requests'), data: this.timeSeries.map(p => p.Count), borderColor: '#3b82f6', backgroundColor: 'rgba(59, 130, 246, 0.1)', fill: true, tension: 0.3 },
            { label: this.t('tokens'), data: this.timeSeries.map(p => p.Tokens), borderColor: '#22c55e', backgroundColor: 'rgba(34, 197, 94, 0.1)', fill: true, tension: 0.3, yAxisID: 'y1' }
          ]
        },
        options: {
          responsive: true, maintainAspectRatio: false,
          scales: {
            y: { beginAtZero: true, position: 'left', grid: { color: '#374151' }, ticks: { color: '#9ca3af' } },
            y1: { beginAtZero: true, position: 'right', grid: { drawOnChartArea: false }, ticks: { color: '#22c55e' } },
            x: { grid: { display: false }, ticks: { color: '#9ca3af', maxTicksLimit: 12 } }
          },
          plugins: { legend: { labels: { color: '#9ca3af' } } }
        }
      })
    },
    formatNumber(n) { if (!n) return '0'; if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'; if (n >= 1000) return (n / 1000).toFixed(1) + 'K'; return n.toString() },
    formatDate(d) { return d && d !== '' ? new Date(d).toLocaleString() : '' },
    formatDuration(ms) { if (!ms) return '-'; if (ms < 1000) return ms.toFixed(0) + 'ms'; return (ms / 1000).toFixed(2) + 's' },
    formatPercent(v) { return v ? v.toFixed(1) + '%' : '0%' },
    async fetchAPIKeys() {
      try { const d = await this.apiCall('/admin/api-keys'); this.apiKeys = d.api_keys || [] }
      catch (e) {}
    },
    async fetchAccountModelStats(accountId) {
      try { const d = await this.apiCall(`/admin/accounts/${accountId}/models?hours=24`); return d.model_stats || {} }
      catch (e) { return {} }
    },
    async openLogDetailModal(log) {
      try {
        const d = await this.apiCall(`/admin/logs/${log.RequestID}`)
        this.logDetailModal = { show: true, log: d.log }
      } catch (e) { this.showToast(e.message, 'error') }
    },
    async openAccountModelsModal(accountId) {
      const models = await this.fetchAccountModelStats(accountId)
      this.accountModelsModal = { show: true, accountId, models }
    },
    async createAPIKey() {
      const name = prompt(this.t('keyName'))
      if (!name) return
      this.loading = true
      try {
        const d = await this.apiCall('/admin/api-keys', { method: 'POST', body: JSON.stringify({ name }) })
        this.apiKeyModal = { show: true, newKey: d }
        this.showToast(this.t('apiKeyCreated'), 'success')
        await this.fetchAPIKeys()
      } catch (e) { this.showToast(e.message, 'error') } finally { this.loading = false }
    },
    async deleteAPIKey(id) {
      this.showConfirm(this.t('deleteAPIKey'), this.t('deleteAPIKeyMsg'), async () => {
        try {
          await this.apiCall(`/admin/api-keys/${id}`, { method: 'DELETE' })
          this.showToast(this.t('apiKeyDeleted'), 'success')
          await this.fetchAPIKeys()
        } catch (e) { this.showToast(e.message, 'error') }
      })
    },
    async toggleAPIKey(id, enabled) {
      try {
        await this.apiCall(`/admin/api-keys/${id}/toggle`, { method: 'PUT', body: JSON.stringify({ enabled }) })
        await this.fetchAPIKeys()
      } catch (e) { this.showToast(e.message, 'error') }
    },
    copyToClipboard(text) {
      navigator.clipboard.writeText(text)
      this.showToast(this.t('copied'), 'success')
    }
  }
})