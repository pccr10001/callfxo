package li.power.app.callfxo.android.call

import android.content.Context
import android.media.AudioDeviceInfo
import android.media.AudioManager
import android.os.Build
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
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

enum class AudioOutput {
  EARPIECE,
  SPEAKER,
  BLUETOOTH,
}

class CallController(private val context: Context) {
  private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
  private val audioManager = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager

  private val wsClient = OkHttpClient.Builder()
    .pingInterval(25, TimeUnit.SECONDS)
    .build()

  private var session: SessionAuth? = null
  private var webSocket: WebSocket? = null
  private var reconnectJob: Job? = null
  private var iceDisconnectJob: Job? = null

  private var appForeground = false
  private var callActive = false
  private var preferredAudioOutput = AudioOutput.EARPIECE

  private var factory: PeerConnectionFactory? = null
  private var peerConnection: PeerConnection? = null
  private var audioSource: AudioSource? = null
  private var audioTrack: AudioTrack? = null
  private var endingCall = false
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

  private val _events = MutableSharedFlow<String>(extraBufferCapacity = 16)
  val events: SharedFlow<String> = _events.asSharedFlow()

  init {
    refreshBluetoothAvailability()
  }

  fun setSession(serverAddr: String, cookieName: String, cookieValue: String) {
    session = SessionAuth(serverAddr, cookieName, cookieValue)
    evaluateConnectionPolicy()
  }

  fun clearSession() {
    session = null
    callActive = false
    _inCall.value = false
    _callBusy.value = false
    _callStatus.value = "idle"
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
    try { factory?.dispose() } catch (_: Exception) {}
    factory = null
    scope.coroutineContext.cancel()
  }

  suspend fun dial(boxId: Long, number: String) {
    val ws = ensureSocketReady() ?: error("websocket not connected")
    if (peerConnection != null) error("call already active")

    callActive = true
    _callBusy.value = true
    configureAudioForCall(true)
    val offerSdp = try {
      createOfferSdp()
    } catch (e: Exception) {
      callActive = false
      _callBusy.value = false
      configureAudioForCall(false)
      throw e
    }
    _callStatus.value = "dialing"

    val payload = JSONObject()
      .put("type", "dial")
      .put("box_id", boxId)
      .put("number", number)
      .put("sdp", offerSdp)
    ws.send(payload.toString())
  }

  fun hangup() {
    webSocket?.send(JSONObject().put("type", "hangup").toString())
    endCall("hangup")
  }

  fun sendDtmf(digits: String) {
    if (digits.isBlank()) return
    webSocket?.send(JSONObject().put("type", "dtmf").put("digits", digits).toString())
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

  private fun evaluateConnectionPolicy() {
    if (shouldConnect()) {
      connectSocket()
    } else {
      disconnectSocket()
    }
  }

  private fun shouldConnect(): Boolean = session != null && (appForeground || callActive)

  private fun connectSocket() {
    if (webSocket != null) return
    val s = session ?: return

    val wsUrl = buildWebSocketUrl(s.serverAddr) ?: run {
      _callStatus.value = "invalid server address"
      return
    }

    val req = Request.Builder()
      .url(wsUrl)
      .header("Cookie", "${s.cookieName}=${s.cookieValue}")
      .build()

    webSocket = wsClient.newWebSocket(req, object : WebSocketListener() {
      override fun onOpen(webSocket: WebSocket, response: Response) {
        reconnectJob?.cancel()
        reconnectJob = null
        _wsConnected.value = true
        _callStatus.value = if (callActive) _callStatus.value else "connected"
      }

      override fun onMessage(webSocket: WebSocket, text: String) {
        runCatching { handleWsMessage(text) }
          .onFailure { _callStatus.value = "signal parse error" }
      }

      override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        this@CallController.webSocket = null
        _wsConnected.value = false
        if (callActive) _callStatus.value = "signaling disconnected, reconnecting"
        if (shouldConnect()) scheduleReconnect()
      }

      override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        this@CallController.webSocket = null
        _wsConnected.value = false
        if (callActive) _callStatus.value = "signaling error, reconnecting"
        if (shouldConnect()) scheduleReconnect()
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

  private fun scheduleReconnect() {
    if (reconnectJob?.isActive == true) return
    reconnectJob = scope.launch {
      delay(1200)
      if (shouldConnect()) connectSocket()
    }
  }

  private suspend fun ensureSocketReady(): WebSocket? {
    connectSocket()
    repeat(40) {
      val ws = webSocket
      if (ws != null && _wsConnected.value) return ws
      delay(100)
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
              _callBusy.value = true
              _inCall.value = true
              _callStatus.value = "call active"
              _events.tryEmit("call_changed")
            } catch (e: Exception) {
              endCall("answer error")
            }
          }
        }
      }
      "candidate" -> {
        val c = msg.optJSONObject("candidate") ?: return
        val cand = IceCandidate(
          c.optString("sdpMid"),
          c.optInt("sdpMLineIndex"),
          c.optString("candidate")
        )
        peerConnection?.addIceCandidate(cand)
      }
      "state" -> {
        val st = msg.optString("state")
        val detail = msg.optString("detail")
        _callStatus.value = if (detail.isNotBlank()) "$st ($detail)" else st
        _events.tryEmit("call_changed")
        if (st == "call_recovered") {
          callActive = true
          _callBusy.value = true
          _inCall.value = true
          evaluateConnectionPolicy()
          _events.tryEmit("call_changed")
          return
        }
        if (st == "idle" || st == "failed" || st == "ended") {
          endCall(st)
        }
        if (st == "sip_connected") {
          _events.tryEmit("call_changed")
        }
      }
      "hangup" -> {
        endCall(msg.optString("reason", "hangup"))
        _events.tryEmit("call_changed")
      }
      "box_status", "boxes_snapshot" -> {
        _events.tryEmit("boxes_changed")
      }
      "error" -> {
        _callStatus.value = "error: ${msg.optString("error")}"
        if (!_inCall.value) {
          callActive = false
          _callBusy.value = false
          evaluateConnectionPolicy()
        }
      }
      else -> Unit
    }
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

