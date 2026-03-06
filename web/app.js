const I18N = {
  'zh-TW': {
    'app.title': 'CallFXO 控制台',
    'lang.label': '語言',
    'login.title': '登入',
    'login.username': '帳號',
    'login.password': '密碼',
    'login.button': '登入',
    'login.prompt': '請輸入帳號密碼。',
    'login.success': '登入成功',
    'login.failed': '登入失敗: {error}',
    'logout': '登出',

    'users.title': '使用者管理',
    'users.add': '新增',
    'users.col.id': 'ID',
    'users.col.username': '帳號',
    'users.col.role': '角色',
    'users.col.action': '操作',
    'users.delete': '刪除',
    'users.setpass': '改密碼',
    'users.deleteConfirm': '刪除使用者 {username} ?',
    'users.setPassPrompt': '輸入使用者 {username} 的新密碼',
    'users.setPassDone': '已更新 {username} 的密碼',
    'users.ph.username': '新帳號',
    'users.ph.password': '新密碼',

    'role.user': '一般',
    'role.admin': '管理員',

    'fxo.title': 'FXO 盒子管理',
    'fxo.add': '新增',
    'fxo.col.name': '名稱',
    'fxo.col.username': '帳號',
    'fxo.col.status': '狀態',
    'fxo.col.action': '操作',
    'fxo.edit': '編輯',
    'fxo.delete': '刪除',
    'fxo.deleteConfirm': '刪除 FXO {name} ?',
    'fxo.tip': '提示：FXO 盒子請使用這裡設定的 SIP 帳號密碼向本伺服器 REGISTER。',
    'fxo.ph.name': '顯示名稱',
    'fxo.ph.user': 'SIP 帳號',
    'fxo.ph.pass': 'SIP 密碼',
    'fxo.prompt.name': '名稱',
    'fxo.prompt.user': 'SIP 帳號',
    'fxo.prompt.pass': 'SIP 密碼(留空表示不變更)',
    'fxo.status.online': '在線',
    'fxo.status.offline': '離線',

    'dial.title': '撥號',
    'dial.boxLabel': '選擇 FXO 盒子',
    'dial.numberLabel': '電話號碼',
    'dial.ph.number': '例如 0912345678',
    'dial.button': '撥號',
    'dial.hangup': '掛斷',
    'dial.idle': '未通話',
    'dial.sentHangup': '已送出掛斷',

    'pwd.title': '修改密碼',
    'pwd.ph.old': '舊密碼',
    'pwd.ph.new': '新密碼',
    'pwd.button': '修改',
    'pwd.changed': '密碼已更新',
    'pwd.needOldNew': '請輸入舊密碼與新密碼',

    'logs.title': '撥號紀錄',
    'logs.col.time': '時間',
    'logs.col.box': '盒子',
    'logs.col.number': '號碼',
    'logs.col.status': '狀態',
    'logs.col.reason': '原因',
    'logs.empty': '暫無撥號紀錄',
    'logs.prev': '上一頁',
    'logs.next': '下一頁',
    'logs.page': '第 {page} / {total} 頁',

    'contacts.title': '通訊錄',
    'contacts.add': '新增',
    'contacts.col.name': '姓名',
    'contacts.col.number': '電話',
    'contacts.col.action': '操作',
    'contacts.delete': '刪除',
    'contacts.deleteConfirm': '刪除聯絡人 {name} ?',
    'contacts.empty': '暫無通訊錄資料',
    'contacts.ph.search': '搜尋名稱或電話',
    'contacts.ph.name': '姓名',
    'contacts.ph.number': '電話',

    'audio.volume': '播放音量',

    'ws.disconnected': 'WebSocket: 未連線',
    'ws.connected': 'WebSocket: 已連線',
    'ws.closed': 'WebSocket: 已斷線',
    'ws.error': 'WebSocket: 錯誤',

    'call.connected': 'SIP 已接通，通話中',
    'call.ended': '通話結束: {reason}',
    'call.wsReconnect': '信令斷線，正在自動重連（通話嘗試維持）...',

    'error.prefix': '錯誤: {error}',
    'error.dialFailed': '撥號失敗: {error}',
    'error.remoteAudio': '遠端音訊處理失敗: {error}',

    'alert.needBoxNumber': '請選擇盒子與輸入號碼',
    'alert.wsNotConnected': 'WebSocket 尚未連線',
    'alert.callActive': '目前已有通話',
    'alert.noPCMU': '此瀏覽器不支援 PCMU，無法在無轉碼模式下通話',

    'state.new': '初始化',
    'state.connecting': '連線中',
    'state.connected': '已連線',
    'state.disconnected': '已斷線',
    'state.failed': '失敗',
    'state.closed': '已關閉',
    'state.dialing': '撥號中',
    'state.idle': '閒置',
    'state.ended': '已結束',
    'state.sip_connected': 'SIP 已連線',
    'state.call_recovered': '已恢復信令連線（通話中）',
  },
  en: {
    'app.title': 'CallFXO Console',
    'lang.label': 'Language',
    'login.title': 'Sign In',
    'login.username': 'Username',
    'login.password': 'Password',
    'login.button': 'Sign In',
    'login.prompt': 'Please enter username and password.',
    'login.success': 'Signed in.',
    'login.failed': 'Login failed: {error}',
    'logout': 'Logout',

    'users.title': 'User Management',
    'users.add': 'Add',
    'users.col.id': 'ID',
    'users.col.username': 'Username',
    'users.col.role': 'Role',
    'users.col.action': 'Action',
    'users.delete': 'Delete',
    'users.setpass': 'Set Password',
    'users.deleteConfirm': 'Delete user {username}?',
    'users.setPassPrompt': 'Enter new password for {username}',
    'users.setPassDone': 'Password updated for {username}',
    'users.ph.username': 'New username',
    'users.ph.password': 'New password',

    'role.user': 'User',
    'role.admin': 'Admin',

    'fxo.title': 'FXO Box Management',
    'fxo.add': 'Add',
    'fxo.col.name': 'Name',
    'fxo.col.username': 'Username',
    'fxo.col.status': 'Status',
    'fxo.col.action': 'Action',
    'fxo.edit': 'Edit',
    'fxo.delete': 'Delete',
    'fxo.deleteConfirm': 'Delete FXO {name}?',
    'fxo.tip': 'Tip: Configure your FXO box to REGISTER to this server with these SIP credentials.',
    'fxo.ph.name': 'Display name',
    'fxo.ph.user': 'SIP username',
    'fxo.ph.pass': 'SIP password',
    'fxo.prompt.name': 'Name',
    'fxo.prompt.user': 'SIP username',
    'fxo.prompt.pass': 'SIP password (leave empty to keep unchanged)',
    'fxo.status.online': 'Online',
    'fxo.status.offline': 'Offline',

    'dial.title': 'Dial',
    'dial.boxLabel': 'Select FXO box',
    'dial.numberLabel': 'Phone number',
    'dial.ph.number': 'e.g. 0912345678',
    'dial.button': 'Dial',
    'dial.hangup': 'Hang up',
    'dial.idle': 'Idle',
    'dial.sentHangup': 'Hangup sent',

    'pwd.title': 'Change Password',
    'pwd.ph.old': 'Old password',
    'pwd.ph.new': 'New password',
    'pwd.button': 'Change',
    'pwd.changed': 'Password updated',
    'pwd.needOldNew': 'Please enter old and new passwords',

    'logs.title': 'Call Logs',
    'logs.col.time': 'Time',
    'logs.col.box': 'Box',
    'logs.col.number': 'Number',
    'logs.col.status': 'Status',
    'logs.col.reason': 'Reason',
    'logs.empty': 'No call logs',
    'logs.prev': 'Prev',
    'logs.next': 'Next',
    'logs.page': 'Page {page} / {total}',

    'contacts.title': 'Contacts',
    'contacts.add': 'Add',
    'contacts.col.name': 'Name',
    'contacts.col.number': 'Number',
    'contacts.col.action': 'Action',
    'contacts.delete': 'Delete',
    'contacts.deleteConfirm': 'Delete contact {name}?',
    'contacts.empty': 'No contacts',
    'contacts.ph.search': 'Search name or number',
    'contacts.ph.name': 'Name',
    'contacts.ph.number': 'Number',

    'audio.volume': 'Playback volume',

    'ws.disconnected': 'WebSocket: Disconnected',
    'ws.connected': 'WebSocket: Connected',
    'ws.closed': 'WebSocket: Closed',
    'ws.error': 'WebSocket: Error',

    'call.connected': 'SIP connected, call is active',
    'call.ended': 'Call ended: {reason}',
    'call.wsReconnect': 'Signaling disconnected, reconnecting (call should continue)...',

    'error.prefix': 'Error: {error}',
    'error.dialFailed': 'Dial failed: {error}',
    'error.remoteAudio': 'Remote audio error: {error}',

    'alert.needBoxNumber': 'Please select an FXO box and enter a number',
    'alert.wsNotConnected': 'WebSocket is not connected',
    'alert.callActive': 'A call is already active',
    'alert.noPCMU': 'This browser does not support PCMU. Transcoder is required.',

    'state.new': 'new',
    'state.connecting': 'connecting',
    'state.connected': 'connected',
    'state.disconnected': 'disconnected',
    'state.failed': 'failed',
    'state.closed': 'closed',
    'state.dialing': 'dialing',
    'state.idle': 'idle',
    'state.ended': 'ended',
    'state.sip_connected': 'SIP connected',
    'state.call_recovered': 'call recovered',
  },
};

