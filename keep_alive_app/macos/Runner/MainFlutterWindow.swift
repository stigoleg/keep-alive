import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
    override func awakeFromNib() {
        let flutterViewController = FlutterViewController()
        let windowFrame = self.frame
        self.contentViewController = flutterViewController
        self.setFrame(windowFrame, display: true)

        RegisterGeneratedPlugins(registry: flutterViewController)

        super.awakeFromNib()

        configureAsMenuBarPopover()
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

        self.orderOut(nil)

        guard let contentView = self.contentView else { return }
        contentView.wantsLayer = true

        let visualEffect = NSVisualEffectView(frame: contentView.bounds)
        visualEffect.autoresizingMask = [.width, .height]
        visualEffect.material = .menu
        visualEffect.blendingMode = .behindWindow
        visualEffect.state = .active
        visualEffect.wantsLayer = true
        visualEffect.layer?.cornerRadius = 12
        visualEffect.layer?.masksToBounds = true

        contentView.addSubview(visualEffect, positioned: .below, relativeTo: contentView.subviews.first)
    }

    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }
}
