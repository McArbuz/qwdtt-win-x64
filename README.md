Описание Сгенерировано AI. 

Если кратко, от человека:

Win x64 клиент под qwdtt. Деплой доступен только с приложения на телефон, RAW подключение не перенесено.

Качаем https://www.wireguard.com/install/?spm=a2ty_o01.29997173.0.0.6e26c921Peecls

Качаем https://go.dev/doc/install ~1.27

Установка: , пошагово.
```powershell
git clone https://github.com/McArbuz/qwdtt-win-x64.git
```
```powershell
cd ваш-путь/qwdtt-win-x64
```
```powershell
cd go_client
```
```powershell
go mod tidy   # обязательно: обновляет go.sum (иначе "missing go.sum entry")
```
```powershell
go build -trimpath -ldflags="-checklinkname=0 -s -w" -o qwdtt.exe .
```
```powershell
cd qwdtt-win-x64\panel
```
```powershell
go mod tidy
```
```powershell
go build -trimpath -ldflags "-s -w -H windowsgui" -o qwdtt-panel.exe .
```
перенесите файл qwdtt-panel.exe в qwdtt-win-x64\go_client (Реком.), или укажите в панели адрес к нему
Панель в qwdtt-win-x64\go_client файл называется "qwdtt-panel.exe"
Запустите qwdtt-panel.exe от имени адм.