const DEFAULT_LANG = 'en';

function normalizeLang(raw) {
  const v = String(raw || '').trim().toLowerCase();
  if (v.startsWith('zh')) return 'zh-TW';
  if (v.startsWith('en')) return 'en';
  return DEFAULT_LANG;
}

function resolveInitialLang() {
  try {
    const saved = localStorage.getItem('callfxo_lang');
    if (saved) return normalizeLang(saved);
  } catch (_) {}
  const fromList = Array.isArray(navigator.languages) ? navigator.languages.find((v) => String(v || '').trim() !== '') : '';
  const browserLang = fromList || navigator.language || navigator.userLanguage || '';
  return browserLang ? normalizeLang(browserLang) : DEFAULT_LANG;
}

const state = {
  me: null,
  boxes: [],
  callLogs: [],
  contacts: [],
  ws: null,
  pc: null,
  localStream: null,
  audioCtx: null,
  remoteSource: null,
  remoteGain: null,
  logsPollTimer: null,
  callLogsPage: 1,
  callLogsPageSize: 10,
  callLogsTotal: 0,
  callLogsTotalPages: 1,
  contactQuery: '',
  contactSearchTimer: null,
  playbackVolume: 1,
  active: false,
  lang: resolveInitialLang(),
};

const loginSection = document.getElementById('loginSection');
const appSection = document.getElementById('appSection');
const loginMsg = document.getElementById('loginMsg');
const callStatus = document.getElementById('callStatus');
const wsStatus = document.getElementById('wsStatus');
const volumeSlider = document.getElementById('volumeSlider');
const volumeValue = document.getElementById('volumeValue');
const callLogsTable = document.getElementById('callLogsTable');
const callLogsPrevBtn = document.getElementById('callLogsPrevBtn');
const callLogsNextBtn = document.getElementById('callLogsNextBtn');
const callLogsPageInfo = document.getElementById('callLogsPageInfo');
const contactsTable = document.getElementById('contactsTable');
const contactSearchInput = document.getElementById('contactSearch');
const contactNameInput = document.getElementById('contactName');
const contactNumberInput = document.getElementById('contactNumber');
const addContactBtn = document.getElementById('addContactBtn');
const dialNumberInput = document.getElementById('dialNumber');
const myOldPassInput = document.getElementById('myOldPass');
const myNewPassInput = document.getElementById('myNewPass');
const changeMyPassBtn = document.getElementById('changeMyPassBtn');
const userAdminPanel = document.getElementById('userAdminPanel');
const boxAdminPanel = document.getElementById('boxAdminPanel');
const rootContainer = document.querySelector('.container');
const langSwitch = document.getElementById('langSwitch');

