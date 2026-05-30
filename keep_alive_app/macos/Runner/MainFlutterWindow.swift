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

        configureAsMenuBarApp()
    }

    private func configureAsMenuBarApp() {
        self.isOpaque = false
        self.hasShadow = true
        self.titleVisibility = .hidden
        self.titlebarAppearsTransparent = true
        self.styleMask = [.borderless, .fullSizeContentView]
        self.level = .floating
        self.collectionBehavior = [.transient, .ignoresCycle]
        self.isMovableByWindowBackground = false
        self.isReleasedWhenClosed = false

        self.backgroundColor = .clear

        guard let contentView = self.contentView else { return }

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
}
