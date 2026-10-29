import XCTest
import ImageIO
import UniformTypeIdentifiers
@testable import PhotoFrame

final class PhotoFrameTests: XCTestCase {
    private func jpeg(_ id: String, in root: URL) throws {
        let context = CGContext(data: nil, width: 80, height: 120, bitsPerComponent: 8, bytesPerRow: 0,
                                space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
        let destination = CGImageDestinationCreateWithURL(root.appendingPathComponent(id + ".jpg") as CFURL,
            UTType.jpeg.identifier as CFString, 1, nil)!
        CGImageDestinationAddImage(destination, context.makeImage()!, nil)
        XCTAssertTrue(CGImageDestinationFinalize(destination))
    }
    func testSavingSelectionDoesNotDeletePreparedPhotoBeforeTimelineCommit() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = PhotoStore(root: root)
        let server = URL(string: "https://example.com")!
        try jpeg("current", in: root)
        try jpeg("incoming", in: root)
        try await store.prepare("incoming", server: server)
        // A selection save can run between prepare() and the producer's append.
        await store.save(Timeline(ids: ["current"]), server: server)
        let prepared = await store.image("incoming", edge: 240)
        XCTAssertNotNil(prepared, "cleanup deleted the producer's not-yet-committed image")
        await store.save(Timeline(ids: ["current", "incoming"]), server: server)
        let restored = await store.restore(server: server)
        XCTAssertEqual(restored.ids, ["current", "incoming"])
        await store.save(Timeline(ids: ["current"]), server: server)
        XCTAssertFalse(FileManager.default.fileExists(atPath: root.appendingPathComponent("incoming.jpg").path),
                       "committed images must eventually become evictable")
    }
    func testStaleSnapshotCannotDeleteNewerCommittedPhotos() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = PhotoStore(root: root), server = URL(string: "https://example.com")!
        try jpeg("a", in: root); try jpeg("b", in: root)
        await store.save(Timeline(ids: ["a", "b"], position: 1), server: server, revision: 20)
        // Simulate an older selection save being scheduled after a producer commit.
        await store.save(Timeline(ids: ["a"]), server: server, revision: 19)
        let restored = await store.restore(server: server)
        XCTAssertEqual(restored.current, "b")
        XCTAssertEqual(restored.ids, ["a", "b"])
    }
    @MainActor
    func testAutoplayRecoversWhenNextCachedImageDisappears() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = PhotoStore(root: root), server = URL(string: "http://127.0.0.1:9")!
        for id in ["a", "b", "c"] { try jpeg(id, in: root) }
        await store.save(Timeline(ids: ["a", "b", "c"]), server: server)
        let model = FrameModel(store: store)
        model.address = server.absoluteString
        model.start()
        defer { model.stop() }
        let restoreDeadline = Date().addingTimeInterval(5)
        while model.pictureID != "a" && Date() < restoreDeadline { try await Task.sleep(nanoseconds: 20_000_000) }
        XCTAssertEqual(model.pictureID, "a")
        // The file vanishes AFTER restoration, exactly as in the live incident.
        try FileManager.default.removeItem(at: root.appendingPathComponent("b.jpg"))
        let playbackDeadline = Date().addingTimeInterval(14)
        while model.pictureID != "c" && Date() < playbackDeadline { try await Task.sleep(nanoseconds: 50_000_000) }
        XCTAssertEqual(model.pictureID, "c", "a bad next image must not permanently stall the timer")
        XCTAssertEqual(model.recoveredPhotos, 1)
        XCTAssertFalse(model.timeline.ids.contains("b"))
        XCTAssertNotNil(model.picture)
        let diagnostics = await model.diagnosticSummary()
        XCTAssertTrue(diagnostics.contains("缺失 0 张"))
    }
    func testRemovingBrokenHistoryPreservesCurrentSelection() {
        var timeline = Timeline(ids: ["old", "current", "broken", "next"], position: 1)
        timeline.remove("broken")
        XCTAssertEqual(timeline.current, "current")
        timeline.remove("old")
        XCTAssertEqual(timeline.current, "current")
        XCTAssertEqual(timeline.ahead, 1)
    }
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
