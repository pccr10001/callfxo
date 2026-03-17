package li.power.app.callfxo.android

import android.Manifest
import android.content.Intent
import android.media.RingtoneManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Backspace
import androidx.compose.material.icons.filled.BluetoothAudio
import androidx.compose.material.icons.filled.Call
import androidx.compose.material.icons.filled.CallEnd
import androidx.compose.material.icons.filled.Dialpad
import androidx.compose.material.icons.filled.Hearing
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.MicOff
import androidx.compose.material.icons.filled.VolumeUp
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Slider
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlin.system.exitProcess
import kotlinx.coroutines.delay
import li.power.app.callfxo.android.call.AudioOutput
import li.power.app.callfxo.android.call.PendingIncomingCall
import li.power.app.callfxo.android.data.ContactSource

class MainActivity : ComponentActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)

    setContent {
      MaterialTheme {
        val vm: MainViewModel = viewModel()
        val ui by vm.ui.collectAsStateWithLifecycle()
        val lifecycleOwner = LocalLifecycleOwner.current

        var pendingDial by remember { mutableStateOf(false) }
        var pendingAnswer by remember { mutableStateOf(false) }
        var pendingLoadDeviceContacts by remember { mutableStateOf(false) }

        val audioPermLauncher = rememberLauncherForActivityResult(
          ActivityResultContracts.RequestPermission()
        ) { granted ->
          if (granted && pendingDial) vm.dial()
          if (granted && pendingAnswer) vm.acceptIncoming()
          pendingDial = false
          pendingAnswer = false
        }

        val contactsPermLauncher = rememberLauncherForActivityResult(
          ActivityResultContracts.RequestPermission()
        ) { granted ->
          if (granted && pendingLoadDeviceContacts) vm.loadDeviceContacts()
          pendingLoadDeviceContacts = false
        }

        val notifPermLauncher = rememberLauncherForActivityResult(
          ActivityResultContracts.RequestPermission()
        ) { }

        val ringtoneLauncher = rememberLauncherForActivityResult(
          ActivityResultContracts.StartActivityForResult()
        ) { result ->
          val pickedUri = result.data?.getParcelableExtra<Uri>(RingtoneManager.EXTRA_RINGTONE_PICKED_URI)
          vm.setRingtoneUri(pickedUri?.toString())
        }

        DisposableEffect(lifecycleOwner) {
          val observer = LifecycleEventObserver { _, event ->
            when (event) {
              Lifecycle.Event.ON_START -> vm.onForegroundChanged(true)
              Lifecycle.Event.ON_STOP -> vm.onForegroundChanged(false)
              else -> Unit
            }
          }
          lifecycleOwner.lifecycle.addObserver(observer)
          onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
        }

        LaunchedEffect(Unit) {
          if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU && !hasPermission(Manifest.permission.POST_NOTIFICATIONS)) {
            notifPermLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
          }
        }

        Box(modifier = Modifier.fillMaxSize()) {
          if (!ui.loggedIn) {
            LoginScreen(
              state = ui,
              onServer = vm::onServerAddrChange,
              onUsername = vm::onUsernameChange,
              onPassword = vm::onPasswordChange,
              onLogin = vm::login,
            )
          } else {
            MainScreen(
              state = ui,
              onLogout = vm::logout,
              onExit = {
                vm.shutdown()
                finishAffinity()
                exitProcess(0)
              },
              onRefresh = vm::refreshAll,
              onSelectBox = vm::onSelectBox,
              onDialPadPress = vm::onDialPadPress,
              onDialBackspace = vm::onBackspaceDial,
              onDial = {
                if (hasPermission(Manifest.permission.RECORD_AUDIO)) {
                  vm.dial()
                } else {
                  pendingDial = true
                  audioPermLauncher.launch(Manifest.permission.RECORD_AUDIO)
                }
              },
              onSearch = vm::onSearchChange,
              onLoadDeviceContacts = {
                if (hasPermission(Manifest.permission.READ_CONTACTS)) {
                  vm.loadDeviceContacts()
                } else {
                  pendingLoadDeviceContacts = true
                  contactsPermLauncher.launch(Manifest.permission.READ_CONTACTS)
                }
              },
              onContactClick = { number -> vm.fillDialNumber(number) },
              onPrevLog = vm::prevLogPage,
              onNextLog = vm::nextLogPage,
              onLogClick = { number -> vm.fillDialNumber(number) },
              onPickRingtone = {
                val intent = Intent(RingtoneManager.ACTION_RINGTONE_PICKER).apply {
                  putExtra(RingtoneManager.EXTRA_RINGTONE_TYPE, RingtoneManager.TYPE_RINGTONE)
                  putExtra(RingtoneManager.EXTRA_RINGTONE_SHOW_DEFAULT, true)
                  putExtra(RingtoneManager.EXTRA_RINGTONE_SHOW_SILENT, false)
                  val existing = ui.ringtoneUri.takeIf { it.isNotBlank() }?.let(Uri::parse)
                  putExtra(RingtoneManager.EXTRA_RINGTONE_EXISTING_URI, existing)
                }
                ringtoneLauncher.launch(intent)
              },
              onSetRingtoneVolume = vm::setRingtoneVolume,
            )
          }

          if (ui.noticeMessage != null) {
            AlertDialog(
              onDismissRequest = vm::clearNotice,
              title = { Text("CallFXO") },
              text = { Text(ui.noticeMessage ?: "") },
              confirmButton = {
                TextButton(onClick = vm::clearNotice) { Text("OK") }
              },
            )
          }

          if (ui.incomingCall != null && !ui.callBusy) {
            IncomingCallOverlay(
              state = ui,
              onAccept = {
                if (hasPermission(Manifest.permission.RECORD_AUDIO)) {
                  vm.acceptIncoming()
                } else {
                  pendingAnswer = true
                  audioPermLauncher.launch(Manifest.permission.RECORD_AUDIO)
                }
              },
              onReject = vm::rejectIncoming,
            )
          }

          if (ui.callBusy || ui.inCall) {
            ActiveCallOverlay(
              state = ui,
              onHangup = vm::hangup,
              onToggleMute = vm::toggleMute,
              onSetAudioOutput = vm::setAudioOutput,
              onDialPadPress = vm::onDialPadPress,
            )
          }
        }
      }
    }
  }

  private fun hasPermission(permission: String): Boolean {
    return ContextCompat.checkSelfPermission(this, permission) == android.content.pm.PackageManager.PERMISSION_GRANTED
  }
}

