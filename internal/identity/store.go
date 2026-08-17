package identity

import (
    "crypto/ed25519"
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
)

type Credentials struct {
    AgentID    string `json:"agent_id,omitempty"`
    KeyID      string `json:"key_id,omitempty"`
    DeviceID   string `json:"device_id"`
    PrivateKey string `json:"private_key"`
    PublicKey  string `json:"public_key"`
}

type Store struct { Path string }

func (s Store) LoadOrCreate() (Credentials, ed25519.PrivateKey, ed25519.PublicKey, error) {
    if data, err := os.ReadFile(s.Path); err == nil {
        var c Credentials
        if err := json.Unmarshal(data, &c); err != nil { return Credentials{}, nil, nil, fmt.Errorf("invalid credentials file: %w", err) }
        privateKey, publicKey, err := decodeKeys(c)
        if err != nil { return Credentials{}, nil, nil, err }
        if c.DeviceID == "" { return Credentials{}, nil, nil, errors.New("credentials are missing device_id") }
        return c, privateKey, publicKey, nil
    } else if !errors.Is(err, os.ErrNotExist) {
        return Credentials{}, nil, nil, fmt.Errorf("read credentials: %w", err)
    }

    publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil { return Credentials{}, nil, nil, fmt.Errorf("generate Ed25519 identity: %w", err) }
    deviceID, err := newDeviceID()
    if err != nil { return Credentials{}, nil, nil, err }
    c := Credentials{
        DeviceID: deviceID,
        PrivateKey: base64.RawURLEncoding.EncodeToString(privateKey),
        PublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
    }
    if err := s.Save(c); err != nil { return Credentials{}, nil, nil, err }
    return c, privateKey, publicKey, nil
}

func (s Store) Save(c Credentials) error {
    if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil { return fmt.Errorf("create credentials directory: %w", err) }
    data, err := json.MarshalIndent(c, "", "  ")
    if err != nil { return err }
    tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".credentials-*")
    if err != nil { return err }
    name := tmp.Name()
    defer os.Remove(name)
    if err := tmp.Chmod(0600); err != nil { _ = tmp.Close(); return err }
    if _, err := tmp.Write(data); err != nil { _ = tmp.Close(); return err }
    if err := tmp.Sync(); err != nil { _ = tmp.Close(); return err }
    if err := tmp.Close(); err != nil { return err }
    return os.Rename(name, s.Path)
}

func decodeKeys(c Credentials) (ed25519.PrivateKey, ed25519.PublicKey, error) {
    privateBytes, err := base64.RawURLEncoding.DecodeString(c.PrivateKey)
    if err != nil || len(privateBytes) != ed25519.PrivateKeySize { return nil, nil, errors.New("invalid Ed25519 private key") }
    publicBytes, err := base64.RawURLEncoding.DecodeString(c.PublicKey)
    if err != nil || len(publicBytes) != ed25519.PublicKeySize { return nil, nil, errors.New("invalid Ed25519 public key") }
    return ed25519.PrivateKey(privateBytes), ed25519.PublicKey(publicBytes), nil
}

func newDeviceID() (string, error) {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil { return "", fmt.Errorf("generate device id: %w", err) }
    b[6] = (b[6] & 0x0f) | 0x40
    b[8] = (b[8] & 0x3f) | 0x80
    return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
