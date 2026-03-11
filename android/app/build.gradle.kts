plugins {
  id("com.android.application")
  id("org.jetbrains.kotlin.android")
  id("org.jetbrains.kotlin.plugin.compose")
  id("org.jetbrains.kotlin.plugin.serialization")
}

android {
  namespace = "li.power.app.callfxo.android"
  compileSdk = 36

  defaultConfig {
    applicationId = "li.power.app.callfxo.android"
    minSdk = 28
    targetSdk = 36
    versionCode = 1
    versionName = "1.0.0"
  }

  buildTypes {
    release {
      isMinifyEnabled = false
      proguardFiles(
        getDefaultProguardFile("proguard-android-optimize.txt"),
        "proguard-rules.pro"
      )
    }
  }

  compileOptions {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
  }

  kotlinOptions {
    jvmTarget = "17"
  }

  buildFeatures {
    compose = true
  }
}

dependencies {
  implementation("androidx.core:core-ktx:1.15.0")
  implementation("androidx.activity:activity-compose:1.10.1")
  implementation("androidx.compose.ui:ui:1.7.8")
  implementation("androidx.compose.ui:ui-tooling-preview:1.7.8")
  implementation("androidx.compose.material:material-icons-extended:1.7.8")
  implementation("androidx.compose.material3:material3:1.3.1")
  implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
  implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
  implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
  implementation("androidx.lifecycle:lifecycle-process:2.8.7")

  implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
  implementation("org.jetbrains.kotlinx:kotlinx-coroutines-play-services:1.8.1")
  implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")

  implementation(platform("com.google.firebase:firebase-bom:34.7.0"))
  implementation("com.google.firebase:firebase-messaging")

  implementation("com.squareup.okhttp3:okhttp:4.12.0")
  implementation("io.getstream:stream-webrtc-android:1.3.10")

  debugImplementation("androidx.compose.ui:ui-tooling:1.7.8")
}