@Composable
private fun LoginScreen(
  state: UiState,
  onServer: (String) -> Unit,
  onUsername: (String) -> Unit,
  onPassword: (String) -> Unit,
  onLogin: () -> Unit,
) {
  Column(
    modifier = Modifier
      .fillMaxSize()
      .padding(20.dp),
    verticalArrangement = Arrangement.Center,
    horizontalAlignment = Alignment.CenterHorizontally,
  ) {
    Text("CallFXO", style = MaterialTheme.typography.headlineMedium)
    Spacer(Modifier.height(12.dp))
    OutlinedTextField(
      modifier = Modifier.fillMaxWidth(),
      value = state.serverAddr,
      onValueChange = onServer,
      label = { Text("Server address") },
      singleLine = true,
    )
    Spacer(Modifier.height(8.dp))
    OutlinedTextField(
      modifier = Modifier.fillMaxWidth(),
      value = state.usernameInput,
      onValueChange = onUsername,
      label = { Text("Username") },
      singleLine = true,
    )
    Spacer(Modifier.height(8.dp))
    OutlinedTextField(
      modifier = Modifier.fillMaxWidth(),
      value = state.passwordInput,
      onValueChange = onPassword,
      label = { Text("Password") },
      visualTransformation = PasswordVisualTransformation(),
      singleLine = true,
    )
    Spacer(Modifier.height(10.dp))
    Button(onClick = onLogin, enabled = !state.loading, modifier = Modifier.fillMaxWidth()) {
      Text(if (state.loading) "Signing in..." else "Sign In")
    }
    if (!state.loginError.isNullOrBlank()) {
      Spacer(Modifier.height(8.dp))
      Text(state.loginError ?: "", color = MaterialTheme.colorScheme.error)
    }
  }
}

