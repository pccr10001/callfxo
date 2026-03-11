package li.power.app.callfxo.android.call

import android.Manifest
import android.content.ComponentName
import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import android.telecom.Connection
import android.telecom.DisconnectCause
import android.telecom.PhoneAccount
import android.telecom.PhoneAccountHandle
import android.telecom.TelecomManager
import android.util.Log
import androidx.core.content.ContextCompat
import li.power.app.callfxo.android.R

object CallTelecomManager {
  private const val ACCOUNT_ID = "callfxo_self_managed"
  private const val EXTRA_INVITE_ID = "callfxo_invite_id"
  private const val EXTRA_BOX_ID = "callfxo_box_id"
  private const val EXTRA_BOX_NAME = "callfxo_box_name"
  private const val EXTRA_NUMBER = "callfxo_number"

  private var lastReportedInviteId: String? = null
  private var pendingOutgoing: OutgoingCallRequest? = null
  private var currentConnection: CallFxoConnection? = null

  fun register(context: Context) {
    if (!canUseTelecom(context)) return
    val telecom = context.getSystemService(TelecomManager::class.java) ?: return
    val account = PhoneAccount.builder(phoneAccountHandle(context), context.getString(R.string.app_name))
      .setCapabilities(PhoneAccount.CAPABILITY_SELF_MANAGED)
      .setShortDescription(context.getString(R.string.app_name))
      .build()
    runCatching { telecom.registerPhoneAccount(account) }
  }

  fun startOutgoing(context: Context, request: OutgoingCallRequest): Boolean {
    if (!canUseTelecom(context)) return false
    val telecom = context.getSystemService(TelecomManager::class.java) ?: return false
    register(context)
    synchronized(this) {
      pendingOutgoing = request
    }
    val extras = Bundle().apply {
      putParcelable(TelecomManager.EXTRA_PHONE_ACCOUNT_HANDLE, phoneAccountHandle(context))
      putLong(EXTRA_BOX_ID, request.boxId)
      putString(EXTRA_BOX_NAME, request.boxName)
      putString(EXTRA_NUMBER, request.number)
      putParcelable(TelecomManager.EXTRA_OUTGOING_CALL_EXTRAS, Bundle().apply {
        putLong(EXTRA_BOX_ID, request.boxId)
        putString(EXTRA_BOX_NAME, request.boxName)
        putString(EXTRA_NUMBER, request.number)
      })
    }
    return runCatching {
      telecom.placeCall(Uri.fromParts("tel", request.number, null), extras)
      true
    }.getOrElse {
      synchronized(this) {
        pendingOutgoing = null
      }
      false
    }
  }

  fun consumeOutgoing(request: android.telecom.ConnectionRequest): OutgoingCallRequest? {
    val outgoingExtras = request.extras?.getBundle(TelecomManager.EXTRA_OUTGOING_CALL_EXTRAS)
    val number = firstNonBlank(
      outgoingExtras?.getString(EXTRA_NUMBER),
      request.extras?.getString(EXTRA_NUMBER),
      request.address?.schemeSpecificPart,
    )
    val boxId = outgoingExtras?.getLong(EXTRA_BOX_ID) ?: request.extras?.getLong(EXTRA_BOX_ID) ?: 0L
    val boxName = firstNonBlank(outgoingExtras?.getString(EXTRA_BOX_NAME), request.extras?.getString(EXTRA_BOX_NAME))
    val fromExtras = if (boxId > 0 && number.isNotBlank()) {
      OutgoingCallRequest(boxId = boxId, boxName = boxName, number = number)
    } else {
      null
    }
    synchronized(this) {
      val pending = pendingOutgoing
      pendingOutgoing = null
      return fromExtras ?: pending
    }
  }

  fun reportIncoming(context: Context, call: PendingIncomingCall) {
    if (!canUseTelecom(context) || call.inviteId.isBlank() || call.inviteId == lastReportedInviteId) return
    register(context)
    val telecom = context.getSystemService(TelecomManager::class.java) ?: return
    val extras = Bundle().apply {
      putString(EXTRA_INVITE_ID, call.inviteId)
      putString(EXTRA_BOX_NAME, call.boxName)
      putString(EXTRA_NUMBER, call.remoteNumber)
      putParcelable(
        TelecomManager.EXTRA_INCOMING_CALL_ADDRESS,
        Uri.fromParts("tel", (call.remoteNumber.ifBlank { call.callerId }).ifBlank { "unknown" }, null)
      )
      putString(TelecomManager.EXTRA_CALL_SUBJECT, call.boxName)
    }
    runCatching {
      telecom.addNewIncomingCall(phoneAccountHandle(context), extras)
      lastReportedInviteId = call.inviteId
    }
  }