Описание
# qWDTT for Windows

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8)
![Platform](https://img.shields.io/badge/Windows-10%2F11%20x64-blue)

> Windows-клиент для **qWDTT** — туннеля через TURN-инфраструктуру звонков VK до собственного VPS.
> Форк [SpaceNeuroX/proxy-turn-vk-android](https://github.com/SpaceNeuroX/proxy-turn-vk-android):
> Go-ядро `go_client` собирается под Windows + собственная GUI-панель.
> **Протокол и сервер qWDTT не изменены** — используется обычный сервер qWDTT.

## Родословная проекта

- [WDTT](https://github.com/amurcanov/proxy-turn-vk-android) — оригинальный Android-клиент (amurcanov)
- [qWDTT](https://github.com/SpaceNeuroX/proxy-turn-vk-android) — самостоятельное развитие WDTT (SpaceNeuroX)
- **этот форк** — нативный клиент qWDTT для Windows: ядро `qwdtt.exe` + панель `qwdtt-panel.exe`

## Что изменено в этом форке

- ✅ Сборка Go-ядра qWDTT (`go_client/`) под Windows x64 — `qwdtt.exe`
- ✅ Собственная GUI-панель для Windows (`panel/`) — `qwdtt-panel.exe` (без консольного окна)
- ✅ Исправления сборки: актуализирован `go.sum`, добавлен флаг `-checklinkname=0`
- [добавь свои изменения: профили, настройки, фиксы…]

Android-приложение и серверная часть из апстрима не тронуты.

## Как это работает

```
VPN-режим:  приложения → WireGuard (127.0.0.1:9000) → qwdtt.exe → DTLS/TURN (VK) → VPS qWDTT → интернет
SOCKS-режим: приложения → SOCKS5 (127.0.0.1:1080) → qwdtt.exe → DTLS/TURN (VK) → VPS qWDTT → интернет
```

Для сети соединение выглядит как медиатрафик звонка WebRTC (маскировка `audio`/`video`).

## Основные флаги ядра

| Флаг | Назначение |
|------|------------|
| `-peer "VPS_IP:56000"` | адрес сервера qWDTT |
| `-password "..."` | пароль клиента (выдаётся на сервере) |
| `-vk "хеш1,хеш2,..."` | хеши звонков VK (через запятую) |
| `-obfs audio\|video` | маскировка под аудио/видеозвонок |
| `-turn-tcp` | принудительно TURN по TCP (спасает, если провайдер режет UDP) |
| `-mode vpn\|socks\|rawtun` | режим туннеля |
| `-listen 127.0.0.1:9000` | локальный адрес для WireGuard (vpn) |
| `-socks 127.0.0.1:1080` | локальный SOCKS5 (socks) |
| `-n 9` | число потоков/воркеров |
| `-go-dns yandex` | DNS для резолва VK (например, Яндекс DNS) |
| `-device-id "my-pc"` | ID устройства (у сервера есть привязка устройств) |

> ⚠️ Выдавай ПК **отдельный device-id и пароль** в админке сервера/Telegram-боте,
> чтобы не «выкинуть» телефон, который уже подключён с тем же ID.

## Требования

### Для запуска

- **Windows 10/11 x64**
- **VPN-режим:** установите [WireGuard for Windows](https://www.wireguard.com/install/) —
  ядро слушает локальный UDP `127.0.0.1:9000`, а WireGuard подключается к нему
  (на Android эту роль выполняет VpnService, на Windows — приложение WireGuard)
- **SOCKS-режим:** WireGuard не нужен — достаточно прописать прокси в приложениях
- VPS с Linux и установленным **сервером qWDTT** (деплой из Android-приложения
  qWDTT: вкладка «Серверы», либо `app/src/main/assets/deploy.sh`), открытые порты:

| Порт | Протокол | Назначение |
|------|----------|------------|
| 22 | TCP | SSH (деплой) |
| 56000 | UDP | DTLS-туннель |
| 56001 | UDP | WireGuard на сервере |
| 56003 | UDP | RAW-слушатель (опционально, для rawtun/OpenWrt) |

- VK-аккаунт с хешем звонка: VK → Сообщество → Звонок → «Пригласить» →
  скопировать ссылку вида `https://vk.com/call/join?hash=XXXX`

### Для сборки из исходников

| Компонент | Версия | Примечание |
|-----------|--------|------------|
| [Go](https://go.dev/dl/) | **≥ 1.26** (`go_client/go.mod`) | актуальная — 1.27.x |
| Git | любая | клонирование |

- **Android SDK / NDK / gomobile не нужны** — это только для Android-сборки (AAR)
- Компилятор C (CGO) не нужен — чистый Go
- Библиотеки ставятся **автоматически** командой `go mod tidy`, вручную ничего качать не надо

Ключевые Go-библиотеки (из `go.mod`):

| Библиотека | Назначение |
|------------|------------|
| `github.com/pion/dtls/v3`, `pion/turn/v5`, `pion/stun/v3` | DTLS-шифрование, TURN/STUN |
| `github.com/bogdanfinn/tls-client` + `fhttp` | HTTP-клиент с браузерным uTLS-fingerprint (VK API) |
| `github.com/armon/go-socks5` | SOCKS5-сервер |
| `golang.zx2c4.com/wireguard` (+ `wintun`, `gvisor`) | WireGuard-стек |
| `golang.org/x/crypto`, `x/net`, `x/sys` | криптография и сеть |
| `github.com/cbeuw/connutil`, `github.com/google/uuid` | утилиты |

## 🔧 Сборка

```powershell
git clone https://github.com/McArbuz/qwdtt-win-x64.git
cd qwdtt-win-x64

# Ядро (Windows x64)
cd go_client
go mod tidy   # обязательно: обновляет go.sum (иначе "missing go.sum entry")
go build -trimpath -ldflags="-checklinkname=0 -s -w" -o qwdtt.exe .

# GUI-панель (без консольного окна)
cd ..\panel
go mod tidy
go build -trimpath -ldflags "-s -w -H windowsgui" -o qwdtt-panel.exe .
```

> **Почему `-checklinkname=0`:** библиотека `bogdanfinn/tls-client` использует
> `go:linkname`, а Go ≥ 1.23 по умолчанию это запрещает. Без флага сборка упадёт.

## 🚀 Запуск

### VPN-режим (весь трафик Windows через туннель)

```powershell
.\qwdtt.exe -peer "VPS_IP:56000" -password "ВАШ_ПАРОЛЬ" `
  -vk "ХЕШ1,ХЕШ2" -obfs audio -turn-tcp `
  -mode vpn -listen 127.0.0.1:9000 -n 9 -go-dns yandex -device-id "my-pc"
```

1. Дождитесь получения TURN-кредов и запуска локального слушателя.
2. Откройте **WireGuard** → «Добавить туннель» → «Импортировать из файла» →
   выберите `wg-turn.conf` (появляется рядом с `qwdtt.exe`).
3. Проверьте, что `Endpoint = 127.0.0.1:9000` (не адрес VPS!).
4. Нажмите «Активировать» и проверьте IP на 2ip.ru — должен быть IP вашего VPS.

### SOCKS-режим (прокси для отдельных приложений)

```powershell
.\qwdtt.exe -peer "VPS_IP:56000" -password "ВАШ_ПАРОЛЬ" `
  -vk "ХЕШ1,ХЕШ2" -obfs video -turn-tcp `
  -mode socks -socks 127.0.0.1:1080 -n 9 -device-id "my-pc"
```

Далее укажите SOCKS5 `127.0.0.1:1080` в настройках приложений, браузера или Proxifier.

## ❓ Частые проблемы

| Симптом | Решение |
|---------|---------|
| `missing go.sum entry for module ...` | `go mod tidy` в каталоге модуля перед сборкой |
| ошибка про `go:linkname` при сборке | добавьте `-ldflags="-checklinkname=0 ..."` |
| провайдер режет/душит UDP | добавьте `-turn-tcp` |
| сервер «выкидывает» телефон при подключении ПК | отдельный `-device-id` и пароль для каждого устройства |
| SmartScreen ругается на exe | «Подробнее» → «Выполнить в любом случае» (бинарник не подписан) |

## Синхронизация с апстримом qWDTT

```powershell
git remote add upstream https://github.com/SpaceNeuroX/proxy-turn-vk-android.git
git fetch upstream
git merge upstream/master
```

## Авторы

- **McArbuz (https://github.com/McArbuz)** — автор Windows-клиента: порт ядра, GUI-панель, поддержка
- **Qwen (Alibaba Group)** — ИИ-ассистент: помощь с кодом, отладкой сборки и документацией

### Благодарности

- [SpaceNeuroX](https://github.com/SpaceNeuroX) — qWDTT (ядро, протокол, сервер)
- [amurcanov](https://github.com/amurcanov) — оригинальный WDTT

## Лицензия

[GNU GPL v3](LICENSE) — как и у qWDTT. Форк обязан сохранять лицензию GPL v3
и уведомления об авторских правах оригинальных авторов.

## Дисклеймер

Проект для личных и исследовательских целей. Ответственность за использование
и соблюдение правил сервисов и законодательства несёт пользователь. Как автор я совершенно за, если SpaceNeuroX возможно доработает напильником и зальет для людей себе и windows порт, или укажет ссылку.
