package i18n

// The Russian texts. Terms follow the AmneziaVPN app's own Russian
// translation ("KillSwitch", "рукопожатие", "логи"). catalog_test.go checks
// that every text the code translates is here, that nothing here is unused,
// and that format verbs match.

// ru translates the command-line tool's texts, looked up by T.
var ru = map[string]string{
	`awg-hs: AmneziaWG for macOS 10.13 High Sierra

Usage:
  awg-hs up [FILE | vpn://KEY | -]   connect; with no argument, reconnect with
                                     the last config that worked
  awg-hs down                        disconnect
  awg-hs status                      show the connection
  awg-hs killswitch [on | off]       show or change the kill switch (on at first)
  awg-hs check FILE | vpn://KEY      check a config without connecting
  awg-hs convert vpn://KEY           print the AmneziaWG .conf inside a key
  awg-hs version

  awg-hs daemon [-verbose]           the background service (run by launchd)

FILE is an AmneziaWG .conf exported from the AmneziaVPN app, and KEY is a
vpn:// key for a self-hosted AmneziaWG server. "-" reads from standard input.

While connected, the kill switch blocks everything that would leave the Mac
outside the tunnel, apart from the local network. If the tunnel fails it
keeps blocking until "awg-hs up" or "awg-hs down".
`: `awg-hs: AmneziaWG для macOS 10.13 High Sierra

Использование:
  awg-hs up [ФАЙЛ | vpn://КЛЮЧ | -]  подключиться; без аргумента — снова
                                     подключиться с последней конфигурацией
  awg-hs down                        отключиться
  awg-hs status                      показать подключение
  awg-hs killswitch [on | off]       показать или переключить KillSwitch
                                     (сначала включён)
  awg-hs check ФАЙЛ | vpn://КЛЮЧ     проверить конфигурацию, не подключаясь
  awg-hs convert vpn://КЛЮЧ          вывести .conf AmneziaWG из ключа
  awg-hs version

  awg-hs daemon [-verbose]           фоновая служба (её запускает launchd)

ФАЙЛ — это .conf AmneziaWG, сохранённый из приложения AmneziaVPN, а КЛЮЧ —
ключ vpn:// для вашего сервера AmneziaWG. «-» читает со стандартного ввода.

Пока VPN подключён, KillSwitch блокирует весь трафик этого Mac в обход
туннеля, кроме локальной сети. Если туннель перестаёт работать, блокировка
остаётся до «awg-hs up» или «awg-hs down».
`,

	"unknown command %q":  "неизвестная команда %q",
	"vpn:// key":          "ключ vpn://",
	"up takes one config": "команде up нужна одна конфигурация",
	"Connecting...":       "Подключение...",

	"usage: awg-hs killswitch [on | off]": "использование: awg-hs killswitch [on | off]",
	"Kill switch: %s\n":                   "KillSwitch: %s\n",
	"Nothing can leave this Mac outside the tunnel, apart from the local network.": "Ничто не покинет этот Mac в обход туннеля, кроме трафика локальной сети.",
	"It will block traffic outside the tunnel from the next \"awg-hs up\".":        "Он будет блокировать трафик в обход туннеля со следующего «awg-hs up».",
	"If the tunnel stops working, traffic goes out directly.":                      "Если туннель перестанет работать, трафик пойдёт напрямую.",

	"cannot reach the awg-hs service (%v).\nIs it installed? Only administrator accounts can use it; otherwise try sudo.": "нет связи со службой awg-hs (%v).\nОна установлена? Пользоваться ей могут только администраторы; иначе попробуйте sudo.",

	"on":  "вкл",
	"off": "выкл",

	"The kill switch is blocking all traffic outside the tunnel, because the tunnel\nis not running. Run \"awg-hs up\" to reconnect, or \"awg-hs down\" to go back\nto the normal internet.\n": "KillSwitch блокирует весь трафик в обход туннеля, потому что туннель\nне работает. Выполните «awg-hs up», чтобы подключиться снова, или\n«awg-hs down», чтобы вернуться к обычному интернету.\n",

	"Disconnected.": "Отключено.",
	"Run \"awg-hs up\" to reconnect to %s.\n": "Выполните «awg-hs up», чтобы снова подключиться (%s).\n",
	"Connected: %s (%s)\n":                    "Подключено: %s (%s)\n",
	"  Server:      %s\n":                     "  Сервер:      %s\n",
	"  Addresses:   %s\n":                     "  Адреса:      %s\n",
	"  Since:       %s (%s)\n":                "  Начало:      %s (%s)\n",
	"  Handshake:   %s ago\n":                 "  Рукопожатие: %s назад\n",
	"  Handshake:   none yet\n":               "  Рукопожатие: ещё не было\n",
	"               (the server is not answering: check its address and port,\n                and that the config is current)\n": "               (сервер не отвечает: проверьте его адрес и порт,\n                и что конфигурация актуальна)\n",
	"  Traffic:     %s received, %s sent\n": "  Трафик:      получено %s, отправлено %s\n",
	"  Kill switch: %s\n":                   "  KillSwitch:  %s\n",
	"  Warning:     %s\n":                   "  Внимание:    %s\n",

	"check takes one config":             "команде check нужна одна конфигурация",
	"Config OK.":                         "Конфигурация в порядке.",
	"  Addresses:  %s\n":                 "  Адреса:     %s\n",
	"  Peer %d:     %s, AllowedIPs %s\n": "  Пир %d:      %s, AllowedIPs %s\n",
	"convert takes one vpn:// key":       "команде convert нужен один ключ vpn://",
	"(none)":                             "(нет)",

	"B":   "Б",
	"KiB": "КБ",
	"MiB": "МБ",
	"GiB": "ГБ",
	"TiB": "ТБ",
	"PiB": "ПБ",
	"EiB": "ЭБ",
}