function t(key, vars = {}) {
  const table = I18N[state.lang] || I18N[DEFAULT_LANG];
  const fallback = I18N[DEFAULT_LANG] || {};
  let text = table[key] || fallback[key] || key;
  Object.keys(vars).forEach((k) => {
    text = text.replaceAll(`{${k}}`, String(vars[k] ?? ''));
  });
  return text;
}

function setText(el, text) {
  if (el) el.textContent = text;
}

function translateSignalState(raw) {
  if (!raw) return '';
  return t(`state.${raw}`);
}

function translateCallStatus(raw) {
  const normalized = String(raw || '').trim().toLowerCase().replaceAll(' ', '_');
  if (!normalized) return '-';
  const key = `state.${normalized}`;
  const label = t(key);
  return label === key ? raw : label;
}

function escapeHTML(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function formatLogTime(value) {
  if (!value) return '-';
  const ts = new Date(value);
  if (Number.isNaN(ts.getTime())) return '-';
  const locale = state.lang === 'zh-TW' ? 'zh-TW' : 'en-US';
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(ts);
}

function setDialNumber(number) {
  const n = String(number || '').trim();
  if (!n || !dialNumberInput) return;
  dialNumberInput.value = n;
  dialNumberInput.focus();
}

function updateCallLogPaginationUI() {
  const totalPages = Math.max(1, Number(state.callLogsTotalPages) || 1);
  let page = Number(state.callLogsPage) || 1;
  if (page < 1) page = 1;
  if (page > totalPages) page = totalPages;
  state.callLogsPage = page;

  setText(callLogsPageInfo, t('logs.page', { page, total: totalPages }));
  if (callLogsPrevBtn) callLogsPrevBtn.disabled = page <= 1;
  if (callLogsNextBtn) callLogsNextBtn.disabled = page >= totalPages;
}

function renderCallLogs(items = state.callLogs) {
  if (!callLogsTable) return;
  const list = Array.isArray(items) ? items : [];
  state.callLogs = list;
  callLogsTable.innerHTML = '';

  if (!list.length) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td colspan="5" style="text-align:center;color:#777;">${escapeHTML(t('logs.empty'))}</td>`;
    callLogsTable.appendChild(tr);
    updateCallLogPaginationUI();
    return;
  }

  list.forEach((item) => {
    const boxName = String(item.fxo_box_name || '').trim() || `#${item.fxo_box_id || '-'}`;
    const number = String(item.number || '').trim() || '-';
    const status = translateCallStatus(item.status);
    const reason = String(item.reason || '').trim() || '-';
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${escapeHTML(formatLogTime(item.started_at))}</td>
      <td>${escapeHTML(boxName)}</td>
      <td><button class="num-link" data-number="${escapeHTML(number)}">${escapeHTML(number)}</button></td>
      <td>${escapeHTML(status)}</td>
      <td>${escapeHTML(reason)}</td>
    `;
    callLogsTable.appendChild(tr);
  });

  callLogsTable.querySelectorAll('button[data-number]').forEach((btn) => {
    btn.onclick = () => setDialNumber(btn.dataset.number || '');
  });
  updateCallLogPaginationUI();
}

async function refreshCallLogs(page = state.callLogsPage) {
  const reqPage = Math.max(1, Number(page) || 1);
  const pageSize = Math.max(1, Math.min(100, Number(state.callLogsPageSize) || 10));
  const data = await api(`/api/calls?page=${reqPage}&page_size=${pageSize}`);
  state.callLogsPage = Math.max(1, Number(data.page) || reqPage);
  state.callLogsPageSize = Math.max(1, Math.min(100, Number(data.page_size) || pageSize));
  state.callLogsTotal = Math.max(0, Number(data.total) || 0);
  state.callLogsTotalPages = Math.max(1, Number(data.total_pages) || Math.ceil(state.callLogsTotal / state.callLogsPageSize) || 1);
  if (state.callLogsPage > state.callLogsTotalPages) {
    return refreshCallLogs(state.callLogsTotalPages);
  }
  renderCallLogs(data.items || []);
}

function startCallLogPolling() {
  if (state.logsPollTimer) return;
  state.logsPollTimer = setInterval(() => {
    if (!state.me) return;
    refreshCallLogs(state.callLogsPage).catch(() => {});
  }, 10000);
}

function stopCallLogPolling() {
  if (!state.logsPollTimer) return;
  clearInterval(state.logsPollTimer);
  state.logsPollTimer = null;
}

function renderContacts(items = state.contacts) {
  if (!contactsTable) return;
  const list = Array.isArray(items) ? items : [];
  state.contacts = list;
  contactsTable.innerHTML = '';

  if (!list.length) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<td colspan="3" style="text-align:center;color:#777;">${escapeHTML(t('contacts.empty'))}</td>`;
    contactsTable.appendChild(tr);
    return;
  }

  list.forEach((item) => {
    const name = String(item.name || '').trim() || '-';
    const number = String(item.number || '').trim() || '-';
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${escapeHTML(name)}</td>
      <td><button class="num-link" data-number="${escapeHTML(number)}">${escapeHTML(number)}</button></td>
      <td><button class="warn" data-id="${item.id}">${escapeHTML(t('contacts.delete'))}</button></td>
    `;
    contactsTable.appendChild(tr);
  });

  contactsTable.querySelectorAll('button[data-number]').forEach((btn) => {
    btn.onclick = () => setDialNumber(btn.dataset.number || '');
  });
  contactsTable.querySelectorAll('button[data-id]').forEach((btn) => {
    btn.onclick = async () => {
      const id = Number(btn.dataset.id || '0');
      if (!id) return;
      const item = state.contacts.find((v) => Number(v.id) === id);
      const name = item ? item.name : String(id);
      if (!confirm(t('contacts.deleteConfirm', { name }))) return;
      try {
        await api(`/api/contacts/${id}`, { method: 'DELETE' });
        await refreshContacts(state.contactQuery);
      } catch (e) {
        alert(e.message || e);
      }
    };
  });
}

async function refreshContacts(query = state.contactQuery) {
  const q = String(query || '').trim();
  state.contactQuery = q;
  const url = q ? `/api/contacts?q=${encodeURIComponent(q)}&limit=500` : '/api/contacts?limit=500';
  const data = await api(url);
  renderContacts(data.items || []);
}

function applyI18nToDOM() {
  document.documentElement.lang = state.lang === 'zh-TW' ? 'zh-Hant' : 'en';
  document.title = t('app.title');
  if (langSwitch) langSwitch.value = state.lang;

  document.querySelectorAll('[data-i18n]').forEach((el) => {
    const key = el.getAttribute('data-i18n');
    if (key) el.textContent = t(key);
  });

  document.querySelectorAll('[data-i18n-ph]').forEach((el) => {
    const key = el.getAttribute('data-i18n-ph');
    if (key) el.setAttribute('placeholder', t(key));
  });
  if (contactSearchInput && contactSearchInput.value !== state.contactQuery) {
    contactSearchInput.value = state.contactQuery;
  }

  if (!state.active) {
    setText(callStatus, t('dial.idle'));
  }

  if (!state.me) {
    setText(loginMsg, t('login.prompt'));
  }

  if (!state.ws || state.ws.readyState > 1) {
    setText(wsStatus, t('ws.disconnected'));
  } else if (state.ws.readyState === 1) {
    setText(wsStatus, t('ws.connected'));
  }

  applyPlaybackVolume();
  renderCallLogs();
  renderContacts();
  updateCallLogPaginationUI();
}

function setLanguage(lang, rerender = true) {
  state.lang = normalizeLang(lang);
  try {
    localStorage.setItem('callfxo_lang', state.lang);
  } catch (_) {}
  applyI18nToDOM();

  if (rerender && state.me) {
    refreshBoxes().catch(() => {});
    if (isAdmin()) refreshUsers().catch(() => {});
    refreshCallLogs(state.callLogsPage).catch(() => {});
    refreshContacts(state.contactQuery).catch(() => {});
  }
}

async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `HTTP ${res.status}`);
  }
  return data;
}

function showApp(authenticated) {
  loginSection.classList.toggle('hide', authenticated);
  appSection.classList.toggle('hide', !authenticated);
  if (rootContainer) {
    rootContainer.classList.toggle('auth-mode', !authenticated);
  }
}

function isAdmin() {
  return (state.me && (state.me.role || '').toLowerCase() === 'admin');
}

function applyRoleUI() {
  const admin = isAdmin();
  if (userAdminPanel) userAdminPanel.classList.remove('hide');
  if (boxAdminPanel) boxAdminPanel.classList.toggle('hide', !admin);
  document.querySelectorAll('.admin-only').forEach((el) => {
    el.classList.toggle('hide', !admin);
  });
}

function roleLabel(role) {
  const r = String(role || 'user').toLowerCase();
  return r === 'admin' ? t('role.admin') : t('role.user');
}

async function checkMe() {
  try {
    const data = await api('/api/me');
    if (data.authenticated) {
      state.me = data.user;
      showApp(true);
      await initAuthed();
    } else {
      stopCallLogPolling();
      if (state.contactSearchTimer) clearTimeout(state.contactSearchTimer);
      state.contactSearchTimer = null;
      state.callLogsPage = 1;
      state.callLogsTotal = 0;
      state.callLogsTotalPages = 1;
      state.contactQuery = '';
      if (contactSearchInput) contactSearchInput.value = '';
      if (myOldPassInput) myOldPassInput.value = '';
      if (myNewPassInput) myNewPassInput.value = '';
      renderCallLogs([]);
      renderContacts([]);
      showApp(false);
      setText(loginMsg, t('login.prompt'));
    }
  } catch (_) {
    stopCallLogPolling();
    if (state.contactSearchTimer) clearTimeout(state.contactSearchTimer);
    state.contactSearchTimer = null;
    state.callLogsPage = 1;
    state.callLogsTotal = 0;
    state.callLogsTotalPages = 1;
    state.contactQuery = '';
    if (contactSearchInput) contactSearchInput.value = '';
    if (myOldPassInput) myOldPassInput.value = '';
    if (myNewPassInput) myNewPassInput.value = '';
    renderCallLogs([]);
    renderContacts([]);
    showApp(false);
    setText(loginMsg, t('login.prompt'));
  }
}

async function initAuthed() {
  applyRoleUI();
  state.callLogsPage = 1;
  state.callLogsTotal = 0;
  state.callLogsTotalPages = 1;
  if (isAdmin()) {
    await Promise.all([refreshUsers(), refreshBoxes(), refreshCallLogs(1), refreshContacts(state.contactQuery)]);
  } else {
    await Promise.all([refreshBoxes(), refreshCallLogs(1), refreshContacts(state.contactQuery)]);
  }
  startCallLogPolling();
  connectWS();
}

async function refreshUsers() {
  if (!isAdmin()) return;
  const data = await api('/api/users');
  const tbody = document.getElementById('usersTable');
  tbody.innerHTML = '';
  (data.items || []).forEach((u) => {
    const uid = Number(u.id) || 0;
    const uname = escapeHTML(u.username || '');
    const role = escapeHTML(roleLabel(u.role));
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${uid}</td>
      <td>${uname}</td>
      <td>${role}</td>
      <td>
        <button data-act="setpass" data-id="${uid}" style="max-width:120px;">${escapeHTML(t('users.setpass'))}</button>
        <button data-act="delete" data-id="${uid}" class="warn" style="max-width:100px;">${escapeHTML(t('users.delete'))}</button>
      </td>`;
    const setPassBtn = tr.querySelector('button[data-act="setpass"]');
    const delBtn = tr.querySelector('button[data-act="delete"]');
    if (setPassBtn) {
      setPassBtn.onclick = async () => {
        const newPassword = prompt(t('users.setPassPrompt', { username: u.username }), '');
        const next = String(newPassword ?? '').trim();
        if (!next) return;
        try {
          await api(`/api/users/${u.id}/password`, {
            method: 'PUT',
            body: JSON.stringify({ new_password: next }),
          });
          alert(t('users.setPassDone', { username: u.username }));
        } catch (e) {
          alert(e.message);
        }
      };
    }
    if (delBtn) {
      delBtn.onclick = async () => {
        if (!confirm(t('users.deleteConfirm', { username: u.username }))) return;
        try {
          await api(`/api/users/${u.id}`, { method: 'DELETE' });
          await refreshUsers();
        } catch (e) {
          alert(e.message);
        }
      };
    }
    tbody.appendChild(tr);
  });
}

