package transport

func processEnvironment(platform string, getenv func(string) string) []string {
	keys := []string{"LANG", "LC_ALL", "LC_MESSAGES"}
	switch platform {
	case "windows":
		keys = append(keys, "SystemRoot", "WINDIR", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "TEMP", "TMP", "PATH")
	case "linux":
		keys = append(keys, "HOME", "PATH", "XDG_DATA_HOME", "XDG_DATA_DIRS", "XDG_CURRENT_DESKTOP", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS")
	}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		value := getenv(key)
		if key == "LANG" && value == "" {
			value = "en_US.UTF-8"
		}
		if value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}
