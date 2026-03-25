package li.power.app.callfxo.android.call

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioDeviceInfo
import android.media.AudioManager
import android.media.MediaPlayer
import android.media.RingtoneManager
import android.net.Uri
import android.os.Build
import android.telecom.DisconnectCause
import android.util.Log
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import li.power.app.callfxo.android.data.ApiClient
import li.power.app.callfxo.android.data.IncomingCallDto
import li.power.app.callfxo.android.data.SessionStore
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import org.webrtc.AudioSource
import org.webrtc.AudioTrack
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.MediaStreamTrack
import org.webrtc.PeerConnection
import org.webrtc.PeerConnectionFactory
import org.webrtc.RtpCapabilities
import org.webrtc.RtpReceiver
import org.webrtc.RtpTransceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription

enum class AudioOutput {
  EARPIECE,
  SPEAKER,
  BLUETOOTH,
}

class CallController(
  private val context: Context,
  private val sessionStore: SessionStore,
  private val apiClient: ApiClient,
  private val notificationManager: CallNotificationManager,
) {
  private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
  private val audioManager = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
  private val refreshMutex = Mutex()

  private val wsClient = OkHttpClient.Builder()
    .pingInterval(25, TimeUnit.SECONDS)
    .build()

  private var webSocket: WebSocket? = null
  private var reconnectJob: Job? = null
  private var iceDisconnectJob: Job? = null
  private var ringtonePlayer: MediaPlayer? = null

  private var appForeground = false
  private var callActive = false
  private var preferredAudioOutput = AudioOutput.EARPIECE

  private var factory: PeerConnectionFactory? = null
  private var peerConnection: PeerConnection? = null
  private var audioSource: AudioSource? = null
  private var audioTrack: AudioTrack? = null
  private var endingCall = false
  private var outgoingSignalStarted = false
  private var audioConfigured = false
  private var prevAudioMode = AudioManager.MODE_NORMAL
  private var prevSpeaker = true
  private var prevMicMute = false
  private var prevBluetoothSco = false

  private val _wsConnected = MutableStateFlow(false)
  val wsConnected: StateFlow<Boolean> = _wsConnected.asStateFlow()

  private val _callStatus = MutableStateFlow("idle")
  val callStatus: StateFlow<String> = _callStatus.asStateFlow()

  private val _inCall = MutableStateFlow(false)
  val inCall: StateFlow<Boolean> = _inCall.asStateFlow()

  private val _callBusy = MutableStateFlow(false)
  val callBusy: StateFlow<Boolean> = _callBusy.asStateFlow()

  private val _audioOutput = MutableStateFlow(AudioOutput.EARPIECE)
  val audioOutput: StateFlow<AudioOutput> = _audioOutput.asStateFlow()

  private val _bluetoothAvailable = MutableStateFlow(false)
  val bluetoothAvailable: StateFlow<Boolean> = _bluetoothAvailable.asStateFlow()

  private val _events = MutableSharedFlow<String>(extraBufferCapacity = 32)
  val events: SharedFlow<String> = _events.asSharedFlow()

  private val _pendingIncoming = MutableStateFlow<List<PendingIncomingCall>>(emptyList())
  val pendingIncoming: StateFlow<List<PendingIncomingCall>> = _pendingIncoming.asStateFlow()

  private val _currentIncoming = MutableStateFlow<PendingIncomingCall?>(null)
  val currentIncoming: StateFlow<PendingIncomingCall?> = _currentIncoming.asStateFlow()

  private val _activeCallInfo = MutableStateFlow(ActiveCallInfo())
  val activeCallInfo: StateFlow<ActiveCallInfo> = _activeCallInfo.asStateFlow()

  private val _muted = MutableStateFlow(false)
  val muted: StateFlow<Boolean> = _muted.asStateFlow()

  private val _uiMessages = MutableSharedFlow<String>(extraBufferCapacity = 16)
  val uiMessages: SharedFlow<String> = _uiMessages.asSharedFlow()

  init {
    notificationManager.ensureChannels()
    CallTelecomManager.register(context)
    refreshBluetoothAvailability()
    evaluateConnectionPolicy()
  }

  fun onSessionChanged() {
    evaluateConnectionPolicy()
    scope.launch {
      syncIncomingSnapshot()
    }
  }

  fun clearSession() {
    stopRinging()
    notificationManager.cancelIncomingCall()
    CallTelecomManager.clearDisconnected()
    callActive = false
    outgoingSignalStarted = false
    _pendingIncoming.value = emptyList()
    _currentIncoming.value = null
    _inCall.value = false
    _callBusy.value = false
    _callStatus.value = "idle"
    _activeCallInfo.value = ActiveCallInfo()
    _muted.value = false
    closePeer()
    configureAudioForCall(false)
    refreshBluetoothAvailability()
    disconnectSocket()
  }

  fun setForeground(foreground: Boolean) {
    appForeground = foreground
    refreshBluetoothAvailability()
    evaluateConnectionPolicy()
  }

  fun shutdown() {
    clearSession()
    try {
      factory?.dispose()
    } catch (_: Exception) {
    }
    factory = null
    scope.coroutineContext.cancel()
  }

  suspend fun startOutgoing(boxId: Long, number: String, boxLabel: String) {
    val request = OutgoingCallRequest(
      boxId = boxId,
      boxName = boxLabel.trim(),
      number = number.trim(),
    )
    try {
      prepareOutgoingState(request)
      if (CallTelecomManager.startOutgoing(context, request)) {
        CallTelecomManager.markDialing()
        return
      }
      performOutgoingDial(request)
    } catch (e: Exception) {
      if (hasOngoingCall()) {
        endCall(
          reason = e.message ?: "dial failed",
          showEndedNotification = false,
          disconnectCause = DisconnectCause.ERROR,
        )
      }
      throw e
    }
  }

  suspend fun acceptIncoming(inviteId: String? = null) {
    val incoming = findIncoming(inviteId) ?: error("incoming call not found")
    val ws = ensureSocketReady() ?: error("websocket not connected")
    if (peerConnection != null) error("call already active")

    callActive = true
    _callBusy.value = true
    _inCall.value = false
    _callStatus.value = "connecting"
    _activeCallInfo.value = ActiveCallInfo(
      displayName = displayCaller(incoming),
      subtitle = incoming.boxName,
      direction = "incoming",
      connectedAtMs = 0L,
    )
    configureAudioForCall(true)
    setMuted(false)
    stopRinging()
    notificationManager.cancelIncomingCall()

    val offerSdp = try {
      createOfferSdp()
    } catch (e: Exception) {
      callActive = false
      _callBusy.value = false
      _activeCallInfo.value = ActiveCallInfo()
      configureAudioForCall(false)
      throw e
    }

    val payload = JSONObject()
      .put("type", "incoming_accept")
      .put("invite_id", incoming.inviteId)
      .put("sdp", offerSdp)
    ws.send(payload.toString())
  }

  fun rejectIncoming(inviteId: String? = null) {
    scope.launch {
      rejectIncomingInternal(inviteId)
    }
  }

  fun hangup() {
    webSocket?.send(JSONObject().put("type", "hangup").toString())
    endCall("hangup", showEndedNotification = false, disconnectCause = DisconnectCause.LOCAL)
  }

  fun sendDtmf(digits: String) {
    if (digits.isBlank()) return
    webSocket?.send(JSONObject().put("type", "dtmf").put("digits", digits).toString())
  }

  fun setMuted(muted: Boolean) {
    _muted.value = muted
    try {
      audioTrack?.setEnabled(!muted)
    } catch (_: Exception) {
    }
  }

  fun setAudioOutput(output: AudioOutput) {
    preferredAudioOutput = output
    if (audioConfigured) {
      applyAudioOutput(output)
    } else {
      _audioOutput.value = output
      refreshBluetoothAvailability()
    }
  }

  fun handlePushPayload(data: Map<String, String>) {
    when (data["event"]?.trim()) {
      "incoming_call" -> {
        val inviteId = data["invite_id"]?.trim().orEmpty()
        if (inviteId.isBlank()) return
        upsertIncoming(
          PendingIncomingCall(
            inviteId = inviteId,
            boxId = data["box_id"]?.toLongOrNull() ?: 0L,
            boxName = data["box_name"].orEmpty(),
            callerId = data["caller_id"].orEmpty(),
            remoteNumber = data["remote_number"].orEmpty(),
            state = "ringing",
          )
        )
        // syncIncomingSnapshot in the background shouldn't automatically wipe local Push events
        // especially if database hasn't updated or if permissions are delayed.
        // We skip aggressive syncing here.
      }
      "incoming_answered" -> {
        val inviteId = data["invite_id"]?.trim().orEmpty()
        val answeredBy = data["answered_by_device_id"]?.trim().orEmpty()
        if (inviteId.isNotBlank()) {
          removeIncoming(inviteId)
        }
        val currentDevice = sessionStore.getSession()?.deviceToken.orEmpty()
        if (answeredBy.isNotBlank() && answeredBy != currentDevice) {
          _uiMessages.tryEmit("Incoming call answered on another device")
        }
      }
      "incoming_stop" -> {
        val inviteId = data["invite_id"]?.trim().orEmpty()
        if (inviteId.isNotBlank()) {
          removeIncoming(inviteId)
        }
      }
    }
  }

  fun handleNotificationAction(action: String?, inviteId: String, onDone: () -> Unit) {
    scope.launch {
      try {
        when (action) {
          CallNotificationManager.ACTION_ACCEPT -> acceptIncoming(inviteId)
          CallNotificationManager.ACTION_REJECT -> rejectIncomingInternal(inviteId)
        }
      } catch (e: Exception) {
        _uiMessages.tryEmit(e.message ?: "notification action failed")
      } finally {
        onDone()
      }
    }
  }

  fun answerFromTelecom(inviteId: String) {
    scope.launch {
      runCatching { acceptIncoming(inviteId) }
    }
  }

  fun rejectFromTelecom(inviteId: String) {
    rejectIncoming(inviteId)
  }

  fun startOutgoingFromTelecom(request: OutgoingCallRequest) {
    scope.launch {
      runCatching {
        prepareOutgoingState(request)
        performOutgoingDial(request)
      }.onFailure { err ->
        endCall(err.message ?: "dial failed", disconnectCause = DisconnectCause.ERROR)
      }
    }
  }

  suspend fun syncIncomingSnapshot() {
    apiClient.listIncomingCalls()
      .onSuccess { items ->
        replaceIncoming(items.map(::toPendingIncoming))
      }
  }

  private suspend fun rejectIncomingInternal(inviteId: String? = null) {
    val incoming = findIncoming(inviteId) ?: return
    ensureSocketReady()?.send(
      JSONObject()
        .put("type", "incoming_reject")
        .put("invite_id", incoming.inviteId)
        .toString()
    )
    removeIncoming(incoming.inviteId)
  }

  private fun evaluateConnectionPolicy() {
    if (shouldConnect()) {
      connectSocket()
    } else {
      disconnectSocket()
    }
  }

  private fun shouldConnect(): Boolean {
    return sessionStore.getSession() != null && (appForeground || callActive || _pendingIncoming.value.isNotEmpty())
  }

  private fun connectSocket() {
    if (webSocket != null) return
    val session = sessionStore.getSession() ?: return

    val wsUrl = buildWebSocketUrl(session.serverAddr) ?: run {
      _callStatus.value = "invalid server address"
      return
    }

    val req = Request.Builder()
      .url(wsUrl)
      .header("Authorization", "Bearer ${session.accessToken}")
      .build()

    webSocket = wsClient.newWebSocket(req, object : WebSocketListener() {
      override fun onOpen(webSocket: WebSocket, response: Response) {
        reconnectJob?.cancel()
        reconnectJob = null
        _wsConnected.value = true
        _callStatus.value = if (callActive) _callStatus.value else "connected"
        scope.launch {
          syncIncomingSnapshot()
        }
      }

      override fun onMessage(webSocket: WebSocket, text: String) {
        runCatching { handleWsMessage(text) }
          .onFailure { _callStatus.value = "signal parse error" }
      }

      override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        this@CallController.webSocket = null
        _wsConnected.value = false
        if (callActive) _callStatus.value = "signaling disconnected, reconnecting"
        scheduleReconnect(afterRefresh = false)
      }

      override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        this@CallController.webSocket = null
        _wsConnected.value = false
        if (callActive) _callStatus.value = "signaling error, reconnecting"
        scheduleReconnect(afterRefresh = response?.code == 401)
      }
    })
  }

  private fun disconnectSocket() {
    reconnectJob?.cancel()
    reconnectJob = null
    webSocket?.close(1000, "normal")
    webSocket = null
    _wsConnected.value = false
  }

  private fun buildWebSocketUrl(serverAddr: String): String? {
    val raw = serverAddr.trim().trimEnd('/')
    if (raw.isBlank()) return null
    val normalized = when {
      raw.startsWith("https://", ignoreCase = true) -> raw
      raw.startsWith("http://", ignoreCase = true) -> raw
      raw.startsWith("wss://", ignoreCase = true) -> "https://${raw.substringAfter("://")}"
      raw.startsWith("ws://", ignoreCase = true) -> "http://${raw.substringAfter("://")}"
      else -> "http://$raw"
    }

    val base = normalized.toHttpUrlOrNull() ?: return null
    val pathPrefix = base.encodedPath.trimEnd('/')
    val wsPath = if (pathPrefix.isBlank() || pathPrefix == "/") "/ws/signaling" else "$pathPrefix/ws/signaling"
    val scheme = if (base.isHttps) "wss" else "ws"
    val host = if (base.host.contains(":")) "[${base.host}]" else base.host
    val isDefaultPort = (base.isHttps && base.port == 443) || (!base.isHttps && base.port == 80)
    val portPart = if (isDefaultPort) "" else ":${base.port}"

    return "$scheme://$host$portPart$wsPath"
  }

  private fun scheduleReconnect(afterRefresh: Boolean) {
    if (!shouldConnect()) return
    if (reconnectJob?.isActive == true) return
    reconnectJob = scope.launch {
      if (afterRefresh) {
        refreshSessionIfNeeded()
      }
      delay(1200)
      if (shouldConnect()) connectSocket()
    }
  }

  private suspend fun refreshSessionIfNeeded(): Boolean {
    val session = sessionStore.getSession() ?: return false
    return refreshMutex.withLock {
      val current = sessionStore.getSession() ?: return@withLock false
      if (current.accessToken != session.accessToken) {
        return@withLock true
      }
      apiClient.refresh().isSuccess
    }
  }

  private suspend fun ensureSocketReady(): WebSocket? {
    connectSocket()
    repeat(50) {
      val ws = webSocket
      if (ws != null && _wsConnected.value) return ws
      delay(100)
      if (webSocket == null && shouldConnect()) {
        connectSocket()
      }
    }
    return null
  }

  private fun handleWsMessage(text: String) {
    val msg = JSONObject(text)
    when (msg.optString("type")) {
      "answer" -> {
        val sdp = msg.optString("sdp")
        if (sdp.isNotBlank()) {
          scope.launch {
            try {
              val pc = peerConnection ?: return@launch
              pc.awaitSetRemote(SessionDescription(SessionDescription.Type.ANSWER, sdp))
              callActive = true
              outgoingSignalStarted = false
              _callBusy.value = true
              _inCall.value = true
              _callStatus.value = "call active"
              CallTelecomManager.markActive()
              if (_activeCallInfo.value.connectedAtMs == 0L) {
                _activeCallInfo.value = _activeCallInfo.value.copy(connectedAtMs = System.currentTimeMillis())
              }
              removeIncoming(msg.optString("invite_id"))
              _events.tryEmit("call_changed")
            } catch (_: Exception) {
              endCall("answer error", disconnectCause = DisconnectCause.ERROR)
            }
          }
        }
      }
      "candidate" -> {
        val c = msg.optJSONObject("candidate") ?: return
        val cand = IceCandidate(
          c.optString("sdpMid"),
          c.optInt("sdpMLineIndex"),
          c.optString("candidate"),
        )
        peerConnection?.addIceCandidate(cand)
      }
      "state" -> {
        val st = msg.optString("state")
        val detail = msg.optString("detail")
        _callStatus.value = if (detail.isNotBlank()) "$st ($detail)" else st
        _events.tryEmit("call_changed")
        when (st) {
          "call_recovered" -> {
            callActive = true
            outgoingSignalStarted = false
            _callBusy.value = true
            _inCall.value = true
            CallTelecomManager.markActive()
            if (_activeCallInfo.value.connectedAtMs == 0L) {
              _activeCallInfo.value = _activeCallInfo.value.copy(connectedAtMs = System.currentTimeMillis())
            }
            evaluateConnectionPolicy()
          }
          "idle", "failed", "ended" -> {
            if (hasOngoingCall()) {
              endCall(
                reason = if (detail.isNotBlank()) "$st: $detail" else st,
                showEndedNotification = st != "idle",
                disconnectCause = disconnectCauseFor(st, detail),
              )
            }
          }
        }
      }
      "hangup" -> {
        val reason = msg.optString("reason", "hangup")
        if (hasOngoingCall()) {
          endCall(reason, disconnectCause = disconnectCauseFor(reason))
        } else {
          _callStatus.value = reason
        }
        _events.tryEmit("call_changed")
      }
      "incoming_call" -> {
        upsertIncoming(
          PendingIncomingCall(
            inviteId = msg.optString("invite_id"),
            boxId = msg.optLong("box_id"),
            boxName = msg.optString("box_name"),
            callerId = msg.optString("caller_id"),
            remoteNumber = msg.optString("remote_number"),
            state = msg.optString("state", "ringing"),
          )
        )
      }
      "incoming_stop" -> {
        removeIncoming(msg.optString("invite_id"))
      }
      "incoming_answered" -> {
        val inviteId = msg.optString("invite_id")
        val answeredBy = msg.optString("answered_by_device_token")
        removeIncoming(inviteId)
        val currentDevice = sessionStore.getSession()?.deviceToken.orEmpty()
        if (answeredBy.isNotBlank() && answeredBy != currentDevice) {
          _uiMessages.tryEmit("Incoming call answered on another device")
        }
      }
      "box_status", "boxes_snapshot" -> {
        _events.tryEmit("boxes_changed")
      }
      "error" -> {
        val errorMessage = msg.optString("error").ifBlank { "signaling error" }
        if (hasOngoingCall()) {
          endCall("error: $errorMessage", disconnectCause = DisconnectCause.ERROR)
        } else {
          _callStatus.value = "error: $errorMessage"
        }
      }
    }
  }

  private fun upsertIncoming(incoming: PendingIncomingCall) {
    if (incoming.inviteId.isBlank()) return
    val mutable = _pendingIncoming.value.toMutableList()
    val idx = mutable.indexOfFirst { it.inviteId == incoming.inviteId }
    if (idx >= 0) {
      mutable[idx] = incoming.copy(receivedAtMs = mutable[idx].receivedAtMs)
    } else {
      mutable.add(0, incoming)
    }
    replaceIncoming(mutable)
  }

  private fun replaceIncoming(items: List<PendingIncomingCall>) {
    _pendingIncoming.value = items
      .filter { it.inviteId.isNotBlank() }
      .distinctBy { it.inviteId }
      .sortedByDescending { it.receivedAtMs }
    Log.i("CallController", "replaceIncoming: updated _pendingIncoming, size=${_pendingIncoming.value.size}")
    _currentIncoming.value = _pendingIncoming.value.firstOrNull()
    updateIncomingPresentation()
  }

  private fun removeIncoming(inviteId: String?) {
    Log.i("CallController", "removeIncoming($inviteId) called. current list size: ${_pendingIncoming.value.size}")
    val normalized = inviteId?.trim().orEmpty()
    if (normalized.isBlank()) {
      updateIncomingPresentation()
      return
    }
    _pendingIncoming.value = _pendingIncoming.value.filterNot { it.inviteId == normalized }
    Log.i("CallController", "removeIncoming($inviteId) finished. new list size: ${_pendingIncoming.value.size}")
    _currentIncoming.value = _pendingIncoming.value.firstOrNull()
    updateIncomingPresentation()
  }

  private fun updateIncomingPresentation() {
    Log.i("CallController", "updateIncomingPresentation: callActive=$callActive, _currentIncoming=${_currentIncoming.value?.inviteId}")
    if (callActive || _currentIncoming.value == null) {
      Log.i("CallController", "updateIncomingPresentation: stopping ringing and cancelling notification.")
      stopRinging()
      notificationManager.cancelIncomingCall()
      if (!callActive) {
        CallTelecomManager.clearDisconnected()
      }
      evaluateConnectionPolicy()
      return
    }
    Log.i("CallController", "updateIncomingPresentation: showing incoming call and starting ringing.")
    startRinging()
    notificationManager.showIncomingCall(_currentIncoming.value!!)
    CallTelecomManager.reportIncoming(context, _currentIncoming.value!!)
    evaluateConnectionPolicy()
  }

  private fun findIncoming(inviteId: String? = null): PendingIncomingCall? {
    val normalized = inviteId?.trim().orEmpty()
    return if (normalized.isBlank()) {
      _currentIncoming.value
    } else {
      _pendingIncoming.value.firstOrNull { it.inviteId == normalized }
    }
  }

  private fun startRinging() {
    if (ringtonePlayer?.isPlaying == true) return
    val uriString = sessionStore.getRingtoneUri()
    val uri = runCatching {
      Uri.parse(uriString)
    }.getOrElse {
      RingtoneManager.getDefaultUri(RingtoneManager.TYPE_RINGTONE)
    }
    if (uri == null) return
    stopRinging()
    ringtonePlayer = runCatching {
      MediaPlayer().apply {
        setAudioAttributes(
          AudioAttributes.Builder()
            .setUsage(AudioAttributes.USAGE_NOTIFICATION_RINGTONE)
            .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
            .build()
        )
        setDataSource(context, uri)
        isLooping = true
        val volume = sessionStore.getRingtoneVolume()
        setVolume(volume, volume)
        prepare()
        start()
      }
    }.getOrNull()
  }

  private fun stopRinging() {
    val player = ringtonePlayer ?: return
    ringtonePlayer = null
    runCatching {
      if (player.isPlaying) {
        player.stop()
      }
    }
    runCatching { player.release() }
  }

  private suspend fun createOfferSdp(): String {
    val pc = createPeerConnection()
    val offer = pc.awaitCreateOffer()
    pc.awaitSetLocal(offer)
    return offer.description
  }

  private fun createPeerConnection(): PeerConnection {
    if (factory == null) {
      PeerConnectionFactory.initialize(
        PeerConnectionFactory.InitializationOptions.builder(context.applicationContext).createInitializationOptions()
      )
      factory = PeerConnectionFactory.builder().createPeerConnectionFactory()
    }

    val current = peerConnection
    if (current != null) return current

    val rtcConfig = PeerConnection.RTCConfiguration(buildIceServers())
    rtcConfig.sdpSemantics = PeerConnection.SdpSemantics.UNIFIED_PLAN

    val pc = factory!!.createPeerConnection(rtcConfig, object : PeerConnection.Observer {
      override fun onSignalingChange(newState: PeerConnection.SignalingState?) = Unit

      override fun onIceConnectionChange(newState: PeerConnection.IceConnectionState?) {
        if (!callActive && peerConnection == null) return
        when (newState) {
          PeerConnection.IceConnectionState.CONNECTED,
          PeerConnection.IceConnectionState.COMPLETED -> {
            iceDisconnectJob?.cancel()
            iceDisconnectJob = null
          }
          PeerConnection.IceConnectionState.DISCONNECTED -> {
            iceDisconnectJob?.cancel()
            iceDisconnectJob = scope.launch {
              delay(8000)
              if (callActive) {
                endCall("webrtc disconnected", disconnectCause = DisconnectCause.ERROR)
              }
            }
          }
          PeerConnection.IceConnectionState.FAILED,
          PeerConnection.IceConnectionState.CLOSED -> {
            iceDisconnectJob?.cancel()
            iceDisconnectJob = null
            endCall("webrtc disconnected", disconnectCause = DisconnectCause.ERROR)
          }
          else -> Unit
        }
      }

      override fun onIceConnectionReceivingChange(receiving: Boolean) = Unit
      override fun onIceGatheringChange(newState: PeerConnection.IceGatheringState?) = Unit

      override fun onIceCandidate(candidate: IceCandidate?) {
        if (candidate == null) return
        val cand = JSONObject()
          .put("sdpMid", candidate.sdpMid)
          .put("sdpMLineIndex", candidate.sdpMLineIndex)
          .put("candidate", candidate.sdp)
        webSocket?.send(JSONObject().put("type", "candidate").put("candidate", cand).toString())
      }

      override fun onIceCandidatesRemoved(candidates: Array<out IceCandidate>?) = Unit
      override fun onAddStream(stream: MediaStream?) = Unit
      override fun onRemoveStream(stream: MediaStream?) = Unit
      override fun onDataChannel(dataChannel: DataChannel?) = Unit
      override fun onRenegotiationNeeded() = Unit

      override fun onAddTrack(receiver: RtpReceiver?, mediaStreams: Array<out MediaStream>?) {
        receiver?.track()?.setEnabled(true)
      }

      override fun onTrack(transceiver: RtpTransceiver?) {
        transceiver?.receiver?.track()?.setEnabled(true)
      }
    }) ?: error("create peer connection failed")

    pc.setAudioPlayout(true)
    pc.setAudioRecording(true)

    val constraints = MediaConstraints().apply {
      optional.add(MediaConstraints.KeyValuePair("googEchoCancellation", "true"))
      optional.add(MediaConstraints.KeyValuePair("googAutoGainControl", "true"))
      optional.add(MediaConstraints.KeyValuePair("googNoiseSuppression", "true"))
      optional.add(MediaConstraints.KeyValuePair("googHighpassFilter", "true"))
    }
    audioSource = factory!!.createAudioSource(constraints)
    audioTrack = factory!!.createAudioTrack("audio0", audioSource).apply {
      setEnabled(!_muted.value)
    }

    val txInit = RtpTransceiver.RtpTransceiverInit(RtpTransceiver.RtpTransceiverDirection.SEND_RECV)
    val audioTx = pc.addTransceiver(MediaStreamTrack.MediaType.MEDIA_TYPE_AUDIO, txInit)
      ?: error("create audio transceiver failed")
    val pcmu = collectPCMUCodecs()
    if (pcmu.isEmpty()) {
      error("PCMU codec unavailable on this device")
    }
    audioTx.setCodecPreferences(pcmu)
    val sender = audioTx.sender
    if (!sender.setTrack(audioTrack!!, false)) {
      error("attach local audio track failed")
    }
    sender.setStreams(listOf("audio_stream"))

    peerConnection = pc
    return pc
  }

  private fun collectPCMUCodecs(): List<RtpCapabilities.CodecCapability> {
    val caps = factory?.getRtpSenderCapabilities(MediaStreamTrack.MediaType.MEDIA_TYPE_AUDIO) ?: return emptyList()
    return caps.codecs
      .asSequence()
      .filterNotNull()
      .filter {
        val mime = it.mimeType?.trim().orEmpty().lowercase()
        val name = it.name?.trim().orEmpty().lowercase()
        mime == "audio/pcmu" || name == "pcmu"
      }
      .toList()
  }

  private fun buildIceServers(): List<PeerConnection.IceServer> {
    val cfg = sessionStore.getWebRTCConfig() ?: return emptyList()
    return cfg.iceServers
      .asSequence()
      .flatMap { server ->
        server.urls
          .asSequence()
          .map { url -> url.trim() }
          .filter { it.isNotBlank() }
          .map { url ->
            PeerConnection.IceServer.builder(url).apply {
              if (server.username.isNotBlank()) {
                setUsername(server.username)
              }
              if (server.credential.isNotBlank()) {
                setPassword(server.credential)
              }
            }.createIceServer()
          }
      }
      .toList()
  }

  private fun prepareOutgoingState(request: OutgoingCallRequest) {
    if (_callBusy.value && _activeCallInfo.value.direction != "outgoing") {
      error("call already active")
    }
    if (peerConnection != null && _activeCallInfo.value.direction != "outgoing") {
      error("call already active")
    }
    callActive = true
    _callBusy.value = true
    _inCall.value = false
    _callStatus.value = "dialing"
    _activeCallInfo.value = ActiveCallInfo(
      displayName = request.number,
      subtitle = request.boxName,
      direction = "outgoing",
      connectedAtMs = 0L,
    )
    configureAudioForCall(true)
    setMuted(false)
    evaluateConnectionPolicy()
  }

  private suspend fun performOutgoingDial(request: OutgoingCallRequest) {
    if (outgoingSignalStarted) return
    outgoingSignalStarted = true
    try {
      val ws = ensureSocketReady() ?: error("websocket not connected")
      val offerSdp = createOfferSdp()
      val payload = JSONObject()
        .put("type", "dial")
        .put("box_id", request.boxId)
        .put("number", request.number)
        .put("sdp", offerSdp)
      if (!ws.send(payload.toString())) {
        error("dial signaling failed")
      }
      CallTelecomManager.markDialing()
    } catch (e: Exception) {
      outgoingSignalStarted = false
      throw e
    }
  }

  private fun hasOngoingCall(): Boolean {
    return callActive || outgoingSignalStarted || _callBusy.value || _inCall.value || peerConnection != null
  }

  private fun disconnectCauseFor(reason: String, detail: String = ""): Int {
    val combined = listOf(reason, detail)
      .joinToString(" ")
      .trim()
      .lowercase()
    return when {
      combined.contains("busy") || combined.contains("486") -> DisconnectCause.BUSY
      combined.contains("reject") || combined.contains("decline") -> DisconnectCause.REJECTED
      combined.contains("timeout") || combined.contains("missed") || combined.contains("480") -> DisconnectCause.MISSED
      combined.contains("error") || combined.contains("fail") || combined.contains("invalid") -> DisconnectCause.ERROR
      combined.contains("remote") || combined.contains("bye") || combined.contains("hangup") || combined.contains("ended") ->
        DisconnectCause.REMOTE
      else -> DisconnectCause.LOCAL
    }
  }

  private fun endCall(
    reason: String,
    showEndedNotification: Boolean = true,
    disconnectCause: Int = DisconnectCause.LOCAL,
  ) {
    if (endingCall) {
      _callStatus.value = reason
      return
    }
    endingCall = true
    callActive = false
    outgoingSignalStarted = false
    _inCall.value = false
    _callBusy.value = false
    _callStatus.value = reason
    _activeCallInfo.value = ActiveCallInfo()
    _muted.value = false
    try {
      iceDisconnectJob?.cancel()
      iceDisconnectJob = null
      closePeer()
      configureAudioForCall(false)
      evaluateConnectionPolicy()
      updateIncomingPresentation()
      _events.tryEmit("call_changed")
      if (showEndedNotification && reason.isNotBlank()) {
        notificationManager.showCallEnded(reason)
      }
      CallTelecomManager.clearDisconnected(disconnectCause)
    } finally {
      endingCall = false
    }
  }

  private fun closePeer() {
    iceDisconnectJob?.cancel()
    iceDisconnectJob = null
    val pc = peerConnection
    peerConnection = null
    try {
      pc?.close()
    } catch (_: Exception) {
    }
    try {
      pc?.dispose()
    } catch (_: Exception) {
    }
    try {
      audioTrack?.dispose()
    } catch (_: Exception) {
    }
    audioTrack = null
    try {
      audioSource?.dispose()
    } catch (_: Exception) {
    }
    audioSource = null
  }

  private fun configureAudioForCall(enable: Boolean) {
    if (enable) {
      if (audioConfigured) return
      prevAudioMode = audioManager.mode
      prevSpeaker = audioManager.isSpeakerphoneOn
      prevMicMute = audioManager.isMicrophoneMute
      prevBluetoothSco = audioManager.isBluetoothScoOn
      if (!CallTelecomManager.hasActiveConnection()) {
        audioManager.mode = AudioManager.MODE_IN_COMMUNICATION
      }
      audioManager.isMicrophoneMute = false
      audioConfigured = true
      stopRinging()
      notificationManager.cancelIncomingCall()
      refreshBluetoothAvailability()
      applyAudioOutput(preferredAudioOutput)
      return
    }

    if (!audioConfigured) return
    if (!CallTelecomManager.hasActiveConnection()) {
      if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        audioManager.clearCommunicationDevice()
      } else {
        audioManager.stopBluetoothSco()
        audioManager.isBluetoothScoOn = false
      }
      audioManager.isSpeakerphoneOn = prevSpeaker
      audioManager.mode = prevAudioMode
    }
    audioManager.isMicrophoneMute = prevMicMute
    audioManager.isBluetoothScoOn = prevBluetoothSco
    audioConfigured = false
    _audioOutput.value = preferredAudioOutput
    refreshBluetoothAvailability()
  }

  private fun refreshBluetoothAvailability() {
    val available = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
      audioManager.availableCommunicationDevices.any { it.type in bluetoothTypes() }
    } else {
      audioManager.isBluetoothScoAvailableOffCall
    }
    _bluetoothAvailable.value = available
  }

  private fun applyAudioOutput(output: AudioOutput): Boolean {
    refreshBluetoothAvailability()

    if (CallTelecomManager.hasActiveConnection()) {
      if (CallTelecomManager.setAudioRoute(output)) {
        _audioOutput.value = output
        return true
      }
    }

    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
      val targetTypes = when (output) {
        AudioOutput.EARPIECE -> intArrayOf(AudioDeviceInfo.TYPE_BUILTIN_EARPIECE)
        AudioOutput.SPEAKER -> intArrayOf(AudioDeviceInfo.TYPE_BUILTIN_SPEAKER)
        AudioOutput.BLUETOOTH -> bluetoothTypes()
      }
      val target = audioManager.availableCommunicationDevices.firstOrNull { d ->
        targetTypes.any { it == d.type }
      }
      if (target != null && audioManager.setCommunicationDevice(target)) {
        _audioOutput.value = output
        return true
      }
      if (output != AudioOutput.SPEAKER) {
        val speaker = audioManager.availableCommunicationDevices.firstOrNull { it.type == AudioDeviceInfo.TYPE_BUILTIN_SPEAKER }
        if (speaker != null && audioManager.setCommunicationDevice(speaker)) {
          _audioOutput.value = AudioOutput.SPEAKER
          return false
        }
      }
      _audioOutput.value = output
      return false
    }

    when (output) {
      AudioOutput.EARPIECE -> {
        audioManager.stopBluetoothSco()
        audioManager.isBluetoothScoOn = false
        audioManager.isSpeakerphoneOn = false
      }
      AudioOutput.SPEAKER -> {
        audioManager.stopBluetoothSco()
        audioManager.isBluetoothScoOn = false
        audioManager.isSpeakerphoneOn = true
      }
      AudioOutput.BLUETOOTH -> {
        audioManager.startBluetoothSco()
        audioManager.isBluetoothScoOn = true
        audioManager.isSpeakerphoneOn = false
      }
    }
    _audioOutput.value = output
    return true
  }

  private fun bluetoothTypes(): IntArray {
    return intArrayOf(
      AudioDeviceInfo.TYPE_BLUETOOTH_SCO,
      AudioDeviceInfo.TYPE_BLE_HEADSET,
      AudioDeviceInfo.TYPE_BLE_SPEAKER,
    )
  }

  private fun toPendingIncoming(dto: IncomingCallDto): PendingIncomingCall {
    return PendingIncomingCall(
      inviteId = dto.id,
      boxId = dto.boxId,
      boxName = dto.boxName,
      callerId = dto.callerId,
      remoteNumber = dto.remoteNumber,
      state = dto.state,
    )
  }

  private fun displayCaller(call: PendingIncomingCall): String {
    return firstNonBlank(call.callerId, call.remoteNumber, "Unknown")
  }

  private fun firstNonBlank(vararg values: String): String {
    return values.firstOrNull { it.isNotBlank() }.orEmpty()
  }
}

