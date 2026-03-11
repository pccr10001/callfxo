package li.power.app.callfxo.android.push

import android.content.Context
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.tasks.await
import li.power.app.callfxo.android.data.ApiClient
import li.power.app.callfxo.android.data.PushConfigData
import li.power.app.callfxo.android.data.SessionStore

object PushRuntime {
  private const val PLATFORM_ANDROID_FCM = "android_fcm"

  fun ensureInitialized(context: Context, sessionStore: SessionStore): Boolean {
    val cfg = sessionStore.getPushConfig() ?: return false
    if (!isUsable(cfg)) return false

    synchronized(this) {
      val existing = FirebaseApp.getApps(context).firstOrNull()
      if (existing != null) {
        return true
      }
      FirebaseApp.initializeApp(context, buildOptions(cfg))
    }
    return true
  }

  suspend fun fetchToken(context: Context, sessionStore: SessionStore): Result<String> = runCatching {
    check(ensureInitialized(context, sessionStore)) { "FCM is disabled" }
    FirebaseMessaging.getInstance().token.await()
  }

  suspend fun syncToken(context: Context, sessionStore: SessionStore, apiClient: ApiClient): Result<String> = runCatching {
    val token = fetchToken(context, sessionStore).getOrThrow()
    apiClient.registerPushToken(token, PLATFORM_ANDROID_FCM).getOrThrow()
    token
  }

  fun platform(): String = PLATFORM_ANDROID_FCM

  private fun isUsable(cfg: PushConfigData): Boolean {
    return cfg.enabled &&
      cfg.project_id.isNotBlank() &&
      cfg.app_id.isNotBlank() &&
      cfg.api_key.isNotBlank() &&
      cfg.messaging_sender_id.isNotBlank()
  }

  private fun buildOptions(cfg: PushConfigData): FirebaseOptions {
    return FirebaseOptions.Builder()
      .setProjectId(cfg.project_id)
      .setApplicationId(cfg.app_id)
      .setApiKey(cfg.api_key)
      .setGcmSenderId(cfg.messaging_sender_id)
      .apply {
        if (cfg.storage_bucket.isNotBlank()) {
          setStorageBucket(cfg.storage_bucket)
        }
      }
      .build()
  }
}
