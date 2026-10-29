import AppKit

let output = URL(fileURLWithPath: CommandLine.arguments[1])
let folder = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".iconset")
try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
defer { try? FileManager.default.removeItem(at: folder) }
for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let edge = size * scale
        let image = NSImage(size: NSSize(width: edge, height: edge))
        image.lockFocus()
        let context = NSGraphicsContext.current!.cgContext
        context.scaleBy(x: CGFloat(edge) / 1024, y: CGFloat(edge) / 1024)
        let frame = NSBezierPath(roundedRect: NSRect(x: 72, y: 72, width: 880, height: 880), xRadius: 206, yRadius: 206)
        frame.addClip()
        NSGradient(starting: NSColor(calibratedRed: 0.08, green: 0.25, blue: 0.34, alpha: 1), ending: NSColor(calibratedRed: 0.39, green: 0.64, blue: 0.65, alpha: 1))!.draw(in: frame, angle: 80)
        NSColor(calibratedRed: 1, green: 0.86, blue: 0.63, alpha: 1).setFill()
        NSBezierPath(ovalIn: NSRect(x: 624, y: 594, width: 133, height: 133)).fill()
        let hills = NSBezierPath()
        hills.move(to: NSPoint(x: 72, y: 72)); hills.line(to: NSPoint(x: 72, y: 355))
        hills.curve(to: NSPoint(x: 650, y: 320), controlPoint1: NSPoint(x: 330, y: 615), controlPoint2: NSPoint(x: 370, y: 300))
        hills.curve(to: NSPoint(x: 952, y: 415), controlPoint1: NSPoint(x: 795, y: 335), controlPoint2: NSPoint(x: 780, y: 565))
        hills.line(to: NSPoint(x: 952, y: 72)); hills.close()
        NSColor(calibratedRed: 0.13, green: 0.34, blue: 0.36, alpha: 1).setFill(); hills.fill()
        let rim = NSBezierPath(roundedRect: NSRect(x: 168, y: 197, width: 688, height: 630), xRadius: 80, yRadius: 80)
        NSColor.white.withAlphaComponent(0.82).setStroke(); rim.lineWidth = 28; rim.stroke()
        image.unlockFocus()
        let rep = NSBitmapImageRep(data: image.tiffRepresentation!)!
        let name = "icon_\(size)x\(size)\(scale == 2 ? "@2x" : "").png"
        try rep.representation(using: .png, properties: [:])!.write(to: folder.appendingPathComponent(name))
    }
}
let process = Process(); process.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
process.arguments = ["-c", "icns", folder.path, "-o", output.path]
try process.run(); process.waitUntilExit()
if process.terminationStatus != 0 { exit(process.terminationStatus) }