  fun bindConnection(connection: CallFxoConnection) {
    currentConnection = connection
  }

  fun markDialing() {
    currentConnection?.setDialingState()
  }

  fun markActive() {
    currentConnection?.setActiveState()
  }

  fun clearDisconnected(cause: Int = DisconnectCause.LOCAL) {
    currentConnection?.setDisconnectedState(cause)
    currentConnection = null
    lastReportedInviteId = null
    synchronized(this) {
      pendingOutgoing = null
    }
  }

  fun inviteIdFromExtras(extras: Bundle?): String {
    return extras?.getString(EXTRA_INVITE_ID).orEmpty()
  }

  fun setAudioRoute(output: li.power.app.callfxo.android.call.AudioOutput): Boolean {
    val conn = currentConnection ?: return false
    when (output) {
      li.power.app.callfxo.android.call.AudioOutput.SPEAKER -> conn.setAudioRoute(android.telecom.CallAudioState.ROUTE_SPEAKER)
      li.power.app.callfxo.android.call.AudioOutput.EARPIECE -> conn.setAudioRoute(android.telecom.CallAudioState.ROUTE_EARPIECE)
      li.power.app.callfxo.android.call.AudioOutput.BLUETOOTH -> conn.setAudioRoute(android.telecom.CallAudioState.ROUTE_BLUETOOTH)
    }
    return true
  }

  fun hasActiveConnection(): Boolean {
    return currentConnection != null
  }

  private fun phoneAccountHandle(context: Context): PhoneAccountHandle {
    return PhoneAccountHandle(ComponentName(context, CallConnectionService::class.java), ACCOUNT_ID)
  }

  private fun canUseTelecom(context: Context): Boolean {
    return ContextCompat.checkSelfPermission(context, Manifest.permission.MANAGE_OWN_CALLS) == PackageManager.PERMISSION_GRANTED
  }

  private fun firstNonBlank(vararg values: String?): String {
    return values.firstOrNull { !it.isNullOrBlank() }?.trim().orEmpty()
  }
}

class CallFxoConnection(
  private val appContext: Context,
  private val direction: String,
  private val inviteId: String = "",
  private val displayName: String = "",
  private val number: String = "",
) : Connection() {
  init {
    connectionProperties = PROPERTY_SELF_MANAGED
    connectionCapabilities = CAPABILITY_MUTE
    setAudioModeIsVoip(true)
    if (number.isNotBlank()) {
      setAddress(Uri.fromParts("tel", number, null), TelecomManager.PRESENTATION_ALLOWED)
    }
    if (displayName.isNotBlank()) {
      setCallerDisplayName(displayName, TelecomManager.PRESENTATION_ALLOWED)
    }
    setInitializing()
    setInitialized()
    if (direction == "incoming") {
      setRinging()
    } else {
      setDialing()
    }
  }

  override fun onAnswer() {
    Log.i("CallFxoConnection", "onAnswer called for: $direction")
    if (direction == "incoming") {
      (appContext.applicationContext as? li.power.app.callfxo.android.CallFxoApp)?.callController?.answerFromTelecom(inviteId)
    }
  }

  override fun onReject() {
    Log.i("CallFxoConnection", "onReject called for: $direction")
    if (direction == "incoming") {
      (appContext.applicationContext as? li.power.app.callfxo.android.CallFxoApp)?.callController?.rejectFromTelecom(inviteId)
    } else {
      (appContext.applicationContext as? li.power.app.callfxo.android.CallFxoApp)?.callController?.hangup()
    }
    setDisconnectedState()
  }

  override fun onAbort() {
    Log.i("CallFxoConnection", "onAbort called")
    (appContext.applicationContext as? li.power.app.callfxo.android.CallFxoApp)?.callController?.hangup()
    setDisconnectedState()
  }

  override fun onDisconnect() {
    Log.i("CallFxoConnection", "onDisconnect called")
    (appContext.applicationContext as? li.power.app.callfxo.android.CallFxoApp)?.callController?.hangup()
    setDisconnectedState()
  }

  fun setDialingState() {
    setDialing()
  }

  fun setActiveState() {
    setActive()
  }

  fun setDisconnectedState(cause: Int = DisconnectCause.LOCAL) {
    setDisconnected(DisconnectCause(cause))
    destroy()
  }
}
