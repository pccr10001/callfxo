package li.power.app.callfxo.android

import android.app.Application
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import li.power.app.callfxo.android.call.CallController
import li.power.app.callfxo.android.call.CallNotificationManager
import li.power.app.callfxo.android.data.ApiClient
import li.power.app.callfxo.android.data.SessionStore
import li.power.app.callfxo.android.push.PushRuntime

class CallFxoApp : Application() {
  val applicationScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
  lateinit var sessionStore: SessionStore
    private set
  lateinit var apiClient: ApiClient
    private set
  lateinit var callNotificationManager: CallNotificationManager
    private set
  lateinit var callController: CallController
    private set

  override fun onCreate() {
    super.onCreate()
    sessionStore = SessionStore(this)
    apiClient = ApiClient(sessionStore)
    callNotificationManager = CallNotificationManager(this)
    PushRuntime.ensureInitialized(this, sessionStore)
    callController = CallController(
      context = this,
      sessionStore = sessionStore,
      apiClient = apiClient,
      notificationManager = callNotificationManager,
    )
  }

  companion object {
    fun from(app: Application): CallFxoApp = app as CallFxoApp
  }
}