function boxStatusHTML(box) {
  const cls = box.online ? 'online' : 'offline';
  const txt = box.online ? t('fxo.status.online') : t('fxo.status.offline');
  return `<span class="badge ${cls}">${txt}</span>`;
}

async function refreshBoxes() {
  const data = await api('/api/fxo');
  state.boxes = data.items || [];

  const tbody = document.getElementById('boxesTable');
  const dialBox = document.getElementById('dialBox');
  tbody.innerHTML = '';
  dialBox.innerHTML = '';

  const admin = isAdmin();
  state.boxes.forEach((b) => {
    const name = escapeHTML(b.name || '');
    const sipUser = escapeHTML(b.sip_username || '');
    const tr = document.createElement('tr');
    const actionHTML = admin
      ? `<button data-act="edit" style="max-width:70px;">${escapeHTML(t('fxo.edit'))}</button><button data-act="del" class="warn" style="max-width:70px;">${escapeHTML(t('fxo.delete'))}</button>`
      : '<span style="color:#888;">-</span>';
    tr.innerHTML = `
      <td>${name}</td>
      <td>${sipUser}</td>
      <td>${boxStatusHTML(b)}</td>
      <td>${actionHTML}</td>
    `;
    const [editBtn, delBtn] = tr.querySelectorAll('button');

    if (admin && editBtn && delBtn) {
      editBtn.onclick = async () => {
        const name = prompt(t('fxo.prompt.name'), b.name) ?? '';
        if (!name.trim()) return;
        const user = prompt(t('fxo.prompt.user'), b.sip_username) ?? '';
        if (!user.trim()) return;
        const pass = prompt(t('fxo.prompt.pass'), '');
        try {
          await api(`/api/fxo/${b.id}`, {
            method: 'PUT',
            body: JSON.stringify({ name, sip_username: user, sip_password: pass || '' }),
          });
          await refreshBoxes();
        } catch (e) {
          alert(e.message);
        }
      };

      delBtn.onclick = async () => {
        if (!confirm(t('fxo.deleteConfirm', { name: b.name }))) return;
        try {
          await api(`/api/fxo/${b.id}`, { method: 'DELETE' });
          await refreshBoxes();
        } catch (e) {
          alert(e.message);
        }
      };
    }

    tbody.appendChild(tr);

    const opt = document.createElement('option');
    opt.value = b.id;
    opt.textContent = `${b.name} (${b.sip_username}) ${b.online ? t('fxo.status.online') : t('fxo.status.offline')}`;
    dialBox.appendChild(opt);
  });
}

