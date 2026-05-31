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
        contentView.layer?.cornerRadius = 12
        contentView.layer?.masksToBounds = true
        contentView.layer?.borderWidth = 0.5
        contentView.layer?.borderColor = NSColor.separatorColor.cgColor

        let visualEffect = NSVisualEffectView(frame: contentView.bounds)
        visualEffect.autoresizingMask = [.width, .height]
        visualEffect.material = .menu
        visualEffect.blendingMode = .behindWindow
        visualEffect.state = .active
        visualEffect.wantsLayer = true
        visualEffect.layer?.cornerRadius = 12
        visualEffect.layer?.masksToBounds = true

        contentView.addSubview(visualEffect, positioned: .below, relativeTo: nil)
        self.visualEffectView = visualEffect

        if let flutterVC = contentViewController as? FlutterViewController {
            flutterView = flutterVC.view
            flutterView?.wantsLayer = true
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
