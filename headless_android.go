//go:build android

package main

// Worker（WorkManager。Java の HeadlessSync）が呼ぶ JNI の入口（Java_com_wails_app_HeadlessSync_*）を libwails.so に含める。
import _ "github.com/ousiassllc/hitomado/internal/androidbg"