function connectWS() {
  if (state.ws && state.ws.readyState <= 1) return;
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const url = `${proto}://${location.host}/ws/signaling`;
  const ws = new WebSocket(url);
  state.ws = ws;

  ws.onopen = () => setText(wsStatus, t('ws.connected'));
  ws.onclose = () => {
    setText(wsStatus, t('ws.closed'));
    if (state.active) {
      setText(callStatus, t('call.wsReconnect'));
    }
    if (state.me) {
      setTimeout(connectWS, 1200);
    }
  };
  ws.onerror = () => setText(wsStatus, t('ws.error'));

  ws.onmessage = async (ev) => {
    const msg = JSON.parse(ev.data);
    switch (msg.type) {
      case 'answer':
        if (state.pc) {
          await state.pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp });
          setText(callStatus, t('call.connected'));
          state.active = true;
          refreshCallLogs(1).catch(() => {});
        }
        break;
      case 'candidate':
        if (state.pc && msg.candidate) {
          try { await state.pc.addIceCandidate(msg.candidate); } catch (_) {}
        }
        break;
      case 'state': {
        const st = translateSignalState(msg.state);
        setText(callStatus, `${st}${msg.detail ? ` (${msg.detail})` : ''}`);
        if (['idle', 'ended', 'failed', 'sip_connected'].includes(String(msg.state || ''))) {
          refreshCallLogs(1).catch(() => {});
        }
        break;
      }
      case 'hangup':
        setText(callStatus, t('call.ended', { reason: msg.reason || '' }));
        cleanupPeer(false);
        refreshCallLogs(1).catch(() => {});
        break;
      case 'error':
        setText(callStatus, t('error.prefix', { error: msg.error }));
        break;
      case 'box_status':
      case 'boxes_snapshot':
        refreshBoxes().catch(() => {});
        break;
      case 'pong':
        break;
      default:
        break;
    }
  };
}

