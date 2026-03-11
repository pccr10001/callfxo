package li.power.app.callfxo.android.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import kotlinx.coroutines.launch
import li.power.app.callfxo.android.CallFxoApp

class CallFxoMessagingService : FirebaseMessagingService() {
  override fun onNewToken(token: String) {
    val app = applicationContext as? CallFxoApp ?: return
    app.applicationScope.launch {
      app.apiClient.registerPushToken(token, PushRuntime.platform()).onFailure { }
    }
  }

  override fun onMessageReceived(message: RemoteMessage) {
    val app = applicationContext as? CallFxoApp ?: return
    val data = message.data
    if (data.isNullOrEmpty()) return
    app.callController.handlePushPayload(data)
  }
}
