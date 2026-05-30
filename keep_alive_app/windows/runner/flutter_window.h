#ifndef RUNNER_FLUTTER_WINDOW_H_
#define RUNNER_FLUTTER_WINDOW_H_

#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <memory>
#include <functional>

#include "win32_window.h"

class FlutterWindow : public Win32Window {
 public:
  explicit FlutterWindow(const flutter::DartProject& project);
  virtual ~FlutterWindow();

  bool OnCreate() override;
  void OnDestroy() override;
  LRESULT MessageHandler(HWND window, UINT const message, WPARAM const wparam,
                         LPARAM const lparam) noexcept override;

 private:
  void InitMethodChannel();
  void HandleMethodCall(
      const flutter::MethodCall<flutter::EncodableValue>& call,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result);

  void HandleSetAutoStart(const flutter::EncodableMap& args,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleIsAutoStartEnabled(
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleSetTrayIcon(const flutter::EncodableMap& args,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleSetTrayTooltip(const flutter::EncodableMap& args,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleShowContextMenu(const flutter::EncodableMap& args,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleShowPopover(
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleHidePopover(
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);
  void HandleGetAppSupportDir(
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>& result);

  void CreateTrayIcon();
  void RemoveTrayIcon();
  void UpdateTrayIcon();

  std::wstring ResolveAssetPath(const std::string& assetKey);

  bool GetTrayIconRect(RECT* outRect);
  void PositionPopupNearTray();
  void StyleAsPopup();
  void StyleAsHidden();
  void NotifyDartTrayEvent(const std::string& event);

  flutter::DartProject project_;
  std::unique_ptr<flutter::FlutterViewController> flutter_controller_;
  std::unique_ptr<flutter::MethodChannel<flutter::EncodableValue>> platform_channel_;

  static constexpr UINT WM_TRAY_ICON = WM_APP + 1;
  static constexpr UINT TRAY_ICON_ID = 1;
  NOTIFYICONDATAW nid_{};
  bool tray_created_ = false;
  bool popover_visible_ = false;
  LONG_PTR original_style_ = 0;
  LONG_PTR original_ex_style_ = 0;

  static constexpr int kPopupWidth = 320;
  static constexpr int kPopupHeight = 500;

  static constexpr const wchar_t kAutoStartKeyPath[] =
      L"Software\\Microsoft\\Windows\\CurrentVersion\\Run";
  static constexpr const wchar_t kAutoStartValueName[] = L"KeepAlive";
};

#endif  // RUNNER_FLUTTER_WINDOW_H_
