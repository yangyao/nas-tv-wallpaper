import AppKit
import Combine
import OSLog

@MainActor final class FrameModel: ObservableObject {
    @Published private(set) var timeline = Timeline()
    @Published private(set) var thumbnails: [String: NSImage] = [:]
    @Published private(set) var picture: NSImage?
    @Published private(set) var pictureID = ""
    @Published var controls = false
    @Published var playing = true
    @Published var settings = false
    @Published var address = UserDefaults.standard.string(forKey: "serverURL") ?? ""
    var window: NSWindow?
    private let store: PhotoStore
    private var producer: Task<Void, Never>?
    private var timer: Task<Void, Never>?
    private var hideTask: Task<Void, Never>?
    private var selection: Task<Void, Never>?
    private var generation = UUID()
    private var lastChange = Date()
    private var server: URL?
    private let log = Logger(subsystem: "com.yangyao.photoframe", category: "photos")

    init(store: PhotoStore = PhotoStore()) { self.store = store }
    func start() {
        guard let url = ServerAddress.parse(address) else { settings = true; return }
        connect(url)
    }
    func configure() {
        guard let url = ServerAddress.parse(address) else { return }
        UserDefaults.standard.set(url.absoluteString, forKey: "serverURL")
        settings = false
        connect(url)
    }
    private func connect(_ url: URL) {
        producer?.cancel(); selection?.cancel(); timer?.cancel()
        let run = UUID(); generation = run; server = url
        timeline = Timeline(); thumbnails = [:]
        // Retain the existing photograph until the new endpoint has a decoded replacement.
        producer = Task {
            let restored = await store.restore(server: url)
            guard !Task.isCancelled, generation == run else { return }
            timeline = restored
            if let id = timeline.current, let cg = await store.image(id, edge: 2560), generation == run {
                picture = NSImage(cgImage: cg, size: .zero); pictureID = id; lastChange = Date()
            }
            for id in restored.ids {
                let cg = await store.image(id, edge: 240)
                guard !Task.isCancelled, generation == run else { return }
                if let cg { thumbnails[id] = NSImage(cgImage: cg, size: .zero) }
            }
            // Remove orphaned files left by an interrupted previous download.
            await store.save(timeline, server: url)
            while !Task.isCancelled && generation == run {
                if timeline.ahead < 20 {
                    let excluded = [timeline.current].compactMap { $0 } + timeline.ids.filter { $0 != timeline.current }
                    do {
                        let candidates = try await store.batch(server: url, exclude: excluded)
                        for candidate in candidates {
                            try Task.checkCancellation()
                            guard generation == run else { return }
                            if timeline.ids.contains(candidate.id) || timeline.ahead >= 20 { continue }
                            do {
                                try await store.prepare(candidate.id, server: url)
                                let thumb = await store.image(candidate.id, edge: 240)
                                guard !Task.isCancelled, generation == run else { return }
                                guard let thumb else { continue }
                                timeline.append(candidate.id).forEach { thumbnails.removeValue(forKey: $0) }
                                thumbnails[candidate.id] = NSImage(cgImage: thumb, size: .zero)
                                if picture == nil || pictureID != timeline.current && timeline.ids.count == 1 {
                                    if let cg = await store.image(candidate.id, edge: 2560), generation == run {
                                        picture = NSImage(cgImage: cg, size: .zero); pictureID = candidate.id; lastChange = Date()
                                    }
                                }
                                await store.save(timeline, server: url)
                            } catch {
                                if Task.isCancelled { return }
                                log.error("Photo preparation failed: \(String(describing: type(of: error)), privacy: .public) code \((error as NSError).code)")
                            }
                        }
                    } catch {
                        if Task.isCancelled { return }
                        log.error("Batch unavailable: \(String(describing: type(of: error)), privacy: .public) code \((error as NSError).code)")
                    }
                }
                try? await Task.sleep(nanoseconds: timeline.ahead < 6 ? 3_000_000_000 : 8_000_000_000)
            }
        }
        timer = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 500_000_000)
                guard !Task.isCancelled else { return }
                if playing && !controls && !settings && Date().timeIntervalSince(lastChange) >= 10 {
                    if timeline.ahead > 0 { move(1) }
                }
            }
        }
    }
    func select(_ index: Int) {
        guard timeline.ids.indices.contains(index), index != timeline.position, let server else { return }
        let id = timeline.ids[index], run = generation
        selection?.cancel()
        selection = Task {
            guard let cg = await store.image(id, edge: 2560), !Task.isCancelled, generation == run,
                  let actual = timeline.ids.firstIndex(of: id) else { return }
            timeline.select(actual)
            picture = NSImage(cgImage: cg, size: .zero); pictureID = id; lastChange = Date()
            await store.save(timeline, server: server)
        }
    }
    func move(_ delta: Int) { select(timeline.position + delta) }
    func reveal() {
        controls = true; hideTask?.cancel()
        hideTask = Task {
            try? await Task.sleep(nanoseconds: 5_000_000_000)
            guard !Task.isCancelled else { return }
            hide()
        }
    }
    func hide() { controls = false; hideTask?.cancel(); lastChange = Date() }
    func togglePlayback() { playing.toggle(); lastChange = Date(); reveal() }
    func key(_ code: UInt16) -> Bool {
        guard !settings else { return false }
        switch code {
        case 123: move(-1)
        case 124: move(1)
        case 125: reveal()
        case 126: hide()
        case 49: togglePlayback()
        case 3: window?.toggleFullScreen(nil)
        case 53:
            if controls { hide() }
            else if window?.styleMask.contains(.fullScreen) == true { window?.toggleFullScreen(nil) }
        default: return false
        }
        return true
    }
}
