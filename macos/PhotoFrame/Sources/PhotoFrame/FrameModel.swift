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
    private var storeRevision: UInt64 = 0
    private var selectionID: UUID?
    private(set) var recoveredPhotos = 0
    private(set) var lastNetworkError = "无"
    private let log = Logger(subsystem: "com.yangyao.photoframe", category: "photos")

    init(store: PhotoStore = PhotoStore()) { self.store = store }
    func stop() {
        generation = UUID()
        producer?.cancel(); timer?.cancel(); selection?.cancel(); hideTask?.cancel()
        selectionID = nil
    }
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
        selectionID = nil
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
            await persist(for: run)
            while !Task.isCancelled && generation == run {
                if timeline.ahead < 20 {
                    let excluded = [timeline.current].compactMap { $0 } + timeline.ids.filter { $0 != timeline.current }
                    do {
                        let candidates = try await store.batch(server: url, exclude: excluded)
                        lastNetworkError = "无"
                        for candidate in candidates {
                            try Task.checkCancellation()
                            guard generation == run else { return }
                            if timeline.ids.contains(candidate.id) || timeline.ahead >= 20 { continue }
                            do {
                                try await store.prepare(candidate.id, server: url)
                                let thumb = await store.image(candidate.id, edge: 240)
                                guard !Task.isCancelled, generation == run else {
                                    await store.discardPrepared(candidate.id); return
                                }
                                guard let thumb else { await store.discardPrepared(candidate.id); continue }
                                timeline.append(candidate.id).forEach { thumbnails.removeValue(forKey: $0) }
                                thumbnails[candidate.id] = NSImage(cgImage: thumb, size: .zero)
                                if picture == nil || pictureID != timeline.current && timeline.ids.count == 1 {
                                    if let cg = await store.image(candidate.id, edge: 2560), generation == run {
                                        picture = NSImage(cgImage: cg, size: .zero); pictureID = candidate.id; lastChange = Date()
                                    }
                                }
                                await persist(for: run)
                            } catch {
                                if Task.isCancelled { return }
                                log.error("Photo preparation failed: \(String(describing: type(of: error)), privacy: .public) code \((error as NSError).code)")
                            }
                        }
                    } catch {
                        if Task.isCancelled { return }
                        lastNetworkError = "网络请求错误 \((error as NSError).code)"
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
                if playing && !controls && !settings && selectionID == nil && Date().timeIntervalSince(lastChange) >= 10 {
                    if timeline.ahead > 0 { move(1) }
                }
            }
        }
    }
    func select(_ index: Int) {
        guard timeline.ids.indices.contains(index), index != timeline.position else { return }
        let id = timeline.ids[index], run = generation
        selection?.cancel()
        let request = UUID(); selectionID = request
        selection = Task {
            defer { if selectionID == request { selectionID = nil } }
            let cg = await store.image(id, edge: 2560)
            guard !Task.isCancelled, generation == run else { return }
            guard let cg else {
                // A missing/corrupt cache entry must not become a permanent barrier.
                timeline.remove(id); thumbnails.removeValue(forKey: id); recoveredPhotos += 1
                await store.invalidate(id)
                await persist(for: run)
                log.notice("Removed unreadable cached photo; playback can continue")
                return
            }
            guard let actual = timeline.ids.firstIndex(of: id) else { return }
            timeline.select(actual)
            picture = NSImage(cgImage: cg, size: .zero); pictureID = id; lastChange = Date()
            await persist(for: run)
        }
    }
    private func persist(for run: UUID) async {
        guard generation == run, let server else { return }
        storeRevision += 1
        await store.save(timeline, server: server, revision: storeRevision)
    }
    func diagnosticSummary() async -> String {
        let missing = await store.missingCount(timeline.ids)
        return """
        播放：\(playing ? "自动播放" : "已暂停")
        控制栏：\(controls ? "显示中（暂缓自动切换）" : "已隐藏")
        缓存：\(timeline.ids.count) 张，后续 \(timeline.ahead) 张，缺失 \(missing) 张
        距上次切换：\(Int(max(0, Date().timeIntervalSince(lastChange)))) 秒
        正在切换：\(selectionID == nil ? "否" : "是")
        已跳过异常缓存：\(recoveredPhotos) 张
        最近网络错误：\(lastNetworkError)
        """
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
