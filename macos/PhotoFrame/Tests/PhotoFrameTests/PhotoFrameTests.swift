import XCTest
import ImageIO
import UniformTypeIdentifiers
@testable import PhotoFrame

final class PhotoFrameTests: XCTestCase {
    func testUnavailableNavigationDoesNotQueueOrChangeSelection() {
        var t = Timeline(ids: ["a", "b"], position: 1)
        for _ in 0..<100 { t.select(2) }
        XCTAssertEqual(t.current, "b")
        _ = t.append("c")
        XCTAssertEqual(t.current, "b")
    }
    func testLongSessionKeepsBoundedHistoryAndPreservesCurrent() {
        var t = Timeline()
        for i in 0..<1000 {
            let current = t.current
            _ = t.append("photo-\(i)")
            if let current { XCTAssertEqual(t.current, current) }
            t.select(t.ids.count - 1)
            XCTAssertLessThanOrEqual(t.ids.count, 60)
        }
        XCTAssertEqual(t.current, "photo-999")
        XCTAssertEqual(Set(t.ids).count, t.ids.count)
    }
    func testAddressRejectsCredentialsAndNonHTTP() {
        XCTAssertNil(ServerAddress.parse("file:///tmp/photo"))
        XCTAssertNil(ServerAddress.parse("https://user:secret@example.com"))
        XCTAssertNil(ServerAddress.parse("https://example.com?token=secret"))
        XCTAssertEqual(ServerAddress.parse("https://example.com:8443/frame/")?.absoluteString, "https://example.com:8443")
    }
    func testCachePrunesOrphansAndSeparatesServers() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = PhotoStore(root: root)
        try Data([1, 2, 3]).write(to: root.appendingPathComponent("orphan.jpg"))
        let server = URL(string: "https://example.com")!
        await store.save(Timeline(), server: server)
        XCTAssertFalse(FileManager.default.fileExists(atPath: root.appendingPathComponent("orphan.jpg").path))
        let restored = await store.restore(server: URL(string: "https://other.example.com")!)
        XCTAssertTrue(restored.ids.isEmpty)
        let invalid = await store.image("../../outside", edge: 240)
        XCTAssertNil(invalid)
    }
    func testOfflineRestoreKeepsSelectedPhotoAndRejectsBrokenFiles() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = PhotoStore(root: root)
        let context = CGContext(data: nil, width: 80, height: 120, bitsPerComponent: 8, bytesPerRow: 0,
                                space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
        let image = context.makeImage()!
        for id in ["a", "b"] {
            let destination = CGImageDestinationCreateWithURL(root.appendingPathComponent(id + ".jpg") as CFURL, UTType.jpeg.identifier as CFString, 1, nil)!
            CGImageDestinationAddImage(destination, image, nil)
            XCTAssertTrue(CGImageDestinationFinalize(destination))
        }
        try Data([1, 2, 3]).write(to: root.appendingPathComponent("broken.jpg"))
        let server = URL(string: "https://example.com")!
        await store.save(Timeline(ids: ["a", "broken", "b"], position: 2), server: server)
        let reopened = PhotoStore(root: root)
        let restored = await reopened.restore(server: server)
        XCTAssertEqual(restored.ids, ["a", "b"])
        XCTAssertEqual(restored.current, "b")
        let decoded = await reopened.image("b", edge: 80)
        XCTAssertEqual(decoded?.height, 80)
    }
}
