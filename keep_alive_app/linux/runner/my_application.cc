#include "my_application.h"

#include <flutter_linux/flutter_linux.h>
#ifdef GDK_WINDOWING_X11
#include <gdk/gdkx.h>
#endif

#include <glib/gstdio.h>
#include <unistd.h>

#include "flutter/generated_plugin_registrant.h"

#define PLATFORM_CHANNEL "com.stigoleg.keepAliveApp/platform"

static const gint kPopupWidth = 320;
static const gint kPopupHeight = 460;

struct _MyApplication {
  GtkApplication parent_instance;
  char** dart_entrypoint_arguments;
  GtkStatusIcon* tray_icon;
  gchar* icon_path;
  gchar* tooltip_text;
  FlMethodChannel* method_channel;
  GtkWindow* popup_window;
  gboolean popover_visible;
};

G_DEFINE_TYPE(MyApplication, my_application, GTK_TYPE_APPLICATION)

// ── Auto-start helpers ──────────────────────────────────

static gchar* get_autostart_desktop_path(void) {
  const gchar* config_dir = g_get_user_config_dir();
  gchar* autostart_dir = g_build_filename(config_dir, "autostart", NULL);
  g_mkdir_with_parents(autostart_dir, 0755);
  gchar* path = g_build_filename(autostart_dir, "keepalive.desktop", NULL);
  g_free(autostart_dir);
  return path;
}

static void set_auto_start(gboolean enabled) {
  gchar* desktop_path = get_autostart_desktop_path();

  if (enabled) {
    gchar* exe_path = g_file_read_link("/proc/self/exe", NULL);
    GKeyFile* keyfile = g_key_file_new();
    g_key_file_set_string(keyfile, "Desktop Entry", "Type", "Application");
    g_key_file_set_string(keyfile, "Desktop Entry", "Name", "KeepAlive");
    g_key_file_set_string(keyfile, "Desktop Entry", "Exec",
                          exe_path ? exe_path : "keep_alive_app");
    g_key_file_set_string(keyfile, "Desktop Entry", "Hidden", "false");
    g_key_file_set_string(keyfile, "Desktop Entry", "X-GNOME-Autostart-enabled", "true");
    g_key_file_set_boolean(keyfile, "Desktop Entry", "Terminal", FALSE);
    g_key_file_set_boolean(keyfile, "Desktop Entry", "NoDisplay", TRUE);

    GError* error = NULL;
    if (!g_key_file_save_to_file(keyfile, desktop_path, &error)) {
      g_warning("Failed to save autostart file: %s", error->message);
      g_error_free(error);
    }
    g_key_file_free(keyfile);
    g_free(exe_path);
  } else {
    g_unlink(desktop_path);
  }

  g_free(desktop_path);
}

static gboolean is_auto_start_enabled(void) {
  gchar* desktop_path = get_autostart_desktop_path();
  gboolean exists = g_file_test(desktop_path, G_FILE_TEST_EXISTS);
  g_free(desktop_path);
  return exists;
}

// ── Popup window helpers ────────────────────────────────

static void popup_window_get_position(MyApplication* self, gint* out_x, gint* out_y) {
  gint screen_x = 0, screen_y = 0;
  GdkScreen* screen = gdk_screen_get_default();
  gint screen_w = gdk_screen_get_width(screen);
  gint screen_h = gdk_screen_get_height(screen);

  if (self->tray_icon) {
    GdkRectangle rect;
    GtkOrientation orientation;
    if (gtk_status_icon_get_geometry(self->tray_icon, &screen, &rect, &orientation)) {
      if (orientation == GTK_ORIENTATION_HORIZONTAL) {
        *out_x = rect.x - (kPopupWidth / 2) + (rect.width / 2);
        if (rect.y < screen_h / 2) {
          *out_y = rect.y + rect.height + 4;
        } else {
          *out_y = rect.y - kPopupHeight - 4;
        }
      } else {
        if (rect.x < screen_w / 2) {
          *out_x = rect.x + rect.width + 4;
        } else {
          *out_x = rect.x - kPopupWidth - 4;
        }
        *out_y = rect.y - (kPopupHeight / 2) + (rect.height / 2);
      }
      return;
    }
  }

  *out_x = screen_w - kPopupWidth - 16;
  *out_y = 4;
}

