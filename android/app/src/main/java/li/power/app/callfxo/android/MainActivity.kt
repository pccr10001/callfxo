package li.power.app.callfxo.android

import android.Manifest
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BluetoothAudio
import androidx.compose.material.icons.filled.Call
import androidx.compose.material.icons.filled.CallEnd
import androidx.compose.material.icons.filled.Hearing
import androidx.compose.material.icons.filled.VolumeUp
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlin.system.exitProcess
import li.power.app.callfxo.android.call.AudioOutput

class MainActivity : ComponentActivity() {
  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)

    setContent {
      MaterialTheme {
        val vm: MainViewModel = viewModel()
        val ui by vm.ui.collectAsStateWithLifecycle()
        val lifecycleOwner = LocalLifecycleOwner.current

        var pendingDial by remember { mutableStateOf(false) }
        var pendingLoadDeviceContacts by remember { mutableStateOf(false) }

        val audioPermLauncher = androidx.activity.compose.rememberLauncherForActivityResult(
          ActivityResultContracts.RequestPermission()
        ) { granted ->
          if (granted && pendingDial) vm.dial()
          pendingDial = false
        }

        val contactsPermLauncher = androidx.activity.compose.rememberLauncherForActivityResult(
          ActivityResultContracts.RequestPermission()
        ) { granted ->
          if (granted && pendingLoadDeviceContacts) vm.loadDeviceContacts()
          pendingLoadDeviceContacts = false
        }

        val notifPermLauncher = androidx.activity.compose.rememberLauncherForActivityResult(
          ActivityResultContracts.RequestPermission()
        ) { }

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
          if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (!hasPermission(Manifest.permission.POST_NOTIFICATIONS)) {
              notifPermLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
          }
        }

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
            onDialNumber = vm::onDialNumberChange,
            onDialPadPress = vm::onDialPadPress,
            onDial = {
              if (hasPermission(Manifest.permission.RECORD_AUDIO)) {
                vm.dial()
              } else {
                pendingDial = true
                audioPermLauncher.launch(Manifest.permission.RECORD_AUDIO)
              }
            },
            onHangup = vm::hangup,
            onSetAudioOutput = vm::setAudioOutput,
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
          )
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
  onDialNumber: (String) -> Unit,
  onDialPadPress: (String) -> Unit,
  onDial: () -> Unit,
  onHangup: () -> Unit,
  onSetAudioOutput: (AudioOutput) -> Unit,
  onSearch: (String) -> Unit,
  onLoadDeviceContacts: () -> Unit,
  onContactClick: (String) -> Unit,
  onPrevLog: () -> Unit,
  onNextLog: () -> Unit,
  onLogClick: (String) -> Unit,
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
    }

    Spacer(Modifier.height(10.dp))
    when (tab) {
      0 -> DialTab(
        state = state,
        onSelectBox = onSelectBox,
        onDialNumber = onDialNumber,
        onDialPadPress = onDialPadPress,
        onDial = onDial,
        onHangup = onHangup,
        onSetAudioOutput = onSetAudioOutput,
      )
      1 -> ContactsTab(
        state = state,
        onSearch = onSearch,
        onLoadDeviceContacts = onLoadDeviceContacts,
        onContactClick = { number ->
          onContactClick(number)
          tab = 0
        },
      )
      else -> LogsTab(
        state = state,
        onPrevLog = onPrevLog,
        onNextLog = onNextLog,
        onLogClick = { number ->
          onLogClick(number)
          tab = 0
        },
      )
    }
  }
}

