//go:build linux && cgo && gtk3

package platform

/*
#cgo pkg-config: gtk+-3.0 gio-2.0
#include <gtk/gtk.h>
#include <gio/gio.h>

static GSettings *sinanDesktopSettings(void) {
    GSettingsSchemaSource *source = g_settings_schema_source_get_default();
    if (!source) return NULL;
    GSettingsSchema *schema = g_settings_schema_source_lookup(source, "org.gnome.desktop.interface", TRUE);
    if (!schema) return NULL;
    GSettings *settings = g_settings_new_full(schema, NULL, NULL);
    g_settings_schema_unref(schema);
    return settings;
}
static gboolean sinanSettingsDark(GSettings *settings) {
    if (!settings) return FALSE;
    GSettingsSchema *schema = NULL;
    g_object_get(settings, "settings-schema", &schema, NULL);
    if (!schema) return FALSE;
    gboolean dark = FALSE;
    gchar *scheme = g_settings_schema_has_key(schema, "color-scheme") ? g_settings_get_string(settings, "color-scheme") : NULL;
    if (g_strcmp0(scheme, "prefer-dark") == 0) dark = TRUE;
    else if (g_strcmp0(scheme, "prefer-light") != 0 && g_settings_schema_has_key(schema, "gtk-theme")) {
        gchar *theme = g_settings_get_string(settings, "gtk-theme");
        gchar *lower = g_ascii_strdown(theme, -1);
        dark = g_str_has_suffix(lower, "-dark");
        g_free(lower);
        g_free(theme);
    }
    g_free(scheme);
    g_settings_schema_unref(schema);
    return dark;
}
static gboolean sinanSystemDark(void) {
    GSettings *settings = sinanDesktopSettings();
    gboolean dark = sinanSettingsDark(settings);
    if (settings) g_object_unref(settings);
    return dark;
}

// Appearance is an application-wide preference. Observe desktop changes only
// after GTK has initialized, without writing to the user's GSettings database.
static int sinanThemeMode = 0;
static void sinanDesktopThemeChanged(GSettings *settings, gchar *key, gpointer unused) {
    if (sinanThemeMode != 0) return;
    GtkSettings *gtk = gtk_settings_get_default();
    if (gtk) g_object_set(gtk, "gtk-application-prefer-dark-theme", sinanSettingsDark(settings), NULL);
}
static void sinanWindowTheme(int mode) {
    static GSettings *settings = NULL;
    static gboolean initialized = FALSE;
    GtkSettings *gtk = gtk_settings_get_default();
    if (!gtk) return;
    if (!initialized) {
        initialized = TRUE;
        settings = sinanDesktopSettings();
        if (settings) g_signal_connect(settings, "changed", G_CALLBACK(sinanDesktopThemeChanged), NULL);
    }
    sinanThemeMode = mode;
    if (mode == 0 && !settings) gtk_settings_reset_property(gtk, "gtk-application-prefer-dark-theme");
    else g_object_set(gtk, "gtk-application-prefer-dark-theme", mode == 2 || (mode == 0 && sinanSettingsDark(settings)), NULL);
}
*/
import "C"
import "unsafe"

func SystemDark() bool { return C.sinanSystemDark() != 0 }

func SetWindowTheme(window unsafe.Pointer, mode string) {
	if window == nil {
		return
	}
	value := 0
	if mode == "light" {
		value = 1
	}
	if mode == "dark" {
		value = 2
	}
	C.sinanWindowTheme(C.int(value))
}
