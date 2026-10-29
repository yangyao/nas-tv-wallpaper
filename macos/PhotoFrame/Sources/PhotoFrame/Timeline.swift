import Foundation

struct Timeline: Codable {
    var ids: [String] = []
    var position = 0
    var current: String? { ids.indices.contains(position) ? ids[position] : nil }
    var ahead: Int { max(0, ids.count - position - 1) }

    mutating func select(_ index: Int) {
        guard ids.indices.contains(index) else { return }
        position = index
    }
    mutating func remove(_ id: String) {
        guard let index = ids.firstIndex(of: id) else { return }
        ids.remove(at: index)
        if index < position { position -= 1 }
        position = min(position, max(0, ids.count - 1))
    }

    mutating func append(_ id: String) -> [String] {
        guard !ids.contains(id) else { return [] }
        ids.append(id)
        let removeCount = min(position, max(max(0, position - 40), max(0, ids.count - 60)))
        let removed = Array(ids.prefix(removeCount))
        ids.removeFirst(removeCount)
        position -= removeCount
        return removed
    }
}

enum ServerAddress {
    static func parse(_ input: String) -> URL? {
        guard var c = URLComponents(string: input.trimmingCharacters(in: .whitespacesAndNewlines)),
              ["http", "https"].contains(c.scheme?.lowercased() ?? ""),
              c.host != nil, c.user == nil, c.password == nil,
              c.query == nil, c.fragment == nil else { return nil }
        c.path = ""
        return c.url
    }
}