static void show_popup_window(MyApplication* self) {
  if (self->popover_visible) return;

  gint x, y;
  popup_window_get_position(self, &x, &y);

  GdkScreen* screen = gdk_screen_get_default();
  gint screen_w = gdk_screen_get_width(screen);
  gint screen_h = gdk_screen_get_height(screen);

  if (x < 0) x = 4;
  if (x + kPopupWidth > screen_w) x = screen_w - kPopupWidth - 4;
  if (y < 0) y = 4;
  if (y + kPopupHeight > screen_h) y = screen_h - kPopupHeight - 4;

  if (self->popup_window == NULL) {
    GtkWidget* window = gtk_window_new(GTK_WINDOW_POPUP);
    gtk_window_set_decorated(GTK_WINDOW(window), FALSE);
    gtk_window_set_skip_taskbar_hint(GTK_WINDOW(window), TRUE);
    gtk_window_set_skip_pager_hint(GTK_WINDOW(window), TRUE);
    gtk_window_set_keep_above(GTK_WINDOW(window), TRUE);
    gtk_widget_set_size_request(window, kPopupWidth, kPopupHeight);

    GdkRGBA bg;
    gdk_rgba_parse(&bg, "#000000");
    gtk_widget_override_background_color(window, GTK_STATE_FLAG_NORMAL, &bg);

    g_signal_connect(window, "focus-out-event", G_CALLBACK(popup_focus_out_cb), self);
    g_signal_connect(window, "key-press-event", G_CALLBACK(popup_key_press_cb), self);

    self->popup_window = GTK_WINDOW(window);
  }

  gtk_window_move(self->popup_window, x, y);
  gtk_widget_show_all(GTK_WIDGET(self->popup_window));
  gtk_window_present(self->popup_window);

  self->popover_visible = TRUE;
}

static void hide_popup_window(MyApplication* self) {
  if (!self->popover_visible) return;

  if (self->popup_window != NULL) {
    gtk_widget_hide(GTK_WIDGET(self->popup_window));
  }

  self->popover_visible = FALSE;

  g_autoptr(FlValue) args = fl_value_new_string("popoverDismissed");
  fl_method_channel_invoke_method(self->method_channel, "onTrayEvent",
                                  args, NULL, NULL, NULL);
}

static gboolean popup_focus_out_cb(GtkWidget* widget, GdkEventFocus* event,
                                   gpointer user_data) {
  hide_popup_window(MY_APPLICATION(user_data));
  return FALSE;
}

static gboolean popup_key_press_cb(GtkWidget* widget, GdkEventKey* event,
                                   gpointer user_data) {
  if (event->keyval == GDK_KEY_Escape) {
    hide_popup_window(MY_APPLICATION(user_data));
    return TRUE;
  }
  return FALSE;
}

// ── Tray icon callbacks ─────────────────────────────────

static void tray_icon_activate_cb(GtkStatusIcon* icon, gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  g_autoptr(FlValue) args = fl_value_new_string("leftClick");
  fl_method_channel_invoke_method(self->method_channel, "onTrayEvent",
                                  args, NULL, NULL, NULL);
}

static void tray_icon_popup_menu_cb(GtkStatusIcon* icon, guint button,
                                    guint32 activate_time, gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  g_autoptr(FlValue) args = fl_value_new_string("rightClick");
  fl_method_channel_invoke_method(self->method_channel, "onTrayEvent",
                                  args, NULL, NULL, NULL);
}

// ── Tray icon management ────────────────────────────────

static void create_tray_icon(MyApplication* self) {
  if (self->tray_icon) return;

  self->tray_icon = gtk_status_icon_new();
  gtk_status_icon_set_visible(self->tray_icon, TRUE);

  if (self->icon_path) {
    gtk_status_icon_set_from_file(self->tray_icon, self->icon_path);
  } else {
    gtk_status_icon_set_from_icon_name(self->tray_icon, "application-x-executable");
  }

  if (self->tooltip_text) {
    gtk_status_icon_set_tooltip_text(self->tray_icon, self->tooltip_text);
  } else {
    gtk_status_icon_set_tooltip_text(self->tray_icon, "KeepAlive");
  }

  g_signal_connect(self->tray_icon, "activate",
                   G_CALLBACK(tray_icon_activate_cb), self);
  g_signal_connect(self->tray_icon, "popup-menu",
                   G_CALLBACK(tray_icon_popup_menu_cb), self);
}

static void destroy_tray_icon(MyApplication* self) {
  if (self->tray_icon) {
    g_object_unref(self->tray_icon);
    self->tray_icon = NULL;
  }
}

// ── Context menu ────────────────────────────────────────

