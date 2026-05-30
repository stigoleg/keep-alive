#include "flutter_window.h"

#include <optional>
#include <shlobj.h>
#include <shellapi.h>

#include "flutter/generated_plugin_registrant.h"

namespace {

std::wstring Utf8ToWide(const std::string& str) {
  if (str.empty()) return L"";
  int size = MultiByteToWideChar(CP_UTF8, 0, str.c_str(), -1, nullptr, 0);
  std::wstring result(size, L'\0');
  MultiByteToWideChar(CP_UTF8, 0, str.c_str(), -1, &result[0], size);
  result.resize(size - 1);
  return result;
}

std::string WideToUtf8(const std::wstring& wstr) {
  if (wstr.empty()) return "";
  int size = WideCharToMultiByte(CP_UTF8, 0, wstr.c_str(), -1, nullptr, 0, nullptr, nullptr);
  std::string result(size, '\0');
  WideCharToMultiByte(CP_UTF8, 0, wstr.c_str(), -1, &result[0], size, nullptr, nullptr);
  result.resize(size - 1);
  return result;
}

}  // namespace

FlutterWindow::FlutterWindow(const flutter::DartProject& project)
    : project_(project) {}

FlutterWindow::~FlutterWindow() {
  RemoveTrayIcon();
}

bool FlutterWindow::OnCreate() {
  if (!Win32Window::OnCreate()) {
    return false;
  }

  RECT frame = GetClientArea();

  flutter_controller_ = std::make_unique<flutter::FlutterViewController>(
      frame.right - frame.left, frame.bottom - frame.top, project_);
  if (!flutter_controller_->engine() || !flutter_controller_->view()) {
    return false;
  }
  RegisterPlugins(flutter_controller_->engine());
  SetChildContent(flutter_controller_->view()->GetNativeWindow());

  flutter_controller_->engine()->SetNextFrameCallback([&]() {
    this->Show();
  });

  flutter_controller_->ForceRedraw();

  InitMethodChannel();

  return true;
}

void FlutterWindow::OnDestroy() {
  RemoveTrayIcon();
  platform_channel_.reset();
  if (flutter_controller_) {
    flutter_controller_ = nullptr;
  }

  Win32Window::OnDestroy();
}

LRESULT
FlutterWindow::MessageHandler(HWND hwnd, UINT const message,
                              WPARAM const wparam,
                              LPARAM const lparam) noexcept {
  if (flutter_controller_) {
    std::optional<LRESULT> result =
        flutter_controller_->HandleTopLevelWindowProc(hwnd, message, wparam,
                                                      lparam);
    if (result) {
      return *result;
    }
  }

  switch (message) {
    case WM_QUERYENDSESSION:
      return TRUE;

    case WM_ENDSESSION:
      if (wparam == TRUE && platform_channel_) {
        platform_channel_->InvokeMethod("systemShutdown",
            std::make_unique<flutter::EncodableValue>(flutter::EncodableMap()));
      }
      return 0;

    case WM_FONTCHANGE:
      flutter_controller_->engine()->ReloadSystemFonts();
      break;

    case WM_TRAY_ICON:
      if (LOWORD(lparam) == WM_RBUTTONUP || LOWORD(lparam) == WM_CONTEXTMENU) {
        // Defer to MethodChannel; the Dart side will call showContextMenu.
      }
      break;
  }

  return Win32Window::MessageHandler(hwnd, message, wparam, lparam);
}

void FlutterWindow::InitMethodChannel() {
  auto messenger = flutter_controller_->engine()->messenger();
  platform_channel_ = std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
      messenger, "com.stigoleg.keepAliveApp/platform",
      &flutter::StandardMethodCodec::GetInstance());

  platform_channel_->SetMethodCallHandler(
      [this](const flutter::MethodCall<flutter::EncodableValue>& call,
             std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result) {
        HandleMethodCall(call, std::move(result));
      });
}

