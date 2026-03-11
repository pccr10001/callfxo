package li.power.app.callfxo.android.call

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import li.power.app.callfxo.android.MainActivity
import li.power.app.callfxo.android.R

class CallNotificationManager(private val context: Context) {
  private val notifications = NotificationManagerCompat.from(context)

  fun showIncomingCall(call: PendingIncomingCall) {
    ensureChannels()
    val openIntent = Intent(context, MainActivity::class.java)
      .putExtra(EXTRA_INVITE_ID, call.inviteId)
      .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_NEW_TASK)
    val openPending = PendingIntent.getActivity(
      context,
      REQUEST_OPEN,
      openIntent,
      PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    val acceptPending = PendingIntent.getBroadcast(
      context,
      REQUEST_ACCEPT,
      Intent(context, IncomingCallActionReceiver::class.java)
        .setAction(ACTION_ACCEPT)
        .putExtra(EXTRA_INVITE_ID, call.inviteId),
      PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    val rejectPending = PendingIntent.getBroadcast(
      context,
      REQUEST_REJECT,
      Intent(context, IncomingCallActionReceiver::class.java)
        .setAction(ACTION_REJECT)
        .putExtra(EXTRA_INVITE_ID, call.inviteId),
      PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    val title = firstNonBlank(call.callerId, call.remoteNumber, context.getString(R.string.incoming_unknown))
    val body = if (call.boxName.isBlank()) {
      context.getString(R.string.notif_incoming_body_generic)
    } else {
      context.getString(R.string.notif_incoming_body, call.boxName)
    }

    val notification = NotificationCompat.Builder(context, CHANNEL_INCOMING)
      .setSmallIcon(android.R.drawable.stat_sys_phone_call)
      .setContentTitle(title)
      .setContentText(body)
      .setCategory(NotificationCompat.CATEGORY_CALL)
      .setPriority(NotificationCompat.PRIORITY_MAX)
      .setOngoing(true)
      .setAutoCancel(false)
      .setContentIntent(openPending)
      .setFullScreenIntent(openPending, true)
      .addAction(0, context.getString(R.string.action_reject), rejectPending)
      .addAction(0, context.getString(R.string.action_answer), acceptPending)
      .build()

    notifications.notify(NOTIFICATION_ID_INCOMING, notification)
  }

  fun cancelIncomingCall() {
    notifications.cancel(NOTIFICATION_ID_INCOMING)
  }

  fun showCallEnded(message: String) {
    ensureChannels()
    val notification = NotificationCompat.Builder(context, CHANNEL_EVENTS)
      .setSmallIcon(android.R.drawable.stat_notify_missed_call)
      .setContentTitle(context.getString(R.string.notif_call_ended))
      .setContentText(message.ifBlank { context.getString(R.string.notif_call_ended) })
      .setAutoCancel(true)
      .setPriority(NotificationCompat.PRIORITY_DEFAULT)
      .build()
    notifications.notify(NOTIFICATION_ID_EVENT, notification)
  }

  fun ensureChannels() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
    val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    nm.createNotificationChannel(
      NotificationChannel(
        CHANNEL_INCOMING,
        context.getString(R.string.notif_channel_incoming),
        NotificationManager.IMPORTANCE_HIGH,
      )
    )
    nm.createNotificationChannel(
      NotificationChannel(
        CHANNEL_EVENTS,
        context.getString(R.string.notif_channel_call),
        NotificationManager.IMPORTANCE_DEFAULT,
      )
    )
  }

  companion object {
    const val ACTION_ACCEPT = "li.power.app.callfxo.android.action.ANSWER_INCOMING"
    const val ACTION_REJECT = "li.power.app.callfxo.android.action.REJECT_INCOMING"
    const val EXTRA_INVITE_ID = "invite_id"

    private const val NOTIFICATION_ID_INCOMING = 3002
    private const val NOTIFICATION_ID_EVENT = 3003
    private const val REQUEST_OPEN = 4100
    private const val REQUEST_ACCEPT = 4101
    private const val REQUEST_REJECT = 4102
    const val CHANNEL_INCOMING = "callfxo_incoming"
    const val CHANNEL_EVENTS = "callfxo_event"

    private fun firstNonBlank(vararg values: String): String {
      return values.firstOrNull { it.isNotBlank() }.orEmpty()
    }
  }
}
