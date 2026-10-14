import Foundation
import Security

/// Server address (UserDefaults) and API token (Keychain). The token never goes to UserDefaults or logs.
struct CredentialStore: Sendable {
    private let service = "io.github.benjamin2607.ambientlight.server"
    private let tokenAccount = "api-token"
    private let urlKey = "serverURL"

    var serverURL: URL? { UserDefaults.standard.string(forKey: urlKey).flatMap(URL.init(string:)) }

    func token() -> String? {
        var query = baseQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    /// Returns false if the Keychain refused the item.
    @discardableResult
    func save(url: URL, token: String) -> Bool {
        SecItemDelete(baseQuery() as CFDictionary)
        var item = baseQuery()
        item[kSecValueData as String] = Data(token.utf8)
        // Never synced via iCloud and unavailable before the first unlock after boot.
        item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        item[kSecAttrLabel as String] = "AmbientLight – server token"
        guard SecItemAdd(item as CFDictionary, nil) == errSecSuccess else { return false }
        UserDefaults.standard.set(url.absoluteString, forKey: urlKey)
        return true
    }

    func clear() {
        SecItemDelete(baseQuery() as CFDictionary)
        UserDefaults.standard.removeObject(forKey: urlKey)
    }

    private func baseQuery() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: tokenAccount,
        ]
    }
}
