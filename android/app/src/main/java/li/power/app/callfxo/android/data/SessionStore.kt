package li.power.app.callfxo.android.data

import android.content.Context
import android.content.SharedPreferences

data class SessionData(
  val serverAddr: String,
  val username: String,
  val cookieName: String,
  val cookieValue: String,
)

class SessionStore(context: Context) {
  private val prefs: SharedPreferences = context.getSharedPreferences("callfxo_session", Context.MODE_PRIVATE)

  fun saveServerAddr(serverAddr: String) {
    prefs.edit().putString(KEY_SERVER_ADDR, serverAddr.trim()).apply()
  }

  fun getServerAddr(): String = prefs.getString(KEY_SERVER_ADDR, "")?.trim().orEmpty()

  fun saveSession(serverAddr: String, username: String, cookieName: String, cookieValue: String) {
    prefs.edit()
      .putString(KEY_SERVER_ADDR, serverAddr.trim())
      .putString(KEY_USERNAME, username.trim())
      .putString(KEY_COOKIE_NAME, cookieName.trim())
      .putString(KEY_COOKIE_VALUE, cookieValue.trim())
      .apply()
  }

  fun getSession(): SessionData? {
    val server = getServerAddr()
    val user = prefs.getString(KEY_USERNAME, "")?.trim().orEmpty()
    val cName = prefs.getString(KEY_COOKIE_NAME, "")?.trim().orEmpty()
    val cValue = prefs.getString(KEY_COOKIE_VALUE, "")?.trim().orEmpty()
    if (server.isBlank() || user.isBlank() || cName.isBlank() || cValue.isBlank()) return null
    return SessionData(server, user, cName, cValue)
  }

  fun clearSession() {
    prefs.edit()
      .remove(KEY_USERNAME)
      .remove(KEY_COOKIE_NAME)
      .remove(KEY_COOKIE_VALUE)
      .apply()
  }

  companion object {
    private const val KEY_SERVER_ADDR = "server_addr"
    private const val KEY_USERNAME = "username"
    private const val KEY_COOKIE_NAME = "cookie_name"
    private const val KEY_COOKIE_VALUE = "cookie_value"
  }
}