private suspend fun PeerConnection.awaitCreateOffer(): SessionDescription = suspendCancellableCoroutine { cont ->
  createOffer(object : SdpObserver {
    override fun onCreateSuccess(desc: SessionDescription?) {
      if (desc != null) cont.resume(desc) else cont.resumeWithException(IllegalStateException("offer null"))
    }

    override fun onSetSuccess() = Unit

    override fun onCreateFailure(error: String?) {
      cont.resumeWithException(IllegalStateException(error ?: "create offer failed"))
    }

    override fun onSetFailure(error: String?) = Unit
  }, MediaConstraints().apply {
    mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveAudio", "true"))
  })
}

private suspend fun PeerConnection.awaitSetLocal(desc: SessionDescription): Unit = suspendCancellableCoroutine { cont ->
  setLocalDescription(object : SdpObserver {
    override fun onCreateSuccess(desc: SessionDescription?) = Unit

    override fun onSetSuccess() = cont.resume(Unit)

    override fun onCreateFailure(error: String?) = Unit

    override fun onSetFailure(error: String?) {
      cont.resumeWithException(IllegalStateException(error ?: "set local failed"))
    }
  }, desc)
}

private suspend fun PeerConnection.awaitSetRemote(desc: SessionDescription): Unit = suspendCancellableCoroutine { cont ->
  setRemoteDescription(object : SdpObserver {
    override fun onCreateSuccess(desc: SessionDescription?) = Unit

    override fun onSetSuccess() = cont.resume(Unit)

    override fun onCreateFailure(error: String?) = Unit

    override fun onSetFailure(error: String?) {
      cont.resumeWithException(IllegalStateException(error ?: "set remote failed"))
    }
  }, desc)
}
