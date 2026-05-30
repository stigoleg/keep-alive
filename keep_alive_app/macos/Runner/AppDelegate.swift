import Cocoa
import FlutterMacOS
import ServiceManagement

private let kPlatformChannelName = "com.stigoleg.keepAliveApp/platform"
private let kLaunchAgentLabel = "com.stigoleg.keepalive"
private let kTrayIconSize: CGFloat = 18.0

@main
class AppDelegate: FlutterAppDelegate {
    private var statusItem: NSStatusItem?
    private var popover: NSPopover?
    private var contextMenuResult: FlutterResult?

    override func applicationDidFinishLaunching(_ aNotification: Notification) {
        guard let controller = mainFlutterWindow?.contentViewController as? FlutterViewController else {
            return
        }

        let channel = FlutterMethodChannel(
            name: kPlatformChannelName,
            binaryMessenger: controller.engine.binaryMessenger
        )

        channel.setMethodCallHandler { [weak self] call, result in
            self?.handleMethodCall(call, result: result)
        }
    }

    override func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        return false
    }

    override func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool {
        return true
    }

    private func handleMethodCall(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        switch call.method {
        case "getPlatformName":
            result("macOS")
        case "setAutoStart":
            handleSetAutoStart(call, result: result)
        case "isAutoStartEnabled":
            handleIsAutoStartEnabled(result: result)
        case "setTrayIcon":
            handleSetTrayIcon(call, result: result)
        case "setTrayTooltip":
            handleSetTrayTooltip(call, result: result)
        case "showContextMenu":
            handleShowContextMenu(call, result: result)
        case "showPopover":
            handleShowPopover(call, result: result)
        case "hidePopover":
            handleHidePopover(result: result)
        case "getAppSupportDir":
            handleGetAppSupportDir(result: result)
        default:
            result(FlutterMethodNotImplemented)
        }
    }

    // MARK: - Auto Start

    private func handleSetAutoStart(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let enabled = (call.arguments as? [String: Any])?["enabled"] as? Bool else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing 'enabled' argument", details: nil))
            return
        }

        if #available(macOS 13.0, *) {
            do {
                if enabled {
                    try SMAppService.mainApp.register()
                } else {
                    try SMAppService.mainApp.unregister()
                }
                result(nil)
            } catch {
                self.setLaunchAgent(enabled: enabled)
                result(nil)
            }
        } else {
            setLaunchAgent(enabled: enabled)
            result(nil)
        }
    }

    private func setLaunchAgent(enabled: Bool) {
        let launchAgentsDir = FileManager.default
            .homeDirectoryForCurrentUser
            .appendingPathComponent("Library/LaunchAgents")
        let plistPath = launchAgentsDir
            .appendingPathComponent("\(kLaunchAgentLabel).plist")

        if enabled {
            try? FileManager.default.createDirectory(at: launchAgentsDir, withIntermediateDirectories: true)

            let bundlePath = Bundle.main.bundlePath
            let plistContent = """
                <?xml version="1.0" encoding="UTF-8"?>
                <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" \
                "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
                <plist version="1.0">
                <dict>
                    <key>Label</key>
                    <string>\(kLaunchAgentLabel)</string>
                    <key>ProgramArguments</key>
                    <array>
                        <string>\(bundlePath)/Contents/MacOS/keep_alive_app</string>
                    </array>
                    <key>RunAtLoad</key>
                    <true/>
                </dict>
                </plist>
                """
            try? plistContent.write(to: plistPath, atomically: true, encoding: .utf8)
        } else {
            try? FileManager.default.removeItem(at: plistPath)
        }
    }

    private func handleIsAutoStartEnabled(result: @escaping FlutterResult) {
        if #available(macOS 13.0, *) {
            result(SMAppService.mainApp.status == .enabled)
        } else {
            let plistPath = FileManager.default
                .homeDirectoryForCurrentUser
                .appendingPathComponent("Library/LaunchAgents/\(kLaunchAgentLabel).plist")
            result(FileManager.default.fileExists(atPath: plistPath.path))
        }
    }

    // MARK: - Tray Icon

    private func handleSetTrayIcon(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let iconPath = (call.arguments as? [String: Any])?["iconPath"] as? String else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing 'iconPath' argument", details: nil))
            return
        }

        if statusItem == nil {
            statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        }

        if let image = NSImage(contentsOfFile: iconPath) {
            image.isTemplate = true
            image.size = NSSize(width: kTrayIconSize, height: kTrayIconSize)
            statusItem?.button?.image = image
            statusItem?.button?.imagePosition = .imageOnly
        }

        result(nil)
    }

    private func handleSetTrayTooltip(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let tooltip = (call.arguments as? [String: Any])?["tooltip"] as? String else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing 'tooltip' argument", details: nil))
            return
        }
        statusItem?.button?.toolTip = tooltip
        result(nil)
    }

    // MARK: - Context Menu

    private func handleShowContextMenu(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let items = (call.arguments as? [String: Any])?["items"] as? [String] else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing 'items' argument", details: nil))
            return
        }

        let menu = NSMenu()
        menu.autoenablesItems = false

        for (index, title) in items.enumerated() {
            if title == "-" {
                menu.addItem(NSMenuItem.separator())
            } else {
                let item = NSMenuItem(title: title, action: #selector(contextMenuItemSelected(_:)), keyEquivalent: "")
                item.tag = index
                item.target = self
                item.isEnabled = true
                menu.addItem(item)
            }
        }

        contextMenuResult = result
        statusItem?.menu = menu
        statusItem?.button?.performClick(nil)
    }

    @objc private func contextMenuItemSelected(_ sender: NSMenuItem) {
        let selectedIndex = sender.tag
        statusItem?.menu = nil
        contextMenuResult?(selectedIndex)
        contextMenuResult = nil
    }

    // MARK: - Popover

    private func handleShowPopover(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let args = call.arguments as? [String: Any],
              let x = args["x"] as? Double,
              let y = args["y"] as? Double else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing position arguments", details: nil))
            return
        }

        guard let statusButton = statusItem?.button else {
            result(FlutterError(code: "NO_TRAY", message: "Tray icon not initialized", details: nil))
            return
        }

        if popover == nil {
            popover = NSPopover()
            popover?.behavior = .transient
            popover?.animates = true
        }

        if let flutterView = mainFlutterWindow?.contentViewController?.view {
            let popupVC = NSViewController()
            popupVC.view = NSView(frame: NSRect(x: 0, y: 0, width: 300, height: 400))
            flutterView.frame = popupVC.view.bounds
            flutterView.autoresizingMask = [.width, .height]
            popupVC.view.addSubview(flutterView)
            popover?.contentViewController = popupVC
        }

        popover?.show(relativeTo: statusButton.bounds, of: statusButton, preferredEdge: .minY)
        result(nil)
    }

    private func handleHidePopover(result: @escaping FlutterResult) {
        popover?.performClose(nil)
        result(nil)
    }

    // MARK: - App Support Dir

    private func handleGetAppSupportDir(result: @escaping FlutterResult) {
        let paths = NSSearchPathForDirectoriesInDomains(.applicationSupportDirectory, .userDomainMask, true)
        if let appSupport = paths.first {
            result(appSupport)
        } else {
            result(FlutterError(code: "DIR_ERROR", message: "Could not resolve application support directory", details: nil))
        }
    }
}
