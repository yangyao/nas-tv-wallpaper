import AppKit
import ImageIO
import UniformTypeIdentifiers

struct PhotoCandidate: Decodable { let id: String }
struct PhotoBatch: Decodable { let photos: [PhotoCandidate] }
struct SavedSession: Codable { let server: String; let timeline: Timeline }

enum FrameError: Error { case invalidResponse, oversized, invalidImage }

// All disk/image work is isolated from the main thread. Originals are temporary;
// only normalized display JPEGs survive a completed download.
actor PhotoStore {
    private let root: URL
    private let session: URLSession
    private var staged = Set<String>()
    private var committed = Set<String>()
    private var latestRevision: UInt64 = 0
    init(root: URL? = nil) {
        self.root = root ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("PhotoFrame", isDirectory: true)
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 25
        config.timeoutIntervalForResource = 60
        config.urlCache = nil
        config.httpMaximumConnectionsPerHost = 2
        session = URLSession(configuration: config, delegate: LANRedirectGuard(), delegateQueue: nil)
        try? FileManager.default.createDirectory(at: self.root, withIntermediateDirectories: true)
        try? FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: self.root.path)
        var directory = self.root
        var values = URLResourceValues(); values.isExcludedFromBackup = true
        try? directory.setResourceValues(values)
    }
    private func file(_ id: String) -> URL? {
        guard !id.isEmpty, id.count <= 128,
              id.allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "-" || $0 == "_") }) else { return nil }
        return root.appendingPathComponent(id + ".jpg")
    }
    func restore(server: URL) -> Timeline {
        guard let data = try? Data(contentsOf: root.appendingPathComponent("session.json")),
              let saved = try? JSONDecoder().decode(SavedSession.self, from: data),
              saved.server == server.absoluteString else { return Timeline() }
        let valid = Array(saved.timeline.ids.prefix(60)).filter { id in
            guard let url = file(id) else { return false }
            return FileManager.default.fileExists(atPath: url.path) && image(id, edge: 240) != nil
        }
        let old = saved.timeline.current
        return Timeline(ids: valid, position: old.flatMap { valid.firstIndex(of: $0) } ?? 0)
    }
    func save(_ timeline: Timeline, server: URL, revision: UInt64? = nil) {
        let incoming = revision ?? latestRevision + 1
        guard incoming > latestRevision else { return }
        latestRevision = incoming
        committed = Set(timeline.ids)
        staged.subtract(committed)
        if let data = try? JSONEncoder().encode(SavedSession(server: server.absoluteString, timeline: timeline)) {
            let destination = root.appendingPathComponent("session.json")
            try? data.write(to: destination, options: .atomic)
            try? FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: destination.path)
        }
        let keep = Set(committed.union(staged).map { $0 + ".jpg" })
        for url in (try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: nil)) ?? [] {
            if url.pathExtension == "jpg" && !keep.contains(url.lastPathComponent) { try? FileManager.default.removeItem(at: url) }
        }
    }
    func batch(server: URL, exclude: [String]) async throws -> [PhotoCandidate] {
        var request = URLRequest(url: server.appendingPathComponent("frame/api/batch"))
        request.httpMethod = "POST"
        request.setValue(server.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")), forHTTPHeaderField: "Origin")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: ["count": 8, "exclude": Array(exclude.prefix(60))])
        let (data, response) = try await session.data(for: request)
        guard (response as? HTTPURLResponse)?.statusCode == 200, data.count < 256 * 1024 else { throw FrameError.invalidResponse }
        return Array(try JSONDecoder().decode(PhotoBatch.self, from: data).photos.prefix(8))
    }
    func prepare(_ id: String, server: URL) async throws {
        try Task.checkCancellation()
        guard let destination = file(id) else { throw FrameError.invalidResponse }
        if FileManager.default.fileExists(atPath: destination.path) { staged.insert(id); return }
        let url = server.appendingPathComponent("frame/photos").appendingPathComponent(id)
        let (temporary, response) = try await session.download(from: url)
        defer { try? FileManager.default.removeItem(at: temporary) }
        try Task.checkCancellation()
        guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw FrameError.invalidResponse }
        let size = try temporary.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? Int.max
        guard size <= 40 * 1024 * 1024 else { throw FrameError.oversized }
        guard let source = CGImageSourceCreateWithURL(temporary as CFURL, [kCGImageSourceShouldCache: false] as CFDictionary),
              let props = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
              let w = props[kCGImagePropertyPixelWidth] as? Int, let h = props[kCGImagePropertyPixelHeight] as? Int,
              w > 0, h > 0, Double(w) * Double(h) <= 40_000_000 else { throw FrameError.invalidImage }
        // ImageIO applies orientation once, before display and thumbnail generation.
        let orientation = props[kCGImagePropertyOrientation] as? Int ?? 1
        let rotated = (5...8).contains(orientation)
        let displayWidth = rotated ? h : w, displayHeight = rotated ? w : h
        let edge = max(w, h)
        let scale = min(1, min(2560.0 / Double(displayWidth), 1440.0 / Double(displayHeight)))
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceThumbnailMaxPixelSize: Int(Double(edge) * scale),
            kCGImageSourceShouldCacheImmediately: true
        ] as CFDictionary) else { throw FrameError.invalidImage }
        for quality in [0.86, 0.72, 0.55, 0.35] {
            let bytes = NSMutableData()
            guard let encoder = CGImageDestinationCreateWithData(bytes, UTType.jpeg.identifier as CFString, 1, nil) else { throw FrameError.invalidImage }
            CGImageDestinationAddImage(encoder, image, [kCGImageDestinationLossyCompressionQuality: quality] as CFDictionary)
            guard CGImageDestinationFinalize(encoder) else { throw FrameError.invalidImage }
            if bytes.length <= 2 * 1024 * 1024 {
                try (bytes as Data).write(to: destination, options: .atomic)
                try? FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: destination.path)
                staged.insert(id)
                return
            }
        }
        throw FrameError.oversized
    }
    func image(_ id: String, edge: Int) -> CGImage? {
        guard let url = file(id), FileManager.default.fileExists(atPath: url.path),
              let source = CGImageSourceCreateWithURL(url as CFURL, nil) else { return nil }
        return CGImageSourceCreateThumbnailAtIndex(source, 0, [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceThumbnailMaxPixelSize: edge,
            kCGImageSourceShouldCacheImmediately: true
        ] as CFDictionary)
    }
    func discardPrepared(_ id: String) {
        staged.remove(id)
        if !committed.contains(id), let url = file(id) { try? FileManager.default.removeItem(at: url) }
    }
    func invalidate(_ id: String) {
        staged.remove(id)
        if let url = file(id) { try? FileManager.default.removeItem(at: url) }
    }
    func missingCount(_ ids: [String]) -> Int {
        ids.filter { id in file(id).map { !FileManager.default.fileExists(atPath: $0.path) } ?? true }.count
    }
}

// Never follow redirects to a different origin (or silently downgrade HTTPS).
final class LANRedirectGuard: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        let a = task.originalRequest?.url, b = request.url
        completionHandler(a?.scheme == b?.scheme && a?.host == b?.host && a?.port == b?.port ? request : nil)
    }
}
