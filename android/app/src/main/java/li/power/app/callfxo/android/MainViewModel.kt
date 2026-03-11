package li.power.app.callfxo.android

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import li.power.app.callfxo.android.call.ActiveCallInfo
import li.power.app.callfxo.android.call.AudioOutput
import li.power.app.callfxo.android.call.CallGuardService
import li.power.app.callfxo.android.call.PendingIncomingCall
import li.power.app.callfxo.android.data.CallLogDto
import li.power.app.callfxo.android.data.ContactSource
import li.power.app.callfxo.android.data.DeviceContactsReader
import li.power.app.callfxo.android.data.FxoBoxDto
import li.power.app.callfxo.android.data.PhoneContact
import li.power.app.callfxo.android.push.PushRuntime

class MainViewModel(app: Application) : AndroidViewModel(app) {
  private val appServices = CallFxoApp.from(app)
  private val sessionStore = appServices.sessionStore
  private val api = appServices.apiClient
  private val deviceContactsReader = DeviceContactsReader()
  private val callController = appServices.callController

  private val _ui = MutableStateFlow(
    UiState(
      serverAddr = sessionStore.getServerAddr(),
      ringtoneUri = sessionStore.getRingtoneUri(),
      ringtoneVolume = sessionStore.getRingtoneVolume(),
    )
  )
  val ui: StateFlow<UiState> = _ui.asStateFlow()

  private var deviceContacts: List<PhoneContact> = emptyList()
  private var serverContacts: List<PhoneContact> = emptyList()
  private var searchJob: Job? = null

  init {
    observeCallController()
    restoreSession()
  }

  private fun observeCallController() {
    viewModelScope.launch {
      callController.wsConnected.collect { connected ->
        _ui.value = _ui.value.copy(wsConnected = connected)
      }
    }

    viewModelScope.launch {
      callController.callStatus.collect { status ->
        _ui.value = _ui.value.copy(callStatus = status)
      }
    }

    viewModelScope.launch {
      callController.inCall.collect { active ->
        _ui.value = _ui.value.copy(inCall = active)
        if (active) {
          CallGuardService.start(getApplication())
        } else if (!_ui.value.callBusy) {
          CallGuardService.stop(getApplication())
        }
      }
    }

    viewModelScope.launch {
      callController.callBusy.collect { busy ->
        _ui.value = _ui.value.copy(callBusy = busy)
        if (busy) {
          CallGuardService.start(getApplication())
        } else if (!_ui.value.inCall) {
          CallGuardService.stop(getApplication())
        }
      }
    }

    viewModelScope.launch {
      callController.audioOutput.collect { output ->
        _ui.value = _ui.value.copy(audioOutput = output)
      }
    }

    viewModelScope.launch {
      callController.bluetoothAvailable.collect { available ->
        _ui.value = _ui.value.copy(bluetoothAvailable = available)
      }
    }

    viewModelScope.launch {
      callController.activeCallInfo.collect { info ->
        _ui.value = _ui.value.copy(
          callDisplayName = info.displayName,
          callDisplaySubtitle = info.subtitle,
          callDirection = info.direction,
          callConnectedAtMs = info.connectedAtMs,
        )
      }
    }

    viewModelScope.launch {
      callController.currentIncoming.collect { incoming ->
        _ui.value = _ui.value.copy(incomingCall = incoming)
      }
    }

    viewModelScope.launch {
      callController.muted.collect { muted ->
        _ui.value = _ui.value.copy(muted = muted)
      }
    }

    viewModelScope.launch {
      callController.uiMessages.collect { message ->
        _ui.value = _ui.value.copy(noticeMessage = message)
      }
    }

    viewModelScope.launch {
      callController.events.collect { ev ->
        when (ev) {
          "boxes_changed" -> refreshBoxes()
          "call_changed" -> refreshCallLogs(1)
        }
      }
    }
  }

  private fun restoreSession() {
    val s = api.currentSession() ?: return
    _ui.value = _ui.value.copy(serverAddr = s.serverAddr, username = s.username, usernameInput = s.username)

    viewModelScope.launch {
      api.me().onSuccess { me ->
        if (me.authenticated && me.user != null) {
          callController.onSessionChanged()
          _ui.value = _ui.value.copy(
            loggedIn = true,
            username = me.user.username,
            role = me.user.role,
            loginError = null,
          )
          refreshAll()
          syncPushRegistration()
        } else {
          sessionStore.clearSession()
        }
      }.onFailure {
        sessionStore.clearSession()
      }
    }
  }

  fun onForegroundChanged(foreground: Boolean) {
    callController.setForeground(foreground)
  }

