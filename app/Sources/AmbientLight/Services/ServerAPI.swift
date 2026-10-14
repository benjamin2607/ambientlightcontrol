import Foundation

/// HTTP client for the AmbientLight server on the NAS.
struct ServerAPI: Sendable {
    let baseURL: URL
    let token: String

    enum APIError: LocalizedError {
        case invalidURL
        case insecureURL
        case unauthorized
        case server(String)
        case unreachable(String)

        var errorDescription: String? {
            switch self {
            case .invalidURL: "Enter the server address, e.g. http://192.168.1.10:8787"
            case .insecureURL: "Plain HTTP is only allowed for addresses in your local network. Use HTTPS."
            case .unauthorized: "The server rejected the token."
            case .server(let message): message
            case .unreachable(let message): "Server not reachable: \(message)"
            }
        }
    }

    /// Validates user input. HTTP is accepted only for local addresses, so the token never travels
    /// unencrypted over the internet.
    static func validatedURL(_ input: String) throws -> URL {
        var text = input.trimmingCharacters(in: .whitespacesAndNewlines)
        while text.hasSuffix("/") { text.removeLast() }
        if !text.contains("://") { text = "http://" + text }
        guard let url = URL(string: text), let scheme = url.scheme?.lowercased(), let host = url.host, !host.isEmpty,
              ["http", "https"].contains(scheme), url.user == nil, url.query == nil, url.fragment == nil else {
            throw APIError.invalidURL
        }
        if scheme == "http", !isLocalHost(host) { throw APIError.insecureURL }
        return url
    }

    static func isLocalHost(_ host: String) -> Bool {
        let host = host.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        if host == "localhost" || host.hasSuffix(".local") || host.hasSuffix(".lan") || host.hasSuffix(".home.arpa") || !host.contains(".") && !host.contains(":") {
            return true
        }
        let parts = host.split(separator: ".").compactMap { Int($0) }
        if parts.count == 4, parts.allSatisfy({ (0...255).contains($0) }) {
            switch (parts[0], parts[1]) {
            case (10, _), (127, _), (192, 168), (169, 254): return true
            case (172, let b) where (16...31).contains(b): return true
            case (100, let b) where (64...127).contains(b): return true // CGNAT, e.g. Tailscale
            default: return false
            }
        }
        // IPv6 unique local (fc00::/7) and link-local (fe80::/10).
        return host.hasPrefix("fc") || host.hasPrefix("fd") || host.hasPrefix("fe80:") || host == "::1"
    }

    private static let session: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.httpCookieStorage = nil
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        config.timeoutIntervalForRequest = 15
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()

    func status() async throws -> ServerStatus {
        try await send("GET", "status")
    }

    func change(_ light: LightChange) async throws -> ServerStatus {
        try await send("POST", "light", body: JSONEncoder().encode(light))
    }

    func update(_ settings: SettingsChange) async throws -> ServerStatus {
        try await send("PATCH", "settings", body: JSONEncoder().encode(settings))
    }

    func resumeAutomation() async throws -> ServerStatus {
        try await send("POST", "automation/resume")
    }

    /// Waits until the prompt on the TV is accepted (up to a minute).
    func pairTV(host: String) async throws -> ServerStatus {
        struct Body: Encodable { let host: String }
        return try await send("POST", "tv/pair", body: JSONEncoder().encode(Body(host: host)), timeout: 75)
    }

    private func send(_ method: String, _ path: String, body: Data? = nil, timeout: TimeInterval = 15) async throws -> ServerStatus {
        var request = URLRequest(url: baseURL.appending(path: "api/v1/" + path), timeoutInterval: timeout)
        request.httpMethod = method
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = body
        }

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await Self.session.data(for: request)
        } catch {
            throw APIError.unreachable(error.localizedDescription)
        }
        let code = (response as? HTTPURLResponse)?.statusCode ?? 0
        switch code {
        case 200:
            let decoder = JSONDecoder()
            decoder.dateDecodingStrategy = .iso8601
            return try decoder.decode(ServerStatus.self, from: data)
        case 401:
            throw APIError.unauthorized
        default:
            let message = (try? JSONDecoder().decode([String: String].self, from: data))?["error"]
            throw APIError.server(message ?? "Server error (HTTP \(code)).")
        }
    }
}