// phrases translates the pieces of error and warning messages, for Message.
var phrases = []phrase{
	// Reading configs (internal/config).
	{"key must be 32 bytes of base64", "ключ должен быть 32 байтами в base64"},
	{"unknown section", "неизвестный раздел"},
	{"more than one [Interface] section", "больше одного раздела [Interface]"},
	{`expected "Key = Value"`, "ожидается «Ключ = Значение»"},
	{"setting is outside of a section", "параметр вне раздела"},
	{"not a supported [Interface] setting", "параметр не поддерживается в [Interface]"},
	{"not a supported [Peer] setting", "параметр не поддерживается в [Peer]"},
	{"must be between 576 and 9000", "должно быть от 576 до 9000"},
	{"is not on/off", "— не on/off"},
	{"[Interface] has no PrivateKey", "в [Interface] нет PrivateKey"},
	{"[Interface] has no Address", "в [Interface] нет Address"},
	{"config has no [Peer] section", "в конфигурации нет раздела [Peer]"},
	{"has no PublicKey", "без PublicKey"},
	{"bad port in", "неверный порт в"},
	{"vpn:// key is not valid base64", "ключ vpn:// — не корректный base64"},
	{"vpn:// key does not contain a server config", "в ключе vpn:// нет конфигурации сервера"},
	{"this is an Amnezia subscription key (Premium or free); only keys for self-hosted servers are supported",
		"это ключ подписки Amnezia (Premium или бесплатной); поддерживаются только ключи для VPN на своём сервере"},
	{"vpn:// key has no AmneziaWG or WireGuard connection in it (export an AmneziaWG connection for this server from the AmneziaVPN app)",
		"в ключе vpn:// нет подключения AmneziaWG или WireGuard (экспортируйте подключение AmneziaWG для этого сервера из приложения AmneziaVPN)"},

	// Connecting (internal/tunnel, the daemon).
	{"resolving server address", "не удалось найти адрес сервера"},
	{"no addresses for", "нет адресов для"},
	{"creating the utun interface", "не удалось создать интерфейс utun"},
	{"configuring AmneziaWG", "не удалось настроить AmneziaWG"},
	{"starting AmneziaWG", "не удалось запустить AmneziaWG"},
	{"turning on the kill switch", "не удалось включить KillSwitch"},
	{"IPv6 traffic is not going through the tunnel (could not add the IPv6 route)",
		"трафик IPv6 идёт в обход туннеля (не удалось добавить маршрут IPv6)"},
	{"DNS could not be switched to the tunnel's servers, so lookups may bypass the tunnel",
		"DNS не удалось переключить на серверы туннеля, поэтому запросы могут идти в обход туннеля"},
	{"no config given and none saved from an earlier connection",
		"конфигурация не указана, а сохранённой от прошлого подключения нет"},
	{"The kill switch is blocking the internet, so the server's name can't be looked up.",
		"KillSwitch блокирует интернет, поэтому не удаётся найти адрес сервера."},
	{`Run "awg-hs down" to lift it, then connect again.`,
		"Выполните «awg-hs down», чтобы снять блокировку, затем подключитесь снова."},
	{"only administrators can control the tunnel", "управлять туннелем могут только администраторы"},
	{"unknown command", "неизвестная команда"},
}