@Composable
private fun DialTab(
  state: UiState,
  onSelectBox: (Long) -> Unit,
  onDialNumber: (String) -> Unit,
  onDialPadPress: (String) -> Unit,
  onDial: () -> Unit,
  onHangup: () -> Unit,
  onSetAudioOutput: (AudioOutput) -> Unit,
) {
  var expanded by remember { mutableStateOf(false) }
  var outputMenuExpanded by remember { mutableStateOf(false) }
  val selected = state.boxes.firstOrNull { it.id == state.selectedBoxId }
  val selectedBoxText = selected?.let { "${it.name} (${it.sipUsername})" } ?: "Select FXO"
  val dialEnabled = selected != null &&
    selected.online &&
    !selected.inUse &&
    state.dialNumber.trim().isNotBlank() &&
    !state.callBusy
  val hangupEnabled = state.callBusy || state.inCall

  Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState())) {
    Row(
      modifier = Modifier.fillMaxWidth(),
      horizontalArrangement = Arrangement.End,
      verticalAlignment = Alignment.CenterVertically,
    ) {
      Box {
        TextButton(onClick = { outputMenuExpanded = true }) {
          val label = when (state.audioOutput) {
            AudioOutput.EARPIECE -> "Earpiece"
            AudioOutput.SPEAKER -> "Speaker"
            AudioOutput.BLUETOOTH -> "Bluetooth"
          }
          val icon = when (state.audioOutput) {
            AudioOutput.EARPIECE -> Icons.Filled.Hearing
            AudioOutput.SPEAKER -> Icons.Filled.VolumeUp
            AudioOutput.BLUETOOTH -> Icons.Filled.BluetoothAudio
          }
          Icon(icon, contentDescription = label)
          Spacer(Modifier.width(6.dp))
          Text(label)
        }
        DropdownMenu(expanded = outputMenuExpanded, onDismissRequest = { outputMenuExpanded = false }) {
          DropdownMenuItem(
            text = { Text("Earpiece") },
            onClick = {
              onSetAudioOutput(AudioOutput.EARPIECE)
              outputMenuExpanded = false
            },
          )
          DropdownMenuItem(
            text = { Text("Speaker") },
            onClick = {
              onSetAudioOutput(AudioOutput.SPEAKER)
              outputMenuExpanded = false
            },
          )
          DropdownMenuItem(
            text = { Text("Bluetooth") },
            enabled = state.bluetoothAvailable,
            onClick = {
              onSetAudioOutput(AudioOutput.BLUETOOTH)
              outputMenuExpanded = false
            },
          )
        }
      }
    }

    OutlinedTextField(
      modifier = Modifier.fillMaxWidth().clickable { expanded = true },
      value = selectedBoxText,
      onValueChange = {},
      label = { Text("FXO Box") },
      readOnly = true,
      singleLine = true,
    )
    DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
      state.boxes.forEach { b ->
        val status = when {
          b.inUse -> "in use"
          b.online -> "online"
          else -> "offline"
        }
        DropdownMenuItem(
          text = { Text("${b.name} ($status)") },
          onClick = {
            onSelectBox(b.id)
            expanded = false
          }
        )
      }
    }

    Spacer(Modifier.height(8.dp))
    OutlinedTextField(
      modifier = Modifier.fillMaxWidth(),
      value = state.dialNumber,
      onValueChange = onDialNumber,
      label = { Text("Phone number") },
      readOnly = true,
      singleLine = true,
    )

    Spacer(Modifier.height(10.dp))
    val keys = listOf("1","2","3","4","5","6","7","8","9","*","0","#")
    for (r in 0 until 4) {
      Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        for (c in 0 until 3) {
          val idx = r * 3 + c
          val d = keys[idx]
          Button(onClick = { onDialPadPress(d) }, modifier = Modifier.weight(1f)) { Text(d) }
        }
      }
      Spacer(Modifier.height(8.dp))
    }

    Spacer(Modifier.height(6.dp))
    Row(
      modifier = Modifier.fillMaxWidth(),
      horizontalArrangement = Arrangement.SpaceEvenly,
      verticalAlignment = Alignment.CenterVertically,
    ) {
      IconButton(
        modifier = Modifier.size(80.dp),
        onClick = onDial,
        enabled = dialEnabled,
        colors = IconButtonDefaults.iconButtonColors(
          containerColor = Color(0xFF2E7D32),
          contentColor = Color.White,
          disabledContainerColor = MaterialTheme.colorScheme.surfaceVariant,
          disabledContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        ),
      ) {
        Icon(Icons.Filled.Call, contentDescription = "Dial", modifier = Modifier.size(36.dp))
      }
      IconButton(
        modifier = Modifier.size(80.dp),
        onClick = onHangup,
        enabled = hangupEnabled,
        colors = IconButtonDefaults.iconButtonColors(
          containerColor = Color(0xFFC62828),
          contentColor = Color.White,
          disabledContainerColor = MaterialTheme.colorScheme.surfaceVariant,
          disabledContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        ),
      ) {
        Icon(Icons.Filled.CallEnd, contentDescription = "Hangup", modifier = Modifier.size(36.dp))
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
              Text(if (c.source == li.power.app.callfxo.android.data.ContactSource.SERVER) "server" else "device")
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