static gint show_context_menu(MyApplication* self, FlValue* items_value) {
  gint selected_index = -1;
  GtkWidget* menu = gtk_menu_new();

  for (size_t i = 0; i < fl_value_get_length(items_value); i++) {
    FlValue* item_value = fl_value_get_list_value(items_value, i);
    const gchar* title = fl_value_get_string(item_value);

    if (g_strcmp0(title, "-") == 0) {
      GtkWidget* sep = gtk_separator_menu_item_new();
      gtk_widget_show(sep);
      gtk_menu_shell_append(GTK_MENU_SHELL(menu), sep);
    } else {
      GtkWidget* item = gtk_menu_item_new_with_label(title);
      g_object_set_data(G_OBJECT(item), "menu-index", GINT_TO_POINTER((gint)i));
      g_signal_connect(item, "activate", G_CALLBACK(context_menu_item_activated_cb),
                       &selected_index);
      gtk_widget_show(item);
      gtk_menu_shell_append(GTK_MENU_SHELL(menu), item);
    }
  }

  gtk_menu_popup_at_pointer(GTK_MENU(menu), NULL);
  gtk_main();

  gtk_widget_destroy(menu);
  while (gtk_events_pending()) gtk_main_iteration();

  return selected_index;
}

static void context_menu_item_activated_cb(GtkMenuItem* item, gpointer user_data) {
  gint* selected_index = (gint*)user_data;
  *selected_index = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(item), "menu-index"));
  gtk_main_quit();
}

// ── Asset path resolution ────────────────────────────

static gchar* resolve_asset_path(const gchar* asset_key) {
  gchar* exe_path = g_file_read_link("/proc/self/exe", NULL);
  if (!exe_path) {
    return g_strdup(asset_key);
  }
  gchar* exe_dir = g_path_get_dirname(exe_path);
  g_free(exe_path);

  gchar* asset_path = g_build_filename(exe_dir, "data", "flutter_assets",
                                       asset_key, NULL);
  g_free(exe_dir);
  return asset_path;
}

// ── App support dir ─────────────────────────────────────

static gchar* get_app_support_dir(void) {
  return g_strdup(g_get_user_data_dir());
}

// ── Battery info ────────────────────────────────────────

static gboolean read_battery_int(const gchar* battery_dir,
                                 const gchar* file,
                                 gint* out_value) {
  gchar* path = g_build_filename(battery_dir, file, NULL);
  gchar* contents = NULL;
  gsize length = 0;
  gboolean ok = g_file_get_contents(path, &contents, &length, NULL);
  g_free(path);
  if (!ok || !contents) {
    g_free(contents);
    return FALSE;
  }
  *out_value = atoi(g_strstrip(contents));
  g_free(contents);
  return TRUE;
}

static gboolean read_battery_string(const gchar* battery_dir,
                                    const gchar* file,
                                    gchar** out_value) {
  gchar* path = g_build_filename(battery_dir, file, NULL);
  gchar* contents = NULL;
  gboolean ok = g_file_get_contents(path, &contents, NULL, NULL);
  g_free(path);
  if (!ok || !contents) {
    g_free(contents);
    return FALSE;
  }
  g_strstrip(contents);
  *out_value = contents;
  return TRUE;
}

static FlValue* get_battery_info(void) {
  FlValue* map = fl_value_new_map();
  const gchar* base = "/sys/class/power_supply";
  GError* error = NULL;
  GDir* dir = g_dir_open(base, 0, &error);
  if (dir == NULL) {
    if (error) g_error_free(error);
    fl_value_set_string(map, "percentage", fl_value_new_float(100.0));
    fl_value_set_string(map, "isCharging", fl_value_new_bool(FALSE));
    fl_value_set_string(map, "isPresent", fl_value_new_bool(FALSE));
    return map;
  }

  gboolean found = FALSE;
  gint capacity = 100;
  gboolean charging = FALSE;
  const gchar* entry;
  while ((entry = g_dir_read_name(dir)) != NULL) {
    if (!g_str_has_prefix(entry, "BAT")) continue;
    gchar* battery_dir = g_build_filename(base, entry, NULL);
    gint cap = 0;
    if (read_battery_int(battery_dir, "capacity", &cap)) {
      capacity = cap;
      found = TRUE;
    }
    gchar* status = NULL;
    if (read_battery_string(battery_dir, "status", &status)) {
      charging = (g_strcmp0(status, "Charging") == 0 ||
                  g_strcmp0(status, "Full") == 0);
      g_free(status);
    }
    g_free(battery_dir);
    if (found) break;
  }
  g_dir_close(dir);

  fl_value_set_string(map, "percentage",
                      fl_value_new_float(found ? (double)capacity : 100.0));
  fl_value_set_string(map, "isCharging", fl_value_new_bool(charging));
  fl_value_set_string(map, "isPresent", fl_value_new_bool(found));
  return map;
}

