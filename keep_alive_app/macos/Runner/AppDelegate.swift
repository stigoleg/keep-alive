import Cocoa
import FlutterMacOS
import ServiceManagement

private let kPlatformChannelName = "com.stigoleg.keepAliveApp/platform"
private let kLaunchAgentLabel = "com.stigoleg.keepalive"
private let kTrayIconSize: CGFloat = 18.0
private let kPopoverWidth: CGFloat = 300.0
private let kPopoverHeight: CGFloat = 480.0

@main
class AppDelegate: FlutterAppDelegate, NSWindowDelegate {
    private var statusItem: NSStatusItem?
    private var contextMenuResult: FlutterResult?
    private weak var flutterChannel: FlutterMethodChannel?
    private var popoverVisible = false

    override func applicationDidFinishLaunching(_ aNotification: Notification) {
        guard let controller = mainFlutterWindow?.contentViewController as? FlutterViewController else {
            return
        }

        let channel = FlutterMethodChannel(
            name: kPlatformChannelName,
            binaryMessenger: controller.engine.binaryMessenger
        )
        flutterChannel = channel

        channel.setMethodCallHandler { [weak self] call, result in
            self?.handleMethodCall(call, result: result)
        }

        mainFlutterWindow?.delegate = self
    }

    override func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        return false
    }

    override func applicationWillTerminate(_ notification: Notification) {
        hidePopover()
        if let channel = flutterChannel {
            channel.invokeMethod("systemShutdown", arguments: nil)
        }
    }

    override func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool {
        return true
    }

    // MARK: - NSWindowDelegate

    func windowDidResignKey(_ notification: Notification) {
        guard let window = notification.object as? NSWindow,
              window == mainFlutterWindow,
              popoverVisible else { return }

        hidePopover()
        flutterChannel?.invokeMethod("onTrayEvent", arguments: "popoverDismissed")
    }

    // MARK: - Method Channel Handler

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
            handleShowPopover(result: result)
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
        guard let args = call.arguments as? [String: Any],
              let enabled = args["enabled"] as? Bool else {
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
                setLaunchAgent(enabled: enabled)
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

    // MARK: - Asset Path Resolution

    private func resolveAssetPath(_ assetKey: String) -> String? {
        let lookupKey = FlutterDartProject.lookupKey(forAsset: assetKey)
        let nsKey = lookupKey as NSString
        let directory = nsKey.deletingLastPathComponent
        let filename = nsKey.lastPathComponent
        let name = (filename as NSString).deletingPathExtension
        let ext = (filename as NSString).pathExtension
        return Bundle.main.path(
            forResource: name,
            ofType: ext.isEmpty ? nil : ext,
            inDirectory: directory.isEmpty ? nil : directory
        )
    }

    // MARK: - Tray Icon

    private func handleSetTrayIcon(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let args = call.arguments as? [String: Any],
              let iconPath = args["iconPath"] as? String else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing 'iconPath' argument", details: nil))
            return
        }

        guard let resolvedPath = resolveAssetPath(iconPath) else {
            result(FlutterError(code: "ASSET_NOT_FOUND", message: "Could not resolve asset: \(iconPath)", details: nil))
            return
        }

        if statusItem == nil {
            statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
            statusItem?.button?.target = self
            statusItem?.button?.action = #selector(statusBarButtonClicked(_:))
            statusItem?.button?.sendAction(on: [.leftMouseUp, .rightMouseUp])
        }

        if let image = NSImage(contentsOfFile: resolvedPath) {
            image.isTemplate = true
            image.size = NSSize(width: kTrayIconSize, height: kTrayIconSize)
            statusItem?.button?.image = image
            statusItem?.button?.imagePosition = .imageOnly
        }

        result(nil)
    }

    private func handleSetTrayTooltip(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let args = call.arguments as? [String: Any],
              let tooltip = args["tooltip"] as? String else {
            result(FlutterError(code: "INVALID_ARG", message: "Missing 'tooltip' argument", details: nil))
            return
        }
        statusItem?.button?.toolTip = tooltip
        result(nil)
    }

    @objc private func statusBarButtonClicked(_ sender: NSStatusBarButton) {
        guard let event = NSApp.currentEvent else { return }

        if event.type == .rightMouseUp {
            hidePopover()
            flutterChannel?.invokeMethod("onTrayEvent", arguments: "rightClick")
        } else {
            flutterChannel?.invokeMethod("onTrayEvent", arguments: "leftClick")
        }
    }

    // MARK: - Context Menu

    private func handleShowContextMenu(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        guard let args = call.arguments as? [String: Any],
              let items = args["items"] as? [String] else {
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

    // MARK: - Popover (Window-Based)

    private func handleShowPopover(result: @escaping FlutterResult) {
        guard let window = mainFlutterWindow,
              let statusButton = statusItem?.button else {
            result(FlutterError(code: "NO_WINDOW", message: "Window or tray icon not ready", details: nil))
            return
        }

        let statusFrame = statusButton.window?.convertToScreen(statusButton.convert(statusButton.bounds, to: nil))
        let screenFrame = NSScreen.main?.visibleFrame ?? .zero

        let anchorX: CGFloat
        let anchorY: CGFloat
        if let frame = statusFrame {
            anchorX = frame.midX - kPopoverWidth / 2
            anchorY = frame.minY - kPopoverHeight - 4
        } else {
            anchorX = screenFrame.maxX - kPopoverWidth - 16
            anchorY = screenFrame.maxY - kPopoverHeight - 4
        }

        var x = max(screenFrame.minX + 8, anchorX)
        var y = max(screenFrame.minY, anchorY)
        if x + kPopoverWidth > screenFrame.maxX {
            x = screenFrame.maxX - kPopoverWidth - 8
        }
        if y + kPopoverHeight > screenFrame.maxY {
            y = statusFrame?.maxY ?? screenFrame.maxY - kPopoverHeight
            if let frame = statusFrame {
                y = frame.maxY + 4
            }
        }

        window.setFrame(NSRect(x: x, y: y, width: kPopoverWidth, height: kPopoverHeight), display: true)
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        popoverVisible = true
        result(nil)
    }

    private func handleHidePopover(result: @escaping FlutterResult) {
        hidePopover()
        result(nil)
    }

    private func hidePopover() {
        guard popoverVisible else { return }
        mainFlutterWindow?.orderOut(nil)
        popoverVisible = false
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
