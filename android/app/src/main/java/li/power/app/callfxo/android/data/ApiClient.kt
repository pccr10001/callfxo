package li.power.app.callfxo.android.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.contentOrNull
import okhttp3.Call
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

@Serializable
data class UserDto(
  val id: Long,
  val username: String,
  val role: String,
)

@Serializable
data class LoginRequest(
  val username: String,
  val password: String,
)

@Serializable
data class LoginResponse(
  val user: UserDto? = null,
)

@Serializable
data class MeResponse(
  val authenticated: Boolean = false,
  val user: UserDto? = null,
)

@Serializable
data class FxoBoxDto(
  val id: Long,
  val name: String,
  @SerialName("sip_username") val sipUsername: String,
  val online: Boolean = false,
  @SerialName("in_use") val inUse: Boolean = false,
)

@Serializable
data class FxoListResponse(
  val items: List<FxoBoxDto> = emptyList(),
)

@Serializable
data class ContactDto(
  val id: Long,
  val name: String,
  val number: String,
)

@Serializable
data class ContactsResponse(
  val items: List<ContactDto> = emptyList(),
)

@Serializable
data class CallLogDto(
  val id: Long,
  @SerialName("fxo_box_name") val fxoBoxName: String = "",
  val number: String,
  val status: String,
  val reason: String = "",
  @SerialName("started_at") val startedAt: String,
)

@Serializable
data class CallLogsResponse(
  val items: List<CallLogDto> = emptyList(),
  val page: Int = 1,
  @SerialName("total_pages") val totalPages: Int = 1,
)

class ApiClient(private val sessionStore: SessionStore) {
  private val json = Json { ignoreUnknownKeys = true }
  private val jsonMedia = "application/json; charset=utf-8".toMediaType()

  private val client = OkHttpClient.Builder()
    .connectTimeout(12, TimeUnit.SECONDS)
    .readTimeout(20, TimeUnit.SECONDS)
    .writeTimeout(20, TimeUnit.SECONDS)
    .build()

  suspend fun login(serverAddr: String, username: String, password: String): Result<UserDto> = runCatching {
    val base = normalizeServerAddr(serverAddr)
    val body = json.encodeToString(LoginRequest.serializer(), LoginRequest(username.trim(), password))
    val req = Request.Builder()
      .url("$base/api/login")
      .post(body.toRequestBody(jsonMedia))
      .header("Content-Type", "application/json")
      .build()

    val res = client.newCall(req).await()
    res.use {
      val raw = it.body?.string().orEmpty()
      if (!it.isSuccessful) {
        throw IOException(extractError(raw, it.code))
      }
      val login = json.decodeFromString(LoginResponse.serializer(), raw)
      val user = login.user ?: throw IOException("missing user")
      val cookie = parseSessionCookie(it) ?: throw IOException("missing session cookie")
      sessionStore.saveSession(base, user.username, cookie.first, cookie.second)
      user
    }
  }

  suspend fun me(): Result<MeResponse> = authedGet("/api/me") { body ->
    json.decodeFromString(MeResponse.serializer(), body)
  }

  suspend fun logout(): Result<Unit> = runCatching {
    val session = sessionStore.getSession() ?: return@runCatching
    val req = Request.Builder()
      .url("${session.serverAddr}/api/logout")
      .post("{}".toRequestBody(jsonMedia))
      .header("Cookie", "${session.cookieName}=${session.cookieValue}")
      .build()
    client.newCall(req).await().use { }
    sessionStore.clearSession()
  }

  suspend fun listFxo(): Result<List<FxoBoxDto>> = authedGet("/api/fxo") { body ->
    json.decodeFromString(FxoListResponse.serializer(), body).items
  }

  suspend fun listContacts(q: String): Result<List<ContactDto>> {
    val query = q.trim()
    val path = if (query.isBlank()) {
      "/api/contacts?limit=500"
    } else {
      "/api/contacts?limit=500&q=${urlEncode(query)}"
    }
    return authedGet(path) { body ->
      json.decodeFromString(ContactsResponse.serializer(), body).items
    }
  }

  suspend fun listCallLogs(page: Int, pageSize: Int): Result<CallLogsResponse> {
    val p = if (page <= 0) 1 else page
    val ps = pageSize.coerceIn(1, 100)
    return authedGet("/api/calls?page=$p&page_size=$ps") { body ->
      json.decodeFromString(CallLogsResponse.serializer(), body)
    }
  }

  fun currentSession(): SessionData? = sessionStore.getSession()

  private suspend fun <T> authedGet(path: String, decode: (String) -> T): Result<T> = runCatching {
    val session = sessionStore.getSession() ?: throw IOException("not logged in")
    val req = Request.Builder()
      .url(session.serverAddr + path)
      .get()
      .header("Cookie", "${session.cookieName}=${session.cookieValue}")
      .build()

    client.newCall(req).await().use { res ->
      val raw = res.body?.string().orEmpty()
      if (!res.isSuccessful) {
        if (res.code == 401) {
          sessionStore.clearSession()
        }
        throw IOException(extractError(raw, res.code))
      }
      decode(raw)
    }
  }

  private fun parseSessionCookie(response: Response): Pair<String, String>? {
    val all = response.headers("Set-Cookie")
    var fallback: Pair<String, String>? = null
    for (line in all) {
      val first = line.substringBefore(';')
      val name = first.substringBefore('=', "").trim()
      val value = first.substringAfter('=', "").trim()
      if (name.isNotBlank() && value.isNotBlank()) {
        val pair = name to value
        if (line.contains("httponly", ignoreCase = true)) {
          return pair
        }
        if (fallback == null) fallback = pair
      }
    }
    return fallback
  }

  private fun normalizeServerAddr(input: String): String {
    val raw = input.trim().trimEnd('/')
    if (raw.startsWith("http://", ignoreCase = true) || raw.startsWith("https://", ignoreCase = true)) return raw
    if (raw.startsWith("ws://", ignoreCase = true)) return "http://${raw.substringAfter("://")}"
    if (raw.startsWith("wss://", ignoreCase = true)) return "https://${raw.substringAfter("://")}"
    return "http://$raw"
  }

  private fun extractError(raw: String, code: Int): String {
    return try {
      val node = json.parseToJsonElement(raw)
      val msg = node.jsonObject["error"]?.jsonPrimitive?.contentOrNull ?: node.toString()
      if (msg.isNotBlank()) "HTTP $code: $msg" else "HTTP $code"
    } catch (_: Exception) {
      if (raw.isNotBlank()) "HTTP $code: $raw" else "HTTP $code"
    }
  }

  private fun urlEncode(v: String): String = java.net.URLEncoder.encode(v, Charsets.UTF_8.name())
}

private suspend fun Call.await(): Response = withContext(Dispatchers.IO) {
  suspendCancellableCoroutine { cont ->
    enqueue(object : okhttp3.Callback {
      override fun onFailure(call: Call, e: IOException) {
        if (cont.isActive) cont.resumeWithException(e)
      }

      override fun onResponse(call: Call, response: Response) {
        if (cont.isActive) cont.resume(response)
      }
    })
    cont.invokeOnCancellation { cancel() }
  }
}