// ── Method channel handler ──────────────────────────────

static void method_channel_cb(FlMethodChannel* channel, FlMethodCall* method_call,
                              gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  const gchar* method = fl_method_call_get_name(method_call);
  FlValue* args = fl_method_call_get_args(method_call);
  g_autoptr(FlMethodResponse) response = NULL;
  g_autoptr(GError) error = NULL;

  if (g_strcmp0(method, "getPlatformName") == 0) {
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(
        fl_value_new_string("Linux")));

  } else if (g_strcmp0(method, "setAutoStart") == 0) {
    FlValue* enabled_val = fl_value_lookup_string(args, "enabled");
    if (enabled_val && fl_value_get_type(enabled_val) == FL_VALUE_TYPE_BOOL) {
      set_auto_start(fl_value_get_bool(enabled_val));
      response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));
    } else {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new(
          "INVALID_ARG", "Missing 'enabled' argument", NULL));
    }

  } else if (g_strcmp0(method, "isAutoStartEnabled") == 0) {
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(
        fl_value_new_bool(is_auto_start_enabled())));

  } else if (g_strcmp0(method, "setTrayIcon") == 0) {
    FlValue* icon_path_val = fl_value_lookup_string(args, "iconPath");
    if (icon_path_val && fl_value_get_type(icon_path_val) == FL_VALUE_TYPE_STRING) {
      g_free(self->icon_path);
      self->icon_path = resolve_asset_path(fl_value_get_string(icon_path_val));
      if (self->tray_icon) {
        gtk_status_icon_set_from_file(self->tray_icon, self->icon_path);
      } else {
        create_tray_icon(self);
      }
      response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));
    } else {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new(
          "INVALID_ARG", "Missing 'iconPath' argument", NULL));
    }

  } else if (g_strcmp0(method, "setTrayTooltip") == 0) {
    FlValue* tooltip_val = fl_value_lookup_string(args, "tooltip");
    if (tooltip_val && fl_value_get_type(tooltip_val) == FL_VALUE_TYPE_STRING) {
      g_free(self->tooltip_text);
      self->tooltip_text = g_strdup(fl_value_get_string(tooltip_val));
      if (self->tray_icon) {
        gtk_status_icon_set_tooltip_text(self->tray_icon, self->tooltip_text);
      }
      response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));
    } else {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new(
          "INVALID_ARG", "Missing 'tooltip' argument", NULL));
    }

  } else if (g_strcmp0(method, "showContextMenu") == 0) {
    FlValue* items_val = fl_value_lookup_string(args, "items");
    if (items_val && fl_value_get_type(items_val) == FL_VALUE_TYPE_LIST) {
      gint selected = show_context_menu(self, items_val);
      if (selected >= 0) {
        response = FL_METHOD_RESPONSE(fl_method_success_response_new(
            fl_value_new_int(selected)));
      } else {
        response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));
      }
    } else {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new(
          "INVALID_ARG", "Missing 'items' argument", NULL));
    }

  } else if (g_strcmp0(method, "showPopover") == 0) {
    show_popup_window(self);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));

  } else if (g_strcmp0(method, "hidePopover") == 0) {
    hide_popup_window(self);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));

  } else if (g_strcmp0(method, "getAppSupportDir") == 0) {
    gchar* dir = get_app_support_dir();
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(
        fl_value_new_string(dir)));
    g_free(dir);

  } else if (g_strcmp0(method, "setStatusBarTitle") == 0) {
    // GtkStatusIcon has no inline title slot; accept and ignore so the
    // shared cross-platform Dart caller does not get MissingPluginException.
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(NULL));

  } else if (g_strcmp0(method, "getBatteryInfo") == 0) {
    FlValue* battery = get_battery_info();
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(battery));

  } else {
    response = FL_METHOD_RESPONSE(fl_method_not_implemented_response_new());
  }

  g_autoptr(GError) resp_error = NULL;
  if (!fl_method_call_respond(method_call, response, &resp_error)) {
    g_warning("Failed to send method response: %s", resp_error->message);
  }
}

// ── Application lifecycle ───────────────────────────────