  fun onServerAddrChange(v: String) {
    _ui.value = _ui.value.copy(serverAddr = v)
    sessionStore.saveServerAddr(v)
  }

  fun onUsernameChange(v: String) {
    _ui.value = _ui.value.copy(usernameInput = v)
  }

  fun onPasswordChange(v: String) {
    _ui.value = _ui.value.copy(passwordInput = v)
  }

  fun onDialPadPress(d: String) {
    if (d.isBlank()) return
    if (_ui.value.inCall || _ui.value.callBusy) {
      callController.sendDtmf(d)
      return
    }
    _ui.value = _ui.value.copy(dialNumber = _ui.value.dialNumber + d)
  }

  fun onBackspaceDial() {
    val current = _ui.value.dialNumber
    if (current.isEmpty()) return
    _ui.value = _ui.value.copy(dialNumber = current.dropLast(1))
  }

  fun onSelectBox(boxId: Long) {
    _ui.value = _ui.value.copy(selectedBoxId = boxId)
  }

  fun login() {
    val st = _ui.value
    val server = st.serverAddr.trim()
    val username = st.usernameInput.trim()
    val password = st.passwordInput
    if (server.isBlank() || username.isBlank() || password.isBlank()) {
      _ui.value = st.copy(loginError = "server/username/password required")
      return
    }

    viewModelScope.launch {
      _ui.value = _ui.value.copy(loading = true, loginError = null)
      api.login(server, username, password)
        .onSuccess { user ->
          callController.onSessionChanged()
          _ui.value = _ui.value.copy(
            loggedIn = true,
            role = user.role,
            username = user.username,
            usernameInput = "",
            passwordInput = "",
            loading = false,
          )
          refreshAll()
          syncPushRegistration()
        }
        .onFailure { e ->
          _ui.value = _ui.value.copy(loading = false, loginError = e.message ?: "login failed")
        }
    }
  }

  fun logout() {
    viewModelScope.launch {
      searchJob?.cancel()
      api.logout()
      callController.clearSession()
      CallGuardService.stop(getApplication())
      _ui.value = UiState(
        serverAddr = sessionStore.getServerAddr(),
        ringtoneUri = sessionStore.getRingtoneUri(),
        ringtoneVolume = sessionStore.getRingtoneVolume(),
      )
      deviceContacts = emptyList()
      serverContacts = emptyList()
    }
  }

  fun shutdown() {
    callController.clearSession()
    CallGuardService.stop(getApplication())
  }

  fun refreshAll() {
    refreshBoxes()
    refreshContacts(_ui.value.search)
    refreshCallLogs(_ui.value.logPage)
    viewModelScope.launch {
      callController.syncIncomingSnapshot()
    }
  }

  fun refreshBoxes() {
    if (!_ui.value.loggedIn) return
    viewModelScope.launch {
      api.listFxo().onSuccess { boxes ->
        val selected = _ui.value.selectedBoxId
        val selectedSafe = if (selected != null && boxes.any { it.id == selected }) selected else boxes.firstOrNull()?.id
        _ui.value = _ui.value.copy(boxes = boxes, selectedBoxId = selectedSafe)
      }
    }
  }

  fun refreshCallLogs(page: Int) {
    if (!_ui.value.loggedIn) return
    viewModelScope.launch {
      api.listCallLogs(page = page, pageSize = 10).onSuccess { resp ->
        _ui.value = _ui.value.copy(
          callLogs = resp.items,
          logPage = resp.page,
          logTotalPages = if (resp.totalPages <= 0) 1 else resp.totalPages,
        )
      }
    }
  }

  fun prevLogPage() {
    val now = _ui.value.logPage
    if (now <= 1) return
    refreshCallLogs(now - 1)
  }

  fun nextLogPage() {
    val now = _ui.value.logPage
    if (now >= _ui.value.logTotalPages) return
    refreshCallLogs(now + 1)
  }

  fun onSearchChange(v: String) {
    _ui.value = _ui.value.copy(search = v)
    searchJob?.cancel()
    searchJob = viewModelScope.launch {
      delay(250)
      refreshContacts(v)
    }
  }

  fun refreshContacts(query: String) {
    if (!_ui.value.loggedIn) return
    viewModelScope.launch {
      api.listContacts(query).onSuccess { items ->
        serverContacts = items.map {
          PhoneContact(id = "s-${it.id}", name = it.name, number = it.number, source = ContactSource.SERVER)
        }
        mergeContacts(query)
      }
    }
  }

