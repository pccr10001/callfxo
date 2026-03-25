package li.power.app.callfxo.android.data

import android.content.Context
import android.content.SharedPreferences
import android.media.RingtoneManager
import java.util.UUID
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

@Serializable
data class PushConfigData(
  val enabled: Boolean = false,
  val project_id: String = "",
  @SerialName("android_app_id") val app_id: String = "",
  @SerialName("android_api_key") val api_key: String = "",
  val messaging_sender_id: String = "",
  val auth_domain: String = "",
  val storage_bucket: String = "",
  val measurement_id: String = "",
  val vapid_key: String = "",
)

@Serializable
data class WebRTCIceServerData(
  val urls: List<String> = emptyList(),
  val username: String = "",
  val credential: String = "",
)

@Serializable
data class WebRTCConfigData(
  @SerialName("ice_servers") val iceServers: List<WebRTCIceServerData> = emptyList(),
)

data class SessionData(
  val serverAddr: String,
  val username: String,
  val accessToken: String,
  val refreshToken: String,
  val deviceToken: String,
)

class SessionStore(context: Context) {
  private val prefs: SharedPreferences = context.getSharedPreferences("callfxo_session", Context.MODE_PRIVATE)
  private val json = Json { ignoreUnknownKeys = true }
  private val defaultRingtoneUri = RingtoneManager.getDefaultUri(RingtoneManager.TYPE_RINGTONE)?.toString().orEmpty()

  fun saveServerAddr(serverAddr: String) {
    prefs.edit().putString(KEY_SERVER_ADDR, serverAddr.trim()).apply()
  }

  fun getServerAddr(): String = prefs.getString(KEY_SERVER_ADDR, "")?.trim().orEmpty()

  fun getOrCreateDeviceToken(): String {
    val existing = prefs.getString(KEY_DEVICE_TOKEN, "")?.trim().orEmpty()
    if (existing.isNotBlank()) return existing
    val created = "android-${UUID.randomUUID()}"
    prefs.edit().putString(KEY_DEVICE_TOKEN, created).apply()
    return created
  }

  fun saveSession(serverAddr: String, username: String, accessToken: String, refreshToken: String, deviceToken: String) {
    prefs.edit()
      .putString(KEY_SERVER_ADDR, serverAddr.trim())
      .putString(KEY_USERNAME, username.trim())
      .putString(KEY_ACCESS_TOKEN, accessToken.trim())
      .putString(KEY_REFRESH_TOKEN, refreshToken.trim())
      .putString(KEY_DEVICE_TOKEN, deviceToken.trim())
      .apply()
  }

  fun updateAccessToken(accessToken: String) {
    prefs.edit().putString(KEY_ACCESS_TOKEN, accessToken.trim()).apply()
  }

  fun updateRefreshToken(refreshToken: String) {
    prefs.edit().putString(KEY_REFRESH_TOKEN, refreshToken.trim()).apply()
  }

  fun getSession(): SessionData? {
    val server = getServerAddr()
    val user = prefs.getString(KEY_USERNAME, "")?.trim().orEmpty()
    val access = prefs.getString(KEY_ACCESS_TOKEN, "")?.trim().orEmpty()
    val refresh = prefs.getString(KEY_REFRESH_TOKEN, "")?.trim().orEmpty()
    val device = getOrCreateDeviceToken()
    if (server.isBlank() || user.isBlank() || access.isBlank() || refresh.isBlank()) return null
    return SessionData(server, user, access, refresh, device)
  }

  fun savePushConfig(config: PushConfigData) {
    prefs.edit().putString(KEY_PUSH_CONFIG, json.encodeToString(PushConfigData.serializer(), config)).apply()
  }

  fun getPushConfig(): PushConfigData? {
    val raw = prefs.getString(KEY_PUSH_CONFIG, "")?.trim().orEmpty()
    if (raw.isBlank()) return null
    return runCatching { json.decodeFromString(PushConfigData.serializer(), raw) }.getOrNull()
  }

  fun saveWebRTCConfig(config: WebRTCConfigData) {
    prefs.edit().putString(KEY_WEBRTC_CONFIG, json.encodeToString(WebRTCConfigData.serializer(), config)).apply()
  }

  fun getWebRTCConfig(): WebRTCConfigData? {
    val raw = prefs.getString(KEY_WEBRTC_CONFIG, "")?.trim().orEmpty()
    if (raw.isBlank()) return null
    return runCatching { json.decodeFromString(WebRTCConfigData.serializer(), raw) }.getOrNull()
  }

  fun saveRingtoneUri(uri: String?) {
    val normalized = (uri ?: "").trim()
    prefs.edit().putString(KEY_RINGTONE_URI, normalized.ifBlank { defaultRingtoneUri }).apply()
  }

  fun getRingtoneUri(): String {
    val raw = prefs.getString(KEY_RINGTONE_URI, "")?.trim().orEmpty()
    return raw.ifBlank { defaultRingtoneUri }
  }

  fun saveRingtoneVolume(volume: Float) {
    prefs.edit().putFloat(KEY_RINGTONE_VOLUME, volume.coerceIn(0f, 1f)).apply()
  }

  fun getRingtoneVolume(): Float {
    val stored = prefs.getFloat(KEY_RINGTONE_VOLUME, 0.85f)
    return stored.coerceIn(0f, 1f)
  }

  fun clearSession() {
    prefs.edit()
      .remove(KEY_USERNAME)
      .remove(KEY_ACCESS_TOKEN)
      .remove(KEY_REFRESH_TOKEN)
      .remove(KEY_PUSH_CONFIG)
      .remove(KEY_WEBRTC_CONFIG)
      .apply()
  }

  companion object {
    private const val KEY_SERVER_ADDR = "server_addr"
    private const val KEY_USERNAME = "username"
    private const val KEY_ACCESS_TOKEN = "access_token"
    private const val KEY_REFRESH_TOKEN = "refresh_token"
    private const val KEY_DEVICE_TOKEN = "device_token"
    private const val KEY_PUSH_CONFIG = "push_config"
    private const val KEY_WEBRTC_CONFIG = "webrtc_config"
    private const val KEY_RINGTONE_URI = "ringtone_uri"
    private const val KEY_RINGTONE_VOLUME = "ringtone_volume"
  }
}
