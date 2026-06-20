# Благодарности и происхождение кода

## [PWDTT](https://github.com/luminescq/PWDTT) — десктопный клиент

**Автор:** [luminescq](https://github.com/luminescq) — Copyright © 2026

Бинарники `pwdtt-client-linux-amd64` и `pwdtt-client-windows-amd64.exe` в [релизах WDTT](https://github.com/ildarmaga/wdtt/releases) — **модифицированная версия** проекта **PWDTT**, распространяемая на условиях **GNU GPL v3.0**.

| | |
|---|---|
| Оригинал | https://github.com/luminescq/PWDTT |
| Исходники модифицированной версии | https://github.com/ildarmaga/pwdtt-client |
| Справка по GPL | [release-assets/PWDTT-SOURCE.md](release-assets/PWDTT-SOURCE.md) |

## [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android)

**Автор:** [amurcanov](https://github.com/amurcanov)

WDTT-server основан на идее и реализации VPN-туннеля из проекта **proxy-turn-vk-android**:

> WireGuard-туннель через DTLS-медиарелей ВК TURN-серверов: трафик проходит от клиента к вашему VPS через промежуточные медиарелей-серверы ВК.

### Что взято и развито из proxy-turn-vk-android

| Область | Описание |
|---------|----------|
| Протокол | DTLS-вход, userspace WireGuard (`wdtt0`), WRAP/RTP-обфускация |
| Сервер | Логика `GETCONF`, выдача WG-конфига клиенту, базовая структура `server.go` |
| Клиент | Совместимость с Android/Go-клиентом из репозитория (порты, handshake) |

| Клиент | Совместимость с WDTT |
|--------|----------------------|
| [vk-turn-proxy-ios](https://github.com/anton48/vk-turn-proxy-ios) (iOS) | ✅ `wdtt://`, TURN override, TestFlight |
| [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) (Android) | ✅ базовый протокол DTLS + WG |

### Что добавлено в WDTT (этот репозиторий)

- Мультипользовательский режим, лимиты трафика и скорости (`tc`)
- Веб-панель `wdtt-panel` (UI в стиле 3x-ui)
- Интеграция Xray (маршрутизация RU/NL, redirect с `wdtt0`)
- Ссылки подключения `wdtt://` (как `vmess://` в 3x-ui)
- Telegram-бот, несколько устройств на пароль, inbound-настройки
- Установщик [wdtt-install](https://github.com/ildarmaga/wdtt-install)

### Лицензия upstream

Проект [proxy-turn-vk-android](https://github.com/amurcanov/proxy-turn-vk-android) распространяется под **GNU GPL v3**.  
WDTT также использует **GPL v3** — см. [LICENSE](LICENSE).

При распространении производных работ необходимо соблюдать условия GPL v3 и указывать исходный проект.

## Другие проекты

| Проект | Использование |
|--------|----------------|
| [3x-ui](https://github.com/MHSanaei/3x-ui) | UI/UX панели, работа с Xray, формат share-ссылок |
| [Xray-core](https://github.com/XTLS/Xray-core) | Маршрутизация трафика VPN-подсети |

## Ссылки

- Upstream VPN: https://github.com/amurcanov/proxy-turn-vk-android
- PWDTT (оригинал): https://github.com/luminescq/PWDTT
- PWDTT (модифицированная версия): https://github.com/ildarmaga/pwdtt-client
- iOS-клиент: https://github.com/anton48/vk-turn-proxy-ios
- WDTT (этот репозиторий): https://github.com/ildarmaga/wdtt
- Установщик: https://github.com/ildarmaga/wdtt-install