void FlutterWindow::HandleMethodCall(
    const flutter::MethodCall<flutter::EncodableValue>& call,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result) {
  const auto& method = call.method_name();
  const auto* args = std::get_if<flutter::EncodableMap>(call.arguments());

  if (method == "getPlatformName") {
    result->Success(flutter::EncodableValue("Windows"));
  } else if (method == "setAutoStart") {
    HandleSetAutoStart(args ? *args : flutter::EncodableMap{}, result);
  } else if (method == "isAutoStartEnabled") {
    HandleIsAutoStartEnabled(result);
  } else if (method == "setTrayIcon") {
    HandleSetTrayIcon(args ? *args : flutter::EncodableMap{}, result);
  } else if (method == "setTrayTooltip") {
    HandleSetTrayTooltip(args ? *args : flutter::EncodableMap{}, result);
  } else if (method == "showContextMenu") {
    HandleShowContextMenu(args ? *args : flutter::EncodableMap{}, result);
  } else if (method == "showPopover" || method == "hidePopover") {
    result->Success();
  } else if (method == "getAppSupportDir") {
    HandleGetAppSupportDir(result);
  } else {
    result->NotImplemented();
  }
}

void FlutterWindow::HandleSetAutoStart(
    const flutter::EncodableMap& args,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result) {
  auto it = args.find(flutter::EncodableValue("enabled"));
  if (it == args.end()) {
    result->Error("INVALID_ARG", "Missing 'enabled' argument");
    return;
  }
  auto enabled = std::get<bool>(it->second);

  if (enabled) {
    wchar_t exePath[MAX_PATH];
    GetModuleFileNameW(nullptr, exePath, MAX_PATH);
    HKEY hKey;
    if (RegOpenKeyExW(HKEY_CURRENT_USER, kAutoStartKeyPath, 0, KEY_SET_VALUE, &hKey) == ERROR_SUCCESS) {
      RegSetValueExW(hKey, kAutoStartValueName, 0, REG_SZ,
                     reinterpret_cast<const BYTE*>(exePath),
                     static_cast<DWORD>((wcslen(exePath) + 1) * sizeof(wchar_t)));
      RegCloseKey(hKey);
    }
  } else {
    HKEY hKey;
    if (RegOpenKeyExW(HKEY_CURRENT_USER, kAutoStartKeyPath, 0, KEY_SET_VALUE, &hKey) == ERROR_SUCCESS) {
      RegDeleteValueW(hKey, kAutoStartValueName);
      RegCloseKey(hKey);
    }
  }
  result->Success();
}

void FlutterWindow::HandleIsAutoStartEnabled(
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result) {
  HKEY hKey;
  bool enabled = false;
  if (RegOpenKeyExW(HKEY_CURRENT_USER, kAutoStartKeyPath, 0, KEY_QUERY_VALUE, &hKey) == ERROR_SUCCESS) {
    DWORD type;
    wchar_t value[MAX_PATH];
    DWORD size = sizeof(value);
    if (RegQueryValueExW(hKey, kAutoStartValueName, nullptr, &type,
                         reinterpret_cast<LPBYTE>(value), &size) == ERROR_SUCCESS) {
      enabled = true;
    }
    RegCloseKey(hKey);
  }
  result->Success(flutter::EncodableValue(enabled));
}

void FlutterWindow::HandleSetTrayIcon(
    const flutter::EncodableMap& args,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result) {
  auto it = args.find(flutter::EncodableValue("iconPath"));
  if (it == args.end()) {
    result->Error("INVALID_ARG", "Missing 'iconPath' argument");
    return;
  }
  auto iconPathUtf8 = std::get<std::string>(it->second);
  std::wstring iconPath = Utf8ToWide(iconPathUtf8);

  HICON hIcon = nullptr;
  if (!iconPath.empty()) {
    hIcon = reinterpret_cast<HICON>(LoadImageW(
        nullptr, iconPath.c_str(), IMAGE_ICON,
        GetSystemMetrics(SM_CXSMICON), GetSystemMetrics(SM_CYSMICON),
        LR_LOADFROMFILE));
  }

  if (!tray_created_) {
    CreateTrayIcon();
  }

  if (hIcon) {
    nid_.hIcon = hIcon;
    nid_.uFlags |= NIF_ICON;
    Shell_NotifyIconW(NIM_MODIFY, &nid_);
  }

  result->Success();
}