async function ensureAudio() {
  if (state.localStream) return state.localStream;
  state.localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
  return state.localStream;
}

function ensureAudioContext() {
  if (!state.audioCtx) {
    const Ctx = window.AudioContext || window.webkitAudioContext;
    if (!Ctx) {
      throw new Error('AudioContext not supported');
    }
    state.audioCtx = new Ctx();
  }
  return state.audioCtx;
}

function applyPlaybackVolume() {
  const pct = Math.round(state.playbackVolume * 100);
  if (volumeValue) volumeValue.textContent = `${pct}%`;
  if (volumeSlider && Number(volumeSlider.value) !== pct) {
    volumeSlider.value = String(pct);
  }
  if (state.remoteGain) {
    state.remoteGain.gain.value = state.playbackVolume;
  }
}

async function attachRemoteAudio(stream) {
  if (!stream) return;
  teardownRemoteAudio();

  const audioCtx = ensureAudioContext();
  if (audioCtx.state === 'suspended') {
    await audioCtx.resume();
  }

  const source = audioCtx.createMediaStreamSource(stream);
  const gain = audioCtx.createGain();
  gain.gain.value = state.playbackVolume;

  source.connect(gain);
  gain.connect(audioCtx.destination);

  state.remoteSource = source;
  state.remoteGain = gain;
}

