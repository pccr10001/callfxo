package li.power.app.callfxo.android.data

import java.io.IOException
import java.net.URLEncoder
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Call
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response

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
  @SerialName("device_token") val deviceToken: String,
  @SerialName("client_type") val clientType: String,
  @SerialName("device_name") val deviceName: String,
)

@Serializable
data class RefreshRequest(
  @SerialName("device_token") val deviceToken: String,
  @SerialName("refresh_token") val refreshToken: String,
  @SerialName("client_type") val clientType: String,
  @SerialName("device_name") val deviceName: String,
)

@Serializable
data class LoginResponse(
  val user: UserDto? = null,
  @SerialName("access_token") val accessToken: String = "",
  @SerialName("refresh_token") val refreshToken: String = "",
  @SerialName("device_token") val deviceToken: String = "",
  @SerialName("push_config") val pushConfig: PushConfigData? = null,
)

@Serializable
data class MeResponse(
  val authenticated: Boolean = false,
  val user: UserDto? = null,
  @SerialName("device_token") val deviceToken: String = "",
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

@Serializable
data class IncomingCallDto(
  val id: String,
  @SerialName("box_id") val boxId: Long,
  @SerialName("box_name") val boxName: String = "",
  @SerialName("caller_id") val callerId: String = "",
  @SerialName("remote_number") val remoteNumber: String = "",
  val state: String = "",
)

@Serializable
data class IncomingCallsResponse(
  val items: List<IncomingCallDto> = emptyList(),
)

@Serializable
data class PushConfigResponse(
  val item: PushConfigData = PushConfigData(),
)

@Serializable
data class PushTokenRequest(
  @SerialName("push_token") val pushToken: String,
  @SerialName("push_platform") val pushPlatform: String,
)

class ApiClient(private val sessionStore: SessionStore) {
  private val json = Json { ignoreUnknownKeys = true }
  private val jsonMedia = "application/json; charset=utf-8".toMediaType()
  private val client = OkHttpClient.Builder()
    .connectTimeout(12, TimeUnit.SECONDS)
    .readTimeout(20, TimeUnit.SECONDS)
    .writeTimeout(20, TimeUnit.SECONDS)
    .build()
  private val refreshMutex = Mutex()

  suspend fun login(serverAddr: String, username: String, password: String): Result<UserDto> = runCatching {
    val base = normalizeServerAddr(serverAddr)
    val deviceToken = sessionStore.getOrCreateDeviceToken()
    val body = json.encodeToString(LoginRequest.serializer(), LoginRequest(
      username = username.trim(),
      password = password,
      deviceToken = deviceToken,
      clientType = "android",
      deviceName = android.os.Build.MODEL ?: "android",
    ))
    val req = Request.Builder()
      .url("$base/api/login")
      .post(body.toRequestBody(jsonMedia))
      .header("Content-Type", "application/json")
      .build()

    client.newCall(req).await().use {
      val raw = it.body?.string().orEmpty()
      if (!it.isSuccessful) {
        throw IOException(extractError(raw, it.code))
      }
      val login = json.decodeFromString(LoginResponse.serializer(), raw)
      val user = login.user ?: throw IOException("missing user")
      sessionStore.saveSession(
        serverAddr = base,
        username = user.username,
        accessToken = login.accessToken,
        refreshToken = login.refreshToken,
        deviceToken = if (login.deviceToken.isBlank()) deviceToken else login.deviceToken,
      )
      login.pushConfig?.let(sessionStore::savePushConfig)
      user
    }
  }

  suspend fun refresh(): Result<Unit> = runCatching {
    val session = sessionStore.getSession() ?: throw IOException("not logged in")
    val body = json.encodeToString(RefreshRequest.serializer(), RefreshRequest(
      deviceToken = session.deviceToken,
      refreshToken = session.refreshToken,
      clientType = "android",
      deviceName = android.os.Build.MODEL ?: "android",
    ))
    val req = Request.Builder()
      .url("${session.serverAddr}/api/refresh")
      .post(body.toRequestBody(jsonMedia))
      .header("Content-Type", "application/json")
      .build()
    client.newCall(req).await().use {
      val raw = it.body?.string().orEmpty()
      if (!it.isSuccessful) {
        if (it.code == 401) {
          sessionStore.clearSession()
        }
        throw IOException(extractError(raw, it.code))
      }
      val resp = json.decodeFromString(LoginResponse.serializer(), raw)
      val current = sessionStore.getSession() ?: session
      sessionStore.saveSession(
        serverAddr = current.serverAddr,
        username = resp.user?.username ?: current.username,
        accessToken = resp.accessToken,
        refreshToken = resp.refreshToken,
        deviceToken = if (resp.deviceToken.isBlank()) current.deviceToken else resp.deviceToken,
      )
      resp.pushConfig?.let(sessionStore::savePushConfig)
    }
  }

  suspend fun refreshWithRetry(maxAttempts: Int = 3): Result<Unit> = refreshMutex.withLock {
    refreshWithRetryInternal(maxAttempts)
  }

  suspend fun me(): Result<MeResponse> = authedGet("/api/me") { body ->
    json.decodeFromString(MeResponse.serializer(), body)
  }

  suspend fun logout(): Result<Unit> = runCatching {
    val session = sessionStore.getSession() ?: return@runCatching
    val req = Request.Builder()
      .url("${session.serverAddr}/api/logout")
      .post("{}".toRequestBody(jsonMedia))
      .header("Authorization", "Bearer ${session.accessToken}")
      .build()
    client.newCall(req).await().use { }
    sessionStore.clearSession()
  }

  suspend fun listFxo(): Result<List<FxoBoxDto>> = authedGet("/api/fxo") { body ->
    json.decodeFromString(FxoListResponse.serializer(), body).items
  }

  suspend fun listContacts(q: String): Result<List<ContactDto>> {
    val query = q.trim()
    val path = if (query.isBlank()) "/api/contacts?limit=500" else "/api/contacts?limit=500&q=${urlEncode(query)}"
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

  suspend fun listIncomingCalls(): Result<List<IncomingCallDto>> = authedGet("/api/incoming") { body ->
    json.decodeFromString(IncomingCallsResponse.serializer(), body).items
  }

  suspend fun getPushConfig(): Result<PushConfigData> = authedGet("/api/push/config") { body ->
    json.decodeFromString(PushConfigResponse.serializer(), body).item
  }

  suspend fun registerPushToken(pushToken: String, pushPlatform: String): Result<Unit> = runCatching {
    authedRequest(
      "/api/device/push",
      "POST",
      json.encodeToString(PushTokenRequest.serializer(), PushTokenRequest(pushToken, pushPlatform))
    )
  }

  fun currentSession(): SessionData? = sessionStore.getSession()

  private suspend fun <T> authedGet(path: String, decode: (String) -> T): Result<T> = runCatching {
    val raw = authedRequest(path, "GET", null)
    decode(raw)
  }

  private suspend fun authedRequest(path: String, method: String, body: String?): String {
    val session = sessionStore.getSession() ?: throw IOException("not logged in")
    val reqBuilder = Request.Builder()
      .url(session.serverAddr + path)
      .header("Authorization", "Bearer ${session.accessToken}")
    val req = when (method) {
      "POST" -> reqBuilder.post((body ?: "{}").toRequestBody(jsonMedia)).build()
      "PUT" -> reqBuilder.put((body ?: "{}").toRequestBody(jsonMedia)).build()
      "DELETE" -> if (body == null) reqBuilder.delete().build() else reqBuilder.delete(body.toRequestBody(jsonMedia)).build()
      else -> reqBuilder.get().build()
    }
    client.newCall(req).await().use { res ->
      val raw = res.body?.string().orEmpty()
      if (res.code == 401) {
        refreshIfNeeded(session).getOrThrow()
        val refreshed = sessionStore.getSession() ?: throw IOException("refresh failed")
        val retry = Request.Builder()
          .url(refreshed.serverAddr + path)
          .header("Authorization", "Bearer ${refreshed.accessToken}")
          .apply {
            when (method) {
              "POST" -> post((body ?: "{}").toRequestBody(jsonMedia))
              "PUT" -> put((body ?: "{}").toRequestBody(jsonMedia))
              "DELETE" -> if (body == null) delete() else delete(body.toRequestBody(jsonMedia))
              else -> get()
            }
          }
          .build()
        client.newCall(retry).await().use { retryRes ->
          val retryRaw = retryRes.body?.string().orEmpty()
          if (!retryRes.isSuccessful) {
            throw IOException(extractError(retryRaw, retryRes.code))
          }
          return retryRaw
        }
      }
      if (!res.isSuccessful) {
        throw IOException(extractError(raw, res.code))
      }
      return raw
    }
  }

  private suspend fun refreshIfNeeded(session: SessionData): Result<Unit> = refreshMutex.withLock {
    val current = sessionStore.getSession() ?: return@withLock Result.failure(IOException("not logged in"))
    if (current.accessToken != session.accessToken) {
      return@withLock Result.success(Unit)
    }
    refreshWithRetryInternal(3)
  }

  private suspend fun refreshWithRetryInternal(maxAttempts: Int): Result<Unit> {
    val attempts = maxAttempts.coerceIn(1, 5)
    var lastFailure: Result<Unit>? = null
    repeat(attempts) { idx ->
      val res = refresh()
      if (res.isSuccess) return res
      val err = res.exceptionOrNull()
      if (err != null && isAuthError(err)) return res
      lastFailure = res
      if (idx < attempts - 1) {
        delay(backoffDelayMs(idx))
      }
    }
    return lastFailure ?: Result.failure(IOException("refresh failed"))
  }

  private fun backoffDelayMs(attemptIndex: Int): Long {
    return when (attemptIndex) {
      0 -> 250L
      1 -> 800L
      else -> 1500L
    }
  }

  private fun isAuthError(err: Throwable): Boolean {
    val msg = err.message ?: return false
    return msg.startsWith("HTTP 401")
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

  private fun urlEncode(v: String): String = URLEncoder.encode(v, Charsets.UTF_8.name())
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
