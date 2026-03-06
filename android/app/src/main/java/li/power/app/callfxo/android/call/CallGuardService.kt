package li.power.app.callfxo.android.call

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import li.power.app.callfxo.android.MainActivity
import li.power.app.callfxo.android.R

class CallGuardService : Service() {
  override fun onBind(intent: Intent?): IBinder? = null

  override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
    startForeground(NOTIF_ID, buildNotification())
    return START_STICKY
  }

  private fun buildNotification(): Notification {
    ensureChannel()
    val launch = Intent(this, MainActivity::class.java)
      .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP)
    val pi = PendingIntent.getActivity(
      this,
      3001,
      launch,
      PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
    )

    return NotificationCompat.Builder(this, CHANNEL_ID)
      .setSmallIcon(android.R.drawable.stat_sys_phone_call)
      .setContentTitle(getString(R.string.notif_call_active))
      .setContentText(getString(R.string.notif_call_active_desc))
      .setContentIntent(pi)
      .setOngoing(true)
      .setOnlyAlertOnce(true)
      .build()
  }

  private fun ensureChannel() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
    val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    val ch = NotificationChannel(
      CHANNEL_ID,
      getString(R.string.notif_channel_call),
      NotificationManager.IMPORTANCE_LOW
    )
    nm.createNotificationChannel(ch)
  }

  companion object {
    private const val CHANNEL_ID = "callfxo_call_guard"
    private const val NOTIF_ID = 3001
    private const val ACTION_START = "li.power.app.callfxo.android.action.START_CALL_GUARD"

    fun start(context: Context) {
      val intent = Intent(context, CallGuardService::class.java).setAction(ACTION_START)
      context.startForegroundService(intent)
    }

    fun stop(context: Context) {
      context.stopService(Intent(context, CallGuardService::class.java))
    }
  }
}