function teardownRemoteAudio() {
  if (state.remoteSource) {
    try { state.remoteSource.disconnect(); } catch (_) {}
    state.remoteSource = null;
  }
  if (state.remoteGain) {
    try { state.remoteGain.disconnect(); } catch (_) {}
    state.remoteGain = null;
  }
}

function stopLocalMedia() {
  if (!state.localStream) return;
  state.localStream.getTracks().forEach((t) => {
    try { t.stop(); } catch (_) {}
  });
  state.localStream = null;
}

function preferPCMU(transceiver) {
  const caps = RTCRtpSender.getCapabilities('audio');
  if (!caps || !caps.codecs) return;
  const pcmu = caps.codecs.filter((c) => c.mimeType.toLowerCase() === 'audio/pcmu');
  if (!pcmu.length) return;
  const rest = caps.codecs.filter((c) => c.mimeType.toLowerCase() !== 'audio/pcmu');
  transceiver.setCodecPreferences([...pcmu, ...rest]);
}

function supportsPCMU() {
  const caps = RTCRtpSender.getCapabilities('audio');
  if (!caps || !caps.codecs) return false;
  return caps.codecs.some((c) => (c.mimeType || '').toLowerCase() === 'audio/pcmu');
}

async function startDial() {
  const boxId = Number(document.getElementById('dialBox').value || '0');
  const number = document.getElementById('dialNumber').value.trim();
  if (!boxId || !number) {
    alert(t('alert.needBoxNumber'));
    return;
  }
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
    alert(t('alert.wsNotConnected'));
    return;
  }
  if (state.pc) {
    alert(t('alert.callActive'));
    return;
  }
  if (!supportsPCMU()) {
    alert(t('alert.noPCMU'));
    return;
  }

  const audioCtx = ensureAudioContext();
  if (audioCtx.state === 'suspended') {
    await audioCtx.resume();
  }

  const pc = new RTCPeerConnection();
  state.pc = pc;

  pc.onicecandidate = (e) => {
    if (e.candidate && state.ws && state.ws.readyState === WebSocket.OPEN) {
      state.ws.send(JSON.stringify({ type: 'candidate', candidate: e.candidate.toJSON() }));
    }
  };
  pc.onconnectionstatechange = () => {
    if (['failed', 'closed', 'disconnected'].includes(pc.connectionState)) {
      cleanupPeer(false);
    }
  };
  pc.ontrack = (e) => {
    const stream = (e.streams && e.streams[0]) ? e.streams[0] : new MediaStream([e.track]);
    attachRemoteAudio(stream).catch((err) => {
      setText(callStatus, t('error.remoteAudio', { error: err.message || err }));
    });
  };

  const tx = pc.addTransceiver('audio', { direction: 'sendrecv' });
  preferPCMU(tx);

  const stream = await ensureAudio();
  stream.getAudioTracks().forEach((track) => pc.addTrack(track, stream));

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);

  setText(callStatus, translateSignalState('dialing'));
  state.ws.send(JSON.stringify({
    type: 'dial',
    box_id: boxId,
    number,
    sdp: pc.localDescription.sdp,
  }));
}

function cleanupPeer(sendHangup) {
  if (sendHangup && state.ws && state.ws.readyState === WebSocket.OPEN) {
    state.ws.send(JSON.stringify({ type: 'hangup' }));
  }
  state.active = false;
  teardownRemoteAudio();
  if (state.pc) {
    try { state.pc.close(); } catch (_) {}
    state.pc = null;
  }
  stopLocalMedia();
  if (state.audioCtx && state.audioCtx.state === 'running') {
    state.audioCtx.suspend().catch(() => {});
  }
}