void FlutterWindow::HandleSetTrayTooltip(
    const flutter::EncodableMap& args,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result) {
  auto it = args.find(flutter::EncodableValue("tooltip"));
  if (it == args.end()) {
    result->Error("INVALID_ARG", "Missing 'tooltip' argument");
    return;
  }
  auto tooltip = std::get<std::string>(it->second);
  std::wstring wideTooltip = Utf8ToWide(tooltip);

  wcsncpy_s(nid_.szTip, wideTooltip.c_str(), _TRUNCATE);
  nid_.uFlags |= NIF_TIP;
  if (tray_created_) {
    Shell_NotifyIconW(NIM_MODIFY, &nid_);
  }

  result->Success();
}

void FlutterWindow::HandleShowContextMenu(
    const flutter::EncodableMap& args,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result) {
  auto it = args.find(flutter::EncodableValue("items"));
  if (it == args.end()) {
    result->Error("INVALID_ARG", "Missing 'items' argument");
    return;
  }
  auto items = std::get<flutter::EncodableList>(it->second);

  HMENU hMenu = CreatePopupMenu();
  int menuIndex = 0;
  for (const auto& item : items) {
    auto title = std::get<std::string>(item);
    if (title == "-") {
      AppendMenuW(hMenu, MF_SEPARATOR, 0, nullptr);
    } else {
      AppendMenuW(hMenu, MF_STRING, menuIndex + 1, Utf8ToWide(title).c_str());
      menuIndex++;
    }
  }

  POINT pt;
  GetCursorPos(&pt);
  SetForegroundWindow(window_handle_);
  UINT selected = TrackPopupMenu(hMenu, TPM_RETURNCMD | TPM_NONOTIFY,
                                  pt.x, pt.y, 0, window_handle_, nullptr);
  DestroyMenu(hMenu);

  if (selected > 0) {
    result->Success(flutter::EncodableValue(static_cast<int>(selected - 1)));
  } else {
    result->Success();
  }
}

void FlutterWindow::HandleGetAppSupportDir(
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result) {
  PWSTR path = nullptr;
  if (SUCCEEDED(SHGetKnownFolderPath(FOLDERID_LocalAppData, 0, nullptr, &path))) {
    std::string utf8Path = WideToUtf8(path);
    CoTaskMemFree(path);
    result->Success(flutter::EncodableValue(utf8Path));
  } else {
    result->Success(flutter::EncodableValue(std::string("")));
  }
}

void FlutterWindow::CreateTrayIcon() {
  memset(&nid_, 0, sizeof(nid_));
  nid_.cbSize = sizeof(NOTIFYICONDATAW);
  nid_.hWnd = window_handle_;
  nid_.uID = TRAY_ICON_ID;
  nid_.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP;
  nid_.uCallbackMessage = WM_TRAY_ICON;
  nid_.hIcon = LoadIcon(GetModuleHandle(nullptr), MAKEINTRESOURCE(IDI_APP_ICON));
  wcsncpy_s(nid_.szTip, L"KeepAlive", _TRUNCATE);

  Shell_NotifyIconW(NIM_ADD, &nid_);
  tray_created_ = true;
}

void FlutterWindow::RemoveTrayIcon() {
  if (tray_created_) {
    Shell_NotifyIconW(NIM_DELETE, &nid_);
    tray_created_ = false;
  }
  if (nid_.hIcon) {
    DestroyIcon(nid_.hIcon);
    nid_.hIcon = nullptr;
  }
}

void FlutterWindow::UpdateTrayIcon() {
  if (tray_created_) {
    Shell_NotifyIconW(NIM_MODIFY, &nid_);
  }
}
