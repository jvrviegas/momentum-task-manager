import Foundation
import Security

/// This Mac's sign-in to one Momentum server. The token signs in as its
/// owner, so it is kept only in the Keychain and the bridge's memory.
struct SavedSignIn: Codable, Sendable, Equatable {
    var server: String
    var token: String
    var email: String
    var expiresAt: Date
}

/// Where the app keeps sign-ins, one per server. KeychainSignInStore keeps
/// them in the Keychain; tests use a fake.
protocol SignInStore: Sendable {
    func read(server: String) throws -> SavedSignIn?
    func save(_ signIn: SavedSignIn) throws
    func remove(server: String) throws
}

/// Keeps each sign-in as a generic password in the login Keychain, under the
/// server's address.
struct KeychainSignInStore: SignInStore {
    var service = "dev.jvrviegas.momentum.sign-in"

    func read(server: String) throws -> SavedSignIn? {
        var query = item(server)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var data: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &data)
        if status == errSecItemNotFound {
            return nil
        }
        guard status == errSecSuccess, let data = data as? Data else { throw KeychainError(status: status) }
        return try JSONDecoder().decode(SavedSignIn.self, from: data)
    }

    func save(_ signIn: SavedSignIn) throws {
        let data = try JSONEncoder().encode(signIn)
        var status = SecItemUpdate(item(signIn.server) as CFDictionary, [kSecValueData as String: data] as CFDictionary)
        if status == errSecItemNotFound {
            var attributes = item(signIn.server)
            attributes[kSecValueData as String] = data
            attributes[kSecAttrLabel as String] = "Momentum sign-in"
            status = SecItemAdd(attributes as CFDictionary, nil)
        }
        guard status == errSecSuccess else { throw KeychainError(status: status) }
    }

    func remove(server: String) throws {
        let status = SecItemDelete(item(server) as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw KeychainError(status: status) }
    }

    private func item(_ server: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: server]
    }
}

struct KeychainError: Error, LocalizedError {
    var status: OSStatus

    var errorDescription: String? {
        SecCopyErrorMessageString(status, nil) as String? ?? "Keychain error \(status)"
    }
}