function bindEvents() {
  document.getElementById('loginBtn').onclick = async () => {
    const username = document.getElementById('loginUser').value.trim();
    const password = document.getElementById('loginPass').value;
    try {
      const res = await api('/api/login', { method: 'POST', body: JSON.stringify({ username, password }) });
      setText(loginMsg, t('login.success'));
      state.me = res.user || { username, role: 'user' };
      showApp(true);
      await initAuthed();
    } catch (e) {
      setText(loginMsg, t('login.failed', { error: e.message }));
    }
  };

  document.getElementById('logoutBtn').onclick = async () => {
    cleanupPeer(true);
    if (state.ws) {
      try { state.ws.close(); } catch (_) {}
      state.ws = null;
    }
    stopCallLogPolling();
    if (state.contactSearchTimer) clearTimeout(state.contactSearchTimer);
    state.contactSearchTimer = null;
    state.me = null;
    state.callLogsPage = 1;
    state.callLogsTotal = 0;
    state.callLogsTotalPages = 1;
    state.contactQuery = '';
    if (contactSearchInput) contactSearchInput.value = '';
    if (myOldPassInput) myOldPassInput.value = '';
    if (myNewPassInput) myNewPassInput.value = '';
    renderCallLogs([]);
    renderContacts([]);
    await api('/api/logout', { method: 'POST' }).catch(() => {});
    showApp(false);
    applyRoleUI();
    setText(loginMsg, t('login.prompt'));
  };

  if (changeMyPassBtn) {
    changeMyPassBtn.onclick = async () => {
      const oldPassword = String(myOldPassInput?.value || '');
      const newPassword = String(myNewPassInput?.value || '');
      if (!oldPassword.trim() || !newPassword.trim()) {
        alert(t('pwd.needOldNew'));
        return;
      }
      try {
        const res = await api('/api/password', {
          method: 'PUT',
          body: JSON.stringify({
            old_password: oldPassword,
            new_password: newPassword,
          }),
        });
        if (myOldPassInput) myOldPassInput.value = '';
        if (myNewPassInput) myNewPassInput.value = '';
        alert(t('pwd.changed'));
        if (res && res.relogin) {
          state.me = null;
          showApp(false);
          applyRoleUI();
          setText(loginMsg, t('login.prompt'));
        }
      } catch (e) {
        alert(e.message || e);
      }
    };
  }

  document.getElementById('addUserBtn').onclick = async () => {
    const username = document.getElementById('newUserName').value.trim();
    const password = document.getElementById('newUserPass').value;
    const role = document.getElementById('newUserRole').value || 'user';
    if (!username || !password) return;
    try {
      await api('/api/users', { method: 'POST', body: JSON.stringify({ username, password, role }) });
      document.getElementById('newUserName').value = '';
      document.getElementById('newUserPass').value = '';
      document.getElementById('newUserRole').value = 'user';
      await refreshUsers();
    } catch (e) {
      alert(e.message);
    }
  };

  document.getElementById('addBoxBtn').onclick = async () => {
    const name = document.getElementById('boxName').value.trim();
    const sipUsername = document.getElementById('boxUser').value.trim();
    const sipPassword = document.getElementById('boxPass').value;
    if (!name || !sipUsername || !sipPassword) return;
    try {
      await api('/api/fxo', {
        method: 'POST',
        body: JSON.stringify({ name, sip_username: sipUsername, sip_password: sipPassword }),
      });
      document.getElementById('boxName').value = '';
      document.getElementById('boxUser').value = '';
      document.getElementById('boxPass').value = '';
      await refreshBoxes();
    } catch (e) {
      alert(e.message);
    }
  };

  document.getElementById('dialBtn').onclick = async () => {
    try {
      await startDial();
      refreshCallLogs(1).catch(() => {});
    } catch (e) {
      setText(callStatus, t('error.dialFailed', { error: e.message }));
      cleanupPeer(false);
    }
  };

  document.getElementById('hangupBtn').onclick = () => {
    cleanupPeer(true);
    setText(callStatus, t('dial.sentHangup'));
    refreshCallLogs(1).catch(() => {});
  };

  if (callLogsPrevBtn) {
    callLogsPrevBtn.onclick = () => {
      if (state.callLogsPage <= 1) return;
      refreshCallLogs(state.callLogsPage - 1).catch(() => {});
    };
  }
  if (callLogsNextBtn) {
    callLogsNextBtn.onclick = () => {
      if (state.callLogsPage >= state.callLogsTotalPages) return;
      refreshCallLogs(state.callLogsPage + 1).catch(() => {});
    };
  }

  if (addContactBtn) {
    addContactBtn.onclick = async () => {
      const name = String(contactNameInput?.value || '').trim();
      const number = String(contactNumberInput?.value || '').trim();
      if (!name || !number) return;
      try {
        await api('/api/contacts', {
          method: 'POST',
          body: JSON.stringify({ name, number }),
        });
        if (contactNameInput) contactNameInput.value = '';
        if (contactNumberInput) contactNumberInput.value = '';
        await refreshContacts(state.contactQuery);
      } catch (e) {
        alert(e.message || e);
      }
    };
  }

  if (contactSearchInput) {
    contactSearchInput.oninput = () => {
      const q = String(contactSearchInput.value || '');
      if (state.contactSearchTimer) clearTimeout(state.contactSearchTimer);
      state.contactSearchTimer = setTimeout(() => {
        state.contactQuery = q.trim();
        refreshContacts(state.contactQuery).catch(() => {});
      }, 220);
    };
  }

  document.querySelectorAll('#kpad button').forEach((btn) => {
    btn.onclick = () => {
      const d = btn.dataset.dtmf;
      if (!d || !state.ws || state.ws.readyState !== WebSocket.OPEN) return;
      state.ws.send(JSON.stringify({ type: 'dtmf', digits: d }));
    };
  });

  if (volumeSlider) {
    volumeSlider.oninput = () => {
      const n = Number(volumeSlider.value);
      const v = Number.isFinite(n) ? Math.max(0, Math.min(100, n)) / 100 : 1;
      state.playbackVolume = v;
      applyPlaybackVolume();
    };
  }

  if (langSwitch) {
    langSwitch.onchange = () => {
      setLanguage(langSwitch.value, true);
    };
  }
}

setLanguage(state.lang, false);
bindEvents();
checkMe();
