package plugins

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/ed25519"
    "crypto/rand"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

type Installation struct {
    PluginID string `json:"pluginId"`
    Slug     string `json:"slug"`
    Version  string `json:"version"`
}

type SecretStore struct { path string; key []byte }

func NewSecretStore(path string, privateKey ed25519.PrivateKey) *SecretStore {
    sum := sha256.Sum256(privateKey)
    return &SecretStore{path: path, key: sum[:]}
}

func (s *SecretStore) Save(pluginID string, value map[string]any) error {
    block, err := aes.NewCipher(s.key); if err != nil { return err }
    gcm, err := cipher.NewGCM(block); if err != nil { return err }
    plaintext, err := json.Marshal(value); if err != nil { return err }
    nonce := make([]byte, gcm.NonceSize()); if _, err := rand.Read(nonce); err != nil { return err }
    ciphertext := gcm.Seal(nonce, nonce, plaintext, []byte(pluginID))
    if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil { return err }
    file := s.path + "." + pluginID
    tmp, err := os.CreateTemp(filepath.Dir(file), ".plugin-secret-*"); if err != nil { return err }
    name := tmp.Name(); defer os.Remove(name)
    if err := tmp.Chmod(0600); err != nil { _ = tmp.Close(); return err }
    if _, err := tmp.Write(ciphertext); err != nil { _ = tmp.Close(); return err }
    if err := tmp.Sync(); err != nil { _ = tmp.Close(); return err }
    if err := tmp.Close(); err != nil { return err }
    return os.Rename(name, file)
}

func (s *SecretStore) Load(pluginID string) (map[string]any, error) {
    ciphertext, err := os.ReadFile(s.path + "." + pluginID); if err != nil { return nil, err }
    block, err := aes.NewCipher(s.key); if err != nil { return nil, err }
    gcm, err := cipher.NewGCM(block); if err != nil { return nil, err }
    if len(ciphertext) < gcm.NonceSize() { return nil, fmt.Errorf("invalid encrypted plugin credential") }
    nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(pluginID)); if err != nil { return nil, fmt.Errorf("decrypt plugin credential: %w", err) }
    var value map[string]any
    if err := json.Unmarshal(plaintext, &value); err != nil { return nil, err }
    return value, nil
}

func (s *SecretStore) Delete(pluginID string) error {
    err := os.Remove(s.path + "." + pluginID)
    if os.IsNotExist(err) { return nil }
    return err
}

func (s *SecretStore) manifestPath() string { return s.path + ".manifest.json" }

func (s *SecretStore) ListInstallations() ([]Installation, error) {
    data, err := os.ReadFile(s.manifestPath())
    if os.IsNotExist(err) { return nil, nil }
    if err != nil { return nil, err }
    var installations []Installation
    if err := json.Unmarshal(data, &installations); err != nil { return nil, fmt.Errorf("decode plugin installation manifest: %w", err) }
    return installations, nil
}

func (s *SecretStore) SaveInstallation(installation Installation) error {
    installations, err := s.ListInstallations()
    if err != nil { return err }
    replaced := false
    for i := range installations {
        if installations[i].PluginID != installation.PluginID { continue }
        installations[i] = installation
        replaced = true
        break
    }
    if !replaced { installations = append(installations, installation) }
    return s.writeManifest(installations)
}

func (s *SecretStore) DeleteInstallation(pluginID string) error {
    installations, err := s.ListInstallations()
    if err != nil { return err }
    filtered := installations[:0]
    for _, installation := range installations {
        if installation.PluginID != pluginID { filtered = append(filtered, installation) }
    }
    if len(filtered) == len(installations) { return nil }
    return s.writeManifest(filtered)
}

func (s *SecretStore) writeManifest(installations []Installation) error {
    if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil { return err }
    data, err := json.MarshalIndent(installations, "", "  ")
    if err != nil { return err }
    file := s.manifestPath()
    tmp, err := os.CreateTemp(filepath.Dir(file), ".plugin-manifest-*")
    if err != nil { return err }
    name := tmp.Name(); defer os.Remove(name)
    if err := tmp.Chmod(0600); err != nil { _ = tmp.Close(); return err }
    if _, err := tmp.Write(data); err != nil { _ = tmp.Close(); return err }
    if err := tmp.Sync(); err != nil { _ = tmp.Close(); return err }
    if err := tmp.Close(); err != nil { return err }
    return os.Rename(name, file)
}