static void first_frame_cb(MyApplication* self, FlView* view) {
  GtkWidget* toplevel = gtk_widget_get_toplevel(GTK_WIDGET(view));
  gtk_widget_hide(toplevel);
  gtk_window_set_decorated(GTK_WINDOW(toplevel), FALSE);
}

static void my_application_activate(GApplication* application) {
  MyApplication* self = MY_APPLICATION(application);
  GtkWindow* window =
      GTK_WINDOW(gtk_application_window_new(GTK_APPLICATION(application)));

  gtk_window_set_title(window, "keep_alive_app");
  gtk_window_set_default_size(window, 320, 500);
  gtk_window_set_decorated(window, FALSE);

  g_autoptr(FlDartProject) project = fl_dart_project_new();
  fl_dart_project_set_dart_entrypoint_arguments(
      project, self->dart_entrypoint_arguments);

  FlView* view = fl_view_new(project);
  GdkRGBA background_color;
  gdk_rgba_parse(&background_color, "#000000");
  fl_view_set_background_color(view, &background_color);
  gtk_widget_show(GTK_WIDGET(view));
  gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));

  g_signal_connect_swapped(view, "first-frame", G_CALLBACK(first_frame_cb), self);
  gtk_widget_realize(GTK_WIDGET(view));

  fl_register_plugins(FL_PLUGIN_REGISTRY(view));

  FlEngine* engine = fl_view_get_engine(view);
  FlBinaryMessenger* messenger = fl_engine_get_binary_messenger(engine);
  self->method_channel = fl_method_channel_new(
      messenger, PLATFORM_CHANNEL,
      FL_METHOD_CODEC(fl_standard_method_codec_new()));
  fl_method_channel_set_method_call_handler(self->method_channel,
      method_channel_cb, self, NULL);

  gtk_widget_grab_focus(GTK_WIDGET(view));
}

static gboolean my_application_local_command_line(GApplication* application,
                                                  gchar*** arguments,
                                                  int* exit_status) {
  MyApplication* self = MY_APPLICATION(application);
  self->dart_entrypoint_arguments = g_strdupv(*arguments + 1);

  g_autoptr(GError) error = nullptr;
  if (!g_application_register(application, nullptr, &error)) {
    g_warning("Failed to register: %s", error->message);
    *exit_status = 1;
    return TRUE;
  }

  g_application_activate(application);
  *exit_status = 0;

  return TRUE;
}

static void my_application_startup(GApplication* application) {
  G_APPLICATION_CLASS(my_application_parent_class)->startup(application);
}

static void my_application_shutdown(GApplication* application) {
  MyApplication* self = MY_APPLICATION(application);
  if (self->method_channel) {
    g_autoptr(FlValue) null_args = fl_value_new_null();
    fl_method_channel_invoke_method(self->method_channel,
        "systemShutdown", null_args, NULL, NULL, NULL);
  }
  hide_popup_window(self);
  destroy_tray_icon(self);
  G_APPLICATION_CLASS(my_application_parent_class)->shutdown(application);
}

static void my_application_dispose(GObject* object) {
  MyApplication* self = MY_APPLICATION(object);
  g_clear_pointer(&self->dart_entrypoint_arguments, g_strfreev);
  g_clear_pointer(&self->icon_path, g_free);
  g_clear_pointer(&self->tooltip_text, g_free);
  g_clear_object(&self->method_channel);
  G_OBJECT_CLASS(my_application_parent_class)->dispose(object);
}

static void my_application_class_init(MyApplicationClass* klass) {
  G_APPLICATION_CLASS(klass)->activate = my_application_activate;
  G_APPLICATION_CLASS(klass)->local_command_line =
      my_application_local_command_line;
  G_APPLICATION_CLASS(klass)->startup = my_application_startup;
  G_APPLICATION_CLASS(klass)->shutdown = my_application_shutdown;
  G_OBJECT_CLASS(klass)->dispose = my_application_dispose;
}

static void my_application_init(MyApplication* self) {
  self->tray_icon = NULL;
  self->icon_path = NULL;
  self->tooltip_text = NULL;
  self->method_channel = NULL;
  self->popup_window = NULL;
  self->popover_visible = FALSE;
}

MyApplication* my_application_new() {
  g_set_prgname(APPLICATION_ID);

  return MY_APPLICATION(g_object_new(my_application_get_type(),
                                     "application-id", APPLICATION_ID,
                                     "flags", G_APPLICATION_NON_UNIQUE, nullptr));
}