@Composable
private fun MainScreen(
  state: UiState,
  onLogout: () -> Unit,
  onExit: () -> Unit,
  onRefresh: () -> Unit,
  onSelectBox: (Long) -> Unit,
  onDialPadPress: (String) -> Unit,
  onDialBackspace: () -> Unit,
  onDial: () -> Unit,
  onSearch: (String) -> Unit,
  onLoadDeviceContacts: () -> Unit,
  onContactClick: (String) -> Unit,
  onPrevLog: () -> Unit,
  onNextLog: () -> Unit,
  onLogClick: (String) -> Unit,
  onPickRingtone: () -> Unit,
  onSetRingtoneVolume: (Float) -> Unit,
) {
  var tab by remember { mutableIntStateOf(0) }

  Column(modifier = Modifier.fillMaxSize().padding(12.dp)) {
    Row(verticalAlignment = Alignment.CenterVertically) {
      Text(
        text = "${state.username} (${if (state.role.isBlank()) "user" else state.role})",
        modifier = Modifier.weight(1f),
        fontWeight = FontWeight.Bold,
      )
      TextButton(onClick = onRefresh) { Text("Refresh") }
      TextButton(onClick = onLogout) { Text("Logout") }
      TextButton(onClick = onExit) { Text("Exit") }
    }

    Spacer(Modifier.height(6.dp))
    Text("WS: ${if (state.wsConnected) "Connected" else "Disconnected"}")
    Text("Call: ${state.callStatus}")

    Spacer(Modifier.height(10.dp))
    TabRow(selectedTabIndex = tab) {
      Tab(selected = tab == 0, onClick = { tab = 0 }, text = { Text("Dial") })
      Tab(selected = tab == 1, onClick = { tab = 1 }, text = { Text("Contacts") })
      Tab(selected = tab == 2, onClick = { tab = 2 }, text = { Text("Logs") })
      Tab(selected = tab == 3, onClick = { tab = 3 }, text = { Text("Settings") })
    }

    Spacer(Modifier.height(10.dp))
    when (tab) {
      0 -> DialTab(
        state = state,
        onSelectBox = onSelectBox,
        onDialPadPress = onDialPadPress,
        onDialBackspace = onDialBackspace,
        onDial = onDial,
      )
      1 -> ContactsTab(
        state = state,
        onSearch = onSearch,
        onLoadDeviceContacts = onLoadDeviceContacts,
        onContactClick = onContactClick,
      )
      2 -> LogsTab(
        state = state,
        onPrevLog = onPrevLog,
        onNextLog = onNextLog,
        onLogClick = onLogClick,
      )
      else -> SettingsTab(
        state = state,
        onPickRingtone = onPickRingtone,
        onSetRingtoneVolume = onSetRingtoneVolume,
      )
    }
  }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DialTab(
  state: UiState,
  onSelectBox: (Long) -> Unit,
  onDialPadPress: (String) -> Unit,
  onDialBackspace: () -> Unit,
  onDial: () -> Unit,
) {
  val selected = state.boxes.firstOrNull { it.id == state.selectedBoxId }
  val dialEnabled = selected != null && selected.online && !selected.inUse && state.dialNumber.isNotBlank() && !state.callBusy
  var lineMenuExpanded by remember { mutableStateOf(false) }
  Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState())) {

    if (state.boxes.isEmpty()) {
      Text("No FXO lines available", color = MaterialTheme.colorScheme.onSurfaceVariant)
    } else {
      ExposedDropdownMenuBox(
        expanded = lineMenuExpanded,
        onExpandedChange = { lineMenuExpanded = !lineMenuExpanded },
      ) {
        val selectedLabel = selected?.name ?: "Select a line"
        OutlinedTextField(
          value = selectedLabel,
          onValueChange = {},
          readOnly = true,
          trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = lineMenuExpanded) },
          modifier = Modifier
            .fillMaxWidth()
            .menuAnchor(),
        )
        ExposedDropdownMenu(
          expanded = lineMenuExpanded,
          onDismissRequest = { lineMenuExpanded = false },
        ) {
          state.boxes.forEach { box ->
            val status = when {
              box.inUse -> "In use"
              box.online -> "Ready"
              else -> "Offline"
            }
            DropdownMenuItem(
              text = {
                Column {
                  Text(box.name)
                  Text(status, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
              },
              onClick = {
                onSelectBox(box.id)
                lineMenuExpanded = false
              },
            )
          }
        }
      }
    }

    val selectedStatus = when {
      selected == null -> "Pick a line before dialing"
      selected.inUse -> "Selected line is busy"
      selected.online -> "Selected line is ready"
      else -> "Selected line is offline"
    }

    Spacer(Modifier.height(12.dp))

    // Phone number display with backspace
    Row(
      modifier = Modifier.fillMaxWidth(),
      verticalAlignment = Alignment.CenterVertically,
    ) {
      Text(
        text = state.dialNumber.ifEmpty { "Enter number" },
        modifier = Modifier.weight(1f).padding(horizontal = 16.dp, vertical = 12.dp),
        style = MaterialTheme.typography.headlineMedium,
        color = if (state.dialNumber.isEmpty()) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
        textAlign = TextAlign.Center,
      )
      if (state.dialNumber.isNotEmpty()) {
        IconButton(onClick = onDialBackspace) {
          Icon(Icons.Filled.Backspace, contentDescription = "Backspace")
        }
      }
    }

    Spacer(Modifier.height(4.dp))
    Text(
      text = selectedStatus,
      modifier = Modifier.fillMaxWidth(),
      color = MaterialTheme.colorScheme.onSurfaceVariant,
      textAlign = TextAlign.Center,
      style = MaterialTheme.typography.bodySmall,
    )

    Spacer(Modifier.height(12.dp))

    // Dial pad grid
    val padKeys = listOf(
      listOf("1", "2", "3"),
      listOf("4", "5", "6"),
      listOf("7", "8", "9"),
      listOf("*", "0", "#"),
    )
    padKeys.forEach { row ->
      Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceEvenly,
      ) {
        row.forEach { key ->
          Box(
            modifier = Modifier
              .size(72.dp)
              .padding(4.dp)
              .background(MaterialTheme.colorScheme.surfaceVariant, CircleShape)
              .clickable(enabled = !state.callBusy) { onDialPadPress(key) },
            contentAlignment = Alignment.Center,
          ) {
            Text(
              text = key,
              style = MaterialTheme.typography.headlineSmall,
              fontWeight = FontWeight.Bold,
            )
          }
        }
      }
    }

    Spacer(Modifier.height(16.dp))

    // Call button
    Row(
      modifier = Modifier.fillMaxWidth(),
      horizontalArrangement = Arrangement.Center,
      verticalAlignment = Alignment.CenterVertically,
    ) {
      IconButton(
        onClick = onDial,
        enabled = dialEnabled,
        modifier = Modifier.size(72.dp),
        colors = IconButtonDefaults.iconButtonColors(
          containerColor = Color(0xFF1B7C3A),
          contentColor = Color.White,
          disabledContainerColor = MaterialTheme.colorScheme.surfaceVariant,
        ),
      ) {
        Icon(Icons.Filled.Call, contentDescription = "Dial", modifier = Modifier.size(34.dp))
      }
    }
  }
}

