package li.power.app.callfxo.android.call

import android.telecom.Connection
import android.telecom.ConnectionRequest
import android.telecom.ConnectionService
import android.telecom.DisconnectCause
import android.telecom.PhoneAccountHandle
import li.power.app.callfxo.android.CallFxoApp

class CallConnectionService : ConnectionService() {
  override fun onCreateIncomingConnection(connectionManagerPhoneAccount: PhoneAccountHandle, request: ConnectionRequest): Connection {
    val address = request.extras?.getString("callfxo_number").orEmpty()
    val subject = request.extras?.getString(android.telecom.TelecomManager.EXTRA_CALL_SUBJECT).orEmpty()
    val connection = CallFxoConnection(
      appContext = applicationContext,
      direction = "incoming",
      inviteId = CallTelecomManager.inviteIdFromExtras(request.extras),
      displayName = subject.ifBlank { address },
      number = address,
    )
    CallTelecomManager.bindConnection(connection)
    return connection
  }

  override fun onCreateOutgoingConnection(connectionManagerPhoneAccount: PhoneAccountHandle, request: ConnectionRequest): Connection {
    val outgoing = CallTelecomManager.consumeOutgoing(request)
    if (outgoing == null) {
      android.util.Log.e("CallConnectionService", "onCreateOutgoingConnection: no pending request found, failing connection")
      return Connection.createFailedConnection(DisconnectCause(DisconnectCause.ERROR))
    }
    val connection = CallFxoConnection(
      appContext = applicationContext,
      direction = "outgoing",
      displayName = outgoing.boxName.ifBlank { outgoing.number },
      number = outgoing.number,
    )
    CallTelecomManager.bindConnection(connection)
    (applicationContext as? CallFxoApp)?.callController?.startOutgoingFromTelecom(outgoing)
    return connection
  }
}