    val rtcConfig = PeerConnection.RTCConfiguration(emptyList())
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
                endCall("webrtc disconnected")
              }
            }
          }
          PeerConnection.IceConnectionState.FAILED,
          PeerConnection.IceConnectionState.CLOSED -> {
            iceDisconnectJob?.cancel()
            iceDisconnectJob = null
            endCall("webrtc disconnected")
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
    audioTrack = factory!!.createAudioTrack("audio0", audioSource)

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

  private fun endCall(reason: String) {
    if (endingCall) {
      _callStatus.value = reason
      return
    }
    endingCall = true
    callActive = false
    _inCall.value = false
    _callBusy.value = false
    _callStatus.value = reason
    try {
      iceDisconnectJob?.cancel()
      iceDisconnectJob = null
      closePeer()
      configureAudioForCall(false)
      evaluateConnectionPolicy()
      _events.tryEmit("call_changed")
    } finally {
      endingCall = false
    }
  }

  private fun closePeer() {
    iceDisconnectJob?.cancel()
    iceDisconnectJob = null
    val pc = peerConnection
    peerConnection = null
    try { pc?.close() } catch (_: Exception) {}
    try { pc?.dispose() } catch (_: Exception) {}
    try { audioTrack?.dispose() } catch (_: Exception) {}
    audioTrack = null
    try { audioSource?.dispose() } catch (_: Exception) {}
    audioSource = null
  }

  private fun configureAudioForCall(enable: Boolean) {
    if (enable) {
      if (audioConfigured) return
      prevAudioMode = audioManager.mode
      prevSpeaker = audioManager.isSpeakerphoneOn
      prevMicMute = audioManager.isMicrophoneMute
      prevBluetoothSco = audioManager.isBluetoothScoOn
      audioManager.mode = AudioManager.MODE_IN_COMMUNICATION
      audioManager.isMicrophoneMute = false
      audioConfigured = true
      refreshBluetoothAvailability()
      applyAudioOutput(preferredAudioOutput)
      return
    }

    if (!audioConfigured) return
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
      audioManager.clearCommunicationDevice()
    } else {
      audioManager.stopBluetoothSco()
      audioManager.isBluetoothScoOn = false
    }
    audioManager.isMicrophoneMute = prevMicMute
    audioManager.isBluetoothScoOn = prevBluetoothSco
    audioManager.isSpeakerphoneOn = prevSpeaker
    audioManager.mode = prevAudioMode
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

  private data class SessionAuth(val serverAddr: String, val cookieName: String, val cookieValue: String)
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