@Composable
private fun ContactsTab(
  state: UiState,
  onSearch: (String) -> Unit,
  onLoadDeviceContacts: () -> Unit,
  onContactClick: (String) -> Unit,
) {
  Column(modifier = Modifier.fillMaxSize()) {
    OutlinedTextField(
      modifier = Modifier.fillMaxWidth(),
      value = state.search,
      onValueChange = onSearch,
      label = { Text("Search name or number") },
      singleLine = true,
    )

    Spacer(Modifier.height(8.dp))
    Row {
      Button(onClick = { onSearch(state.search) }) { Text("Refresh Server") }
      Spacer(Modifier.width(8.dp))
      Button(onClick = onLoadDeviceContacts) { Text("Load Device Contacts") }
    }

    Spacer(Modifier.height(8.dp))
    LazyColumn(modifier = Modifier.fillMaxSize()) {
      items(state.contacts, key = { it.id }) { c ->
        Card(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp)) {
          Row(
            modifier = Modifier.fillMaxWidth().padding(10.dp),
            verticalAlignment = Alignment.CenterVertically,
          ) {
            Column(modifier = Modifier.weight(1f)) {
              Text(if (c.name.isBlank()) c.number else c.name, fontWeight = FontWeight.SemiBold)
              Text(c.number)
              Text(if (c.source == ContactSource.SERVER) "server" else "device")
            }
            TextButton(onClick = { onContactClick(c.number) }) { Text("Use") }
          }
        }
      }
    }
  }
}