  fun loadDeviceContacts() {
    viewModelScope.launch {
      deviceContacts = deviceContactsReader.readAll(getApplication())
      mergeContacts(_ui.value.search)
    }
  }

  private fun mergeContacts(queryRaw: String) {
    val q = queryRaw.trim().lowercase()
    val merged = (serverContacts + deviceContacts)
      .filter {
        q.isBlank() || it.name.lowercase().contains(q) || it.number.lowercase().contains(q)
      }
      .distinctBy { "${it.name}|${it.number}|${it.source}" }
      .sortedWith(compareBy<PhoneContact>({ it.name.lowercase() }, { it.number }))

    _ui.value = _ui.value.copy(contacts = merged)
  }

  fun fillDialNumber(number: String) {
    _ui.value = _ui.value.copy(dialNumber = number)
  }

  fun dial() {
    val st = _ui.value
    val boxId = st.selectedBoxId
    val num = st.dialNumber.trim()
    if (st.callBusy) {
      _ui.value = st.copy(callStatus = "call already active")
      return
    }
    if (boxId == null || num.isBlank()) {
      _ui.value = st.copy(callStatus = "select box and number")
      return
    }
    val box = st.boxes.firstOrNull { it.id == boxId }
    if (box == null) {
      _ui.value = st.copy(callStatus = "fxo box not found")
      return
    }
    if (!box.online) {
      _ui.value = st.copy(callStatus = "fxo box is offline")
      return
    }
    if (box.inUse) {
      _ui.value = st.copy(callStatus = "fxo box is in use")
      return
    }

    viewModelScope.launch {
      runCatching { callController.startOutgoing(boxId, num, box.name) }
        .onFailure { e -> _ui.value = _ui.value.copy(callStatus = e.message ?: "dial failed") }
    }
  }

  fun acceptIncoming() {
    viewModelScope.launch {
      runCatching { callController.acceptIncoming(_ui.value.incomingCall?.inviteId) }
        .onFailure { e -> _ui.value = _ui.value.copy(callStatus = e.message ?: "accept failed") }
    }
  }

  fun rejectIncoming() {
    callController.rejectIncoming(_ui.value.incomingCall?.inviteId)
  }

  fun hangup() {
    callController.hangup()
  }

  fun toggleMute() {
    callController.setMuted(!_ui.value.muted)
  }

  fun setAudioOutput(output: AudioOutput) {
    callController.setAudioOutput(output)
  }

  fun clearNotice() {
    _ui.value = _ui.value.copy(noticeMessage = null)
  }

  fun setRingtoneVolume(volume: Float) {
    val normalized = volume.coerceIn(0f, 1f)
    sessionStore.saveRingtoneVolume(normalized)
    _ui.value = _ui.value.copy(ringtoneVolume = normalized)
  }

  fun setRingtoneUri(uri: String?) {
    sessionStore.saveRingtoneUri(uri)
    _ui.value = _ui.value.copy(ringtoneUri = sessionStore.getRingtoneUri())
  }

  private fun syncPushRegistration() {
    viewModelScope.launch {
      api.getPushConfig().onSuccess { cfg ->
        sessionStore.savePushConfig(cfg)
      }
      PushRuntime.syncToken(getApplication(), sessionStore, api).onFailure { }
    }
  }

  override fun onCleared() {
    super.onCleared()
    searchJob?.cancel()
  }
}

data class UiState(
  val loading: Boolean = false,
  val loggedIn: Boolean = false,
  val role: String = "",
  val serverAddr: String = "",
  val username: String = "",
  val usernameInput: String = "",
  val passwordInput: String = "",
  val loginError: String? = null,

  val boxes: List<FxoBoxDto> = emptyList(),
  val selectedBoxId: Long? = null,
  val dialNumber: String = "",
  val wsConnected: Boolean = false,
  val callStatus: String = "idle",
  val inCall: Boolean = false,
  val callBusy: Boolean = false,
  val audioOutput: AudioOutput = AudioOutput.EARPIECE,
  val bluetoothAvailable: Boolean = false,
  val muted: Boolean = false,

  val callDisplayName: String = "",
  val callDisplaySubtitle: String = "",
  val callDirection: String = "",
  val callConnectedAtMs: Long = 0L,
  val incomingCall: PendingIncomingCall? = null,
  val noticeMessage: String? = null,

  val ringtoneUri: String = "",
  val ringtoneVolume: Float = 0.85f,

  val search: String = "",
  val contacts: List<PhoneContact> = emptyList(),

  val callLogs: List<CallLogDto> = emptyList(),
  val logPage: Int = 1,
  val logTotalPages: Int = 1,
)
