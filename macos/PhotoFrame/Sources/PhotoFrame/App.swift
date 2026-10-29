import AppKit
import SwiftUI

@MainActor final class FrameWindow: NSWindow {
    weak var model: FrameModel?
    override func keyDown(with event: NSEvent) {
        if !event.modifierFlags.contains(.command), !event.modifierFlags.contains(.control),
           model?.key(event.keyCode) == true { return }
        super.keyDown(with: event)
    }
    override func sendEvent(_ event: NSEvent) {
        if event.type == .keyDown, !event.modifierFlags.contains(.command),
           !event.modifierFlags.contains(.control), model?.key(event.keyCode) == true { return }
        if event.type == .mouseMoved && event.locationInWindow.y < 180 && isKeyWindow {
            model?.reveal()
        }
        super.sendEvent(event)
    }
}

@MainActor final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate {
    let model = FrameModel()
    var frameWindow: FrameWindow?
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        makeMenu()
        let window = FrameWindow(contentRect: NSRect(x: 0, y: 0, width: 1100, height: 720),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView], backing: .buffered, defer: false)
        window.title = "照片相框"
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.backgroundColor = NSColor(calibratedWhite: 0.08, alpha: 1)
        window.minSize = NSSize(width: 480, height: 360)
        window.isReleasedWhenClosed = false
        window.acceptsMouseMovedEvents = true
        window.collectionBehavior = [.fullScreenPrimary]
        window.tabbingMode = .disallowed
        window.delegate = self; window.model = model; model.window = window
        window.contentView = NSHostingView(rootView: FrameView(model: model))
        window.setFrameAutosaveName("PhotoFrameMainWindow")
        if !window.setFrameUsingName("PhotoFrameMainWindow") { window.center() }
        frameWindow = window
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: false)
        model.start()
    }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        frameWindow?.makeKeyAndOrderFront(nil); return true
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
    private func makeMenu() {
        let menu = NSMenu()
        let app = NSMenuItem(); menu.addItem(app)
        let appMenu = NSMenu(title: "照片相框"); app.submenu = appMenu
        appMenu.addItem(withTitle: "关于照片相框", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        let settings = appMenu.addItem(withTitle: "设置…", action: #selector(showSettings), keyEquivalent: ","); settings.target = self
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "隐藏照片相框", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        appMenu.addItem(withTitle: "退出照片相框", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        let edit = NSMenuItem(); menu.addItem(edit); edit.submenu = NSMenu(title: "编辑")
        for (title, selector, key) in [("撤销", "undo:", "z"), ("剪切", "cut:", "x"), ("复制", "copy:", "c"), ("粘贴", "paste:", "v"), ("全选", "selectAll:", "a")] {
            edit.submenu?.addItem(withTitle: title, action: Selector(selector), keyEquivalent: key)
        }
        let view = NSMenuItem(); menu.addItem(view); view.submenu = NSMenu(title: "显示")
        let full = view.submenu!.addItem(withTitle: "进入或退出全屏", action: #selector(NSWindow.toggleFullScreen(_:)), keyEquivalent: "f")
        full.keyEquivalentModifierMask = [.command, .control]
        view.submenu?.addItem(withTitle: "最小化", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
        NSApp.mainMenu = menu
    }
    @objc private func showSettings() { model.settings = true; frameWindow?.makeKeyAndOrderFront(nil) }
}

@main struct PhotoFrameApp {
    static func main() {
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.delegate = delegate
        withExtendedLifetime(delegate) { app.run() }
    }
}