@Composable
private fun LogsTab(
  state: UiState,
  onPrevLog: () -> Unit,
  onNextLog: () -> Unit,
  onLogClick: (String) -> Unit,
) {
  Column(modifier = Modifier.fillMaxSize()) {
    Row(verticalAlignment = Alignment.CenterVertically) {
      Button(onClick = onPrevLog, enabled = state.logPage > 1) { Text("Prev") }
      Spacer(Modifier.width(10.dp))
      Text("${state.logPage} / ${state.logTotalPages}")
      Spacer(Modifier.width(10.dp))
      Button(onClick = onNextLog, enabled = state.logPage < state.logTotalPages) { Text("Next") }
    }

    Spacer(Modifier.height(8.dp))
    LazyColumn(modifier = Modifier.fillMaxSize()) {
      items(state.callLogs, key = { it.id }) { l ->
        Card(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp)) {
          Column(modifier = Modifier.fillMaxWidth().padding(10.dp)) {
            Text(l.startedAt, fontWeight = FontWeight.SemiBold)
            Text("${l.fxoBoxName}  ${l.status}")
            Text(l.reason)
            TextButton(onClick = { onLogClick(l.number) }) { Text(l.number) }
          }
        }
        HorizontalDivider()
      }
    }
  }
}

@Composable
private fun SettingsTab(
  state: UiState,
  onPickRingtone: () -> Unit,
  onSetRingtoneVolume: (Float) -> Unit,
) {
  Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState())) {
    Text("Incoming call ringtone", style = MaterialTheme.typography.titleMedium)
    Spacer(Modifier.height(8.dp))
    Text(state.ringtoneUri.ifBlank { "System default" })
    Spacer(Modifier.height(8.dp))
    Button(onClick = onPickRingtone) { Text("Choose Ringtone") }

    Spacer(Modifier.height(20.dp))
    Text("Ringtone volume", style = MaterialTheme.typography.titleMedium)
    Spacer(Modifier.height(8.dp))
    Slider(
      value = state.ringtoneVolume,
      onValueChange = onSetRingtoneVolume,
      valueRange = 0f..1f,
    )
    Text("${(state.ringtoneVolume * 100).toInt()}%")
  }
}

@Composable
private fun IncomingCallOverlay(
  state: UiState,
  onAccept: () -> Unit,
  onReject: () -> Unit,
) {
  val incoming = state.incomingCall ?: return
  Box(
    modifier = Modifier
      .fillMaxSize()
      .background(
        Brush.verticalGradient(
          colors = listOf(Color(0xFF0D1B2A), Color(0xFF14213D), Color(0xFF1F2937))
        )
      ),
  ) {
    Column(
      modifier = Modifier
        .fillMaxSize()
        .padding(24.dp),
      horizontalAlignment = Alignment.CenterHorizontally,
      verticalArrangement = Arrangement.SpaceBetween,
    ) {
      // Top section - caller info
      Column(
        modifier = Modifier.weight(1f),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
      ) {
        Text(
          text = "Incoming Call",
          color = Color.White.copy(alpha = 0.7f),
          style = MaterialTheme.typography.titleMedium,
        )
        Spacer(Modifier.height(24.dp))
        AvatarBadge(label = displayCaller(incoming), dark = true)
        Spacer(Modifier.height(20.dp))
        Text(
          displayCaller(incoming),
          style = MaterialTheme.typography.headlineLarge,
          color = Color.White,
          textAlign = TextAlign.Center,
        )
        if (incoming.boxName.isNotBlank()) {
          Spacer(Modifier.height(6.dp))
          Text(
            incoming.boxName,
            color = Color.White.copy(alpha = 0.7f),
            style = MaterialTheme.typography.bodyLarge,
          )
        }
        if (incoming.remoteNumber.isNotBlank() && incoming.remoteNumber != displayCaller(incoming)) {
          Spacer(Modifier.height(4.dp))
          Text(
            incoming.remoteNumber,
            color = Color.White.copy(alpha = 0.5f),
            style = MaterialTheme.typography.bodyMedium,
          )
        }
      }

      // Bottom section - action buttons
      Row(
        modifier = Modifier
          .fillMaxWidth()
          .padding(bottom = 48.dp),
        horizontalArrangement = Arrangement.SpaceEvenly,
        verticalAlignment = Alignment.CenterVertically,
      ) {
        // Reject button
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
          IconButton(
            onClick = onReject,
            modifier = Modifier.size(72.dp),
            colors = IconButtonDefaults.iconButtonColors(
              containerColor = Color(0xFFC62828),
              contentColor = Color.White,
            ),
          ) {
            Icon(Icons.Filled.CallEnd, contentDescription = "Reject", modifier = Modifier.size(36.dp))
          }
          Spacer(Modifier.height(8.dp))
          Text("Reject", color = Color.White.copy(alpha = 0.7f), style = MaterialTheme.typography.bodySmall)
        }

        // Accept button
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
          IconButton(
            onClick = onAccept,
            modifier = Modifier.size(72.dp),
            colors = IconButtonDefaults.iconButtonColors(
              containerColor = Color(0xFF2E7D32),
              contentColor = Color.White,
            ),
          ) {
            Icon(Icons.Filled.Call, contentDescription = "Answer", modifier = Modifier.size(36.dp))
          }
          Spacer(Modifier.height(8.dp))
          Text("Answer", color = Color.White.copy(alpha = 0.7f), style = MaterialTheme.typography.bodySmall)
        }
      }
    }
  }
}

