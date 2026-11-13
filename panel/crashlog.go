package main

import (
"fmt"
"os"
"path/filepath"
"runtime/debug"
"syscall"
"time"
)

func crashLogPath() string {
return filepath.Join(baseDir, "panel-crash.log")
}

// У GUI-приложения (-H windowsgui) нет консоли, поэтому паники рантайма
// исчезают бесследно. Перенаправляем stderr процесса в файл рядом с exe.
func installCrashLog() {
f, e := os.OpenFile(crashLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
if e != nil {
return
}
// STD_ERROR_HANDLE = -12 (как DWORD: 0xFFFFFFF4)
syscall.NewLazyDLL("kernel32.dll").NewProc("SetStdHandle").Call(uintptr(0xFFFFFFF4), f.Fd())
// f намеренно не закрываем — файл нужен до самого выхода
}

// Вызывается как defer crashGuard() в main: ловит панику, пишет стек и показывает окно.
func crashGuard() {
r := recover()
if r == nil {
return
}
msg := fmt.Sprintf("[%s] PANIC: %v\n%s\n",
time.Now().Format("2006-01-02 15:04:05"), r, debug.Stack())
if f, e := os.OpenFile(crashLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); e == nil {
f.WriteString(msg)
f.Close()
}
msgBox("qWDTT • PC — ошибка запуска",
fmt.Sprintf("Панель упала: %v\n\nПодробности: panel-crash.log рядом с exe.", r))
}