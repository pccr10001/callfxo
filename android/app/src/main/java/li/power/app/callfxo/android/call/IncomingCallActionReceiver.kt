package li.power.app.callfxo.android.call

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import li.power.app.callfxo.android.CallFxoApp

class IncomingCallActionReceiver : BroadcastReceiver() {
  override fun onReceive(context: Context, intent: Intent) {
    val app = context.applicationContext as? CallFxoApp ?: return
    val inviteId = intent.getStringExtra(CallNotificationManager.EXTRA_INVITE_ID).orEmpty()
    val pending = goAsync()
    app.callController.handleNotificationAction(intent.action, inviteId) {
      pending.finish()
    }
  }
}