@Composable
private fun ActiveCallOverlay(
  state: UiState,
  onHangup: () -> Unit,
  onToggleMute: () -> Unit,
  onSetAudioOutput: (AudioOutput) -> Unit,
  onDialPadPress: (String) -> Unit,
) {
  var showKeypad by remember { mutableStateOf(false) }
  Box(
    modifier = Modifier
      .fillMaxSize()
      .background(
        Brush.verticalGradient(
          colors = listOf(Color(0xFF0D1B2A), Color(0xFF14213D), Color(0xFF1F2937))
        )
      ),
  ) {
    Column(
      modifier = Modifier
        .fillMaxSize()
        .padding(24.dp),
      horizontalAlignment = Alignment.CenterHorizontally,
      verticalArrangement = Arrangement.SpaceBetween,
    ) {
      // Top section - call info
      Column(
        modifier = Modifier.weight(1f),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
      ) {
        Text(
          text = when {
            state.inCall -> "Call in progress"
            state.callDirection == "incoming" -> "Connecting"
            else -> "Dialing"
          },
          color = Color.White.copy(alpha = 0.7f),
          style = MaterialTheme.typography.titleMedium,
        )
        Spacer(Modifier.height(24.dp))
        AvatarBadge(label = state.callDisplayName.ifBlank { "CallFXO" }, dark = true)
        Spacer(Modifier.height(20.dp))
        Text(
          state.callDisplayName.ifBlank { "Unknown" },
          style = MaterialTheme.typography.headlineLarge,
          color = Color.White,
          textAlign = TextAlign.Center,
        )
        if (state.callDisplaySubtitle.isNotBlank()) {
          Spacer(Modifier.height(6.dp))
          Text(state.callDisplaySubtitle, color = Color.White.copy(alpha = 0.7f))
        }
        Spacer(Modifier.height(12.dp))
        CallDurationText(connectedAtMs = state.callConnectedAtMs, fallback = state.callStatus)
      }

      // Bottom section - action buttons grid + hangup
      Column(
        modifier = Modifier.padding(bottom = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
      ) {
        // 2x2 grid of action buttons
        Row(
          modifier = Modifier.fillMaxWidth(),
          horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
          // Mute button
          CallActionButton(
            icon = if (state.muted) Icons.Filled.MicOff else Icons.Filled.Mic,
            label = if (state.muted) "Unmute" else "Mute",
            active = state.muted,
            onClick = onToggleMute,
          )
          // Speaker button
          CallActionButton(
            icon = Icons.Filled.VolumeUp,
            label = "Speaker",
            active = state.audioOutput == AudioOutput.SPEAKER,
            onClick = { onSetAudioOutput(AudioOutput.SPEAKER) },
          )
        }

        Spacer(Modifier.height(16.dp))

        Row(
          modifier = Modifier.fillMaxWidth(),
          horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
          // Earpiece button
          CallActionButton(
            icon = Icons.Filled.Hearing,
            label = "Earpiece",
            active = state.audioOutput == AudioOutput.EARPIECE,
            onClick = { onSetAudioOutput(AudioOutput.EARPIECE) },
          )
          // Bluetooth button
          CallActionButton(
            icon = Icons.Filled.BluetoothAudio,
            label = "Bluetooth",
            active = state.audioOutput == AudioOutput.BLUETOOTH,
            enabled = state.bluetoothAvailable,
            onClick = { onSetAudioOutput(AudioOutput.BLUETOOTH) },
          )
          // Keypad button
          CallActionButton(
            icon = Icons.Filled.Dialpad,
            label = "Keypad",
            active = showKeypad,
            onClick = { showKeypad = !showKeypad },
          )
        }

        // Inline DTMF keypad
        if (showKeypad) {
          Spacer(Modifier.height(16.dp))
          val dtmfKeys = listOf(
            listOf("1", "2", "3"),
            listOf("4", "5", "6"),
            listOf("7", "8", "9"),
            listOf("*", "0", "#"),
          )
          dtmfKeys.forEach { row ->
            Row(
              modifier = Modifier.fillMaxWidth(),
              horizontalArrangement = Arrangement.SpaceEvenly,
            ) {
              row.forEach { key ->
                TextButton(
                  onClick = { onDialPadPress(key) },
                  modifier = Modifier.size(64.dp),
                ) {
                  Text(key, color = Color.White, style = MaterialTheme.typography.headlineMedium)
                }
              }
            }
          }
        }

        Spacer(Modifier.height(32.dp))

        // Hangup button
        IconButton(
          onClick = onHangup,
          modifier = Modifier.size(80.dp),
          colors = IconButtonDefaults.iconButtonColors(
            containerColor = Color(0xFFC62828),
            contentColor = Color.White,
          ),
        ) {
          Icon(Icons.Filled.CallEnd, contentDescription = "Hangup", modifier = Modifier.size(40.dp))
        }
      }
    }
  }
}

@Composable
private fun CallActionButton(
  icon: androidx.compose.ui.graphics.vector.ImageVector,
  label: String,
  active: Boolean = false,
  enabled: Boolean = true,
  onClick: () -> Unit,
) {
  val bgColor = when {
    !enabled -> Color.White.copy(alpha = 0.05f)
    active -> Color.White.copy(alpha = 0.3f)
    else -> Color.White.copy(alpha = 0.12f)
  }
  val contentColor = when {
    !enabled -> Color.White.copy(alpha = 0.3f)
    else -> Color.White
  }
  Column(
    horizontalAlignment = Alignment.CenterHorizontally,
  ) {
    IconButton(
      onClick = onClick,
      enabled = enabled,
      modifier = Modifier.size(64.dp),
      colors = IconButtonDefaults.iconButtonColors(
        containerColor = bgColor,
        contentColor = contentColor,
        disabledContainerColor = bgColor,
        disabledContentColor = contentColor,
      ),
    ) {
      Icon(icon, contentDescription = label, modifier = Modifier.size(28.dp))
    }
    Spacer(Modifier.height(6.dp))
    Text(
      label,
      color = contentColor,
      style = MaterialTheme.typography.bodySmall,
    )
  }
}

@Composable
private fun CallDurationText(connectedAtMs: Long, fallback: String) {
  var now by remember(connectedAtMs) { mutableStateOf(System.currentTimeMillis()) }
  LaunchedEffect(connectedAtMs) {
    if (connectedAtMs <= 0L) return@LaunchedEffect
    while (true) {
      now = System.currentTimeMillis()
      delay(1000)
    }
  }
  val label = if (connectedAtMs <= 0L) {
    fallback
  } else {
    formatDuration(((now - connectedAtMs).coerceAtLeast(0L)) / 1000L)
  }
  Text(label, style = MaterialTheme.typography.headlineSmall, color = Color.White)
}

@Composable
private fun AvatarBadge(label: String, dark: Boolean = false) {
  val bg = if (dark) Color.White.copy(alpha = 0.16f) else MaterialTheme.colorScheme.primaryContainer
  val fg = if (dark) Color.White else MaterialTheme.colorScheme.onPrimaryContainer
  Box(
    modifier = Modifier
      .size(84.dp)
      .background(bg, CircleShape),
    contentAlignment = Alignment.Center,
  ) {
    Text(
      text = label.firstOrNull()?.uppercase() ?: "?",
      style = MaterialTheme.typography.headlineMedium,
      color = fg,
    )
  }
}

private fun displayCaller(incoming: PendingIncomingCall): String {
  return when {
    incoming.callerId.isNotBlank() -> incoming.callerId
    incoming.remoteNumber.isNotBlank() -> incoming.remoteNumber
    else -> "Unknown"
  }
}

private fun formatDuration(totalSeconds: Long): String {
  val hours = totalSeconds / 3600
  val minutes = (totalSeconds % 3600) / 60
  val seconds = totalSeconds % 60
  return if (hours > 0) {
    "%02d:%02d:%02d".format(hours, minutes, seconds)
  } else {
    "%02d:%02d".format(minutes, seconds)
  }
}
