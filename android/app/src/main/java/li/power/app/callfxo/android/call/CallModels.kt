package li.power.app.callfxo.android.call

data class PendingIncomingCall(
  val inviteId: String,
  val boxId: Long,
  val boxName: String,
  val callerId: String,
  val remoteNumber: String,
  val state: String,
  val receivedAtMs: Long = System.currentTimeMillis(),
)

data class OutgoingCallRequest(
  val boxId: Long,
  val boxName: String,
  val number: String,
)

data class ActiveCallInfo(
  val displayName: String = "",
  val subtitle: String = "",
  val direction: String = "",
  val connectedAtMs: Long = 0L,
)
