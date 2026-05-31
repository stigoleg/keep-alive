import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
    private var visualEffectView: NSVisualEffectView?
    private var flutterView: NSView?

    override func awakeFromNib() {
        let flutterViewController = FlutterViewController()
        let windowFrame = self.frame
        self.contentViewController = flutterViewController
        self.setFrame(windowFrame, display: true)

        RegisterGeneratedPlugins(registry: flutterViewController)

        super.awakeFromNib()

        configureAsMenuBarPopover()

        self.alphaValue = 0.0
    }

    private func configureAsMenuBarPopover() {
        self.isOpaque = false
        self.backgroundColor = .clear
        self.hasShadow = true
        self.titleVisibility = .hidden
        self.titlebarAppearsTransparent = true
        self.styleMask = [.borderless, .fullSizeContentView]
        self.level = .floating
        self.collectionBehavior = [.transient, .ignoresCycle]
        self.isMovableByWindowBackground = false
        self.isReleasedWhenClosed = false

        guard let contentView = self.contentView else { return }
        contentView.wantsLayer = true
        contentView.layer?.backgroundColor = NSColor.clear.cgColor
        contentView.layer?.cornerRadius = 12
        contentView.layer?.masksToBounds = true
        contentView.layer?.borderWidth = 0.5
        contentView.layer?.borderColor = NSColor.separatorColor.cgColor

        // Prefer NSGlassEffectView (Liquid Glass) on macOS 26+ when the
        // running system + SDK support it. We look the class up dynamically
        // so the build still succeeds on older Xcode SDKs.
        let glassClass: AnyClass? = NSClassFromString("NSGlassEffectView")
        if let cls = glassClass as? NSView.Type {
            let glass = cls.init(frame: contentView.bounds)
            glass.autoresizingMask = [.width, .height]
            glass.wantsLayer = true
            glass.layer?.cornerRadius = 12
            glass.layer?.masksToBounds = true
            contentView.addSubview(glass, positioned: .below, relativeTo: nil)
        } else {
            let visualEffect = NSVisualEffectView(frame: contentView.bounds)
            visualEffect.autoresizingMask = [.width, .height]
            // .popover is the modern menu-bar popover material; macOS 26
            // auto-uplifts this to Liquid Glass styling at runtime.
            visualEffect.material = .popover
            visualEffect.blendingMode = .behindWindow
            visualEffect.state = .active
            visualEffect.wantsLayer = true
            visualEffect.layer?.cornerRadius = 12
            visualEffect.layer?.masksToBounds = true
            contentView.addSubview(visualEffect, positioned: .below, relativeTo: nil)
            self.visualEffectView = visualEffect
        }

        if let flutterVC = contentViewController as? FlutterViewController {
            flutterView = flutterVC.view
            flutterView?.wantsLayer = true
            flutterView?.layer?.backgroundColor = NSColor.clear.cgColor
        }
    }

    func animateShow() {
        NSAnimationContext.runAnimationGroup { context in
            context.duration = 0.12
            context.timingFunction = CAMediaTimingFunction(name: .easeOut)
            self.animator().alphaValue = 1.0
        }
    }

    func animateHide(completion: (() -> Void)? = nil) {
        NSAnimationContext.runAnimationGroup({ context in
            context.duration = 0.08
            context.timingFunction = CAMediaTimingFunction(name: .easeIn)
            self.animator().alphaValue = 0.0
        }, completionHandler: {
            self.orderOut(nil)
            completion?()
        })
    }

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { true }
}
