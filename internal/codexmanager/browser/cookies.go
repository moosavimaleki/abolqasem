package browser

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	ss "github.com/zalando/go-keyring/secret_service"
	"golang.org/x/crypto/pbkdf2"
	_ "modernc.org/sqlite"
)

var ErrEncryptedCookie = errors.New("Chrome cookie encryption could not be unlocked")

type Decryptor func([]byte) (string, error)

// CopyCookieDB creates an isolated read-only snapshot for a platform cookie
// decoder. It copies the SQLite sidecars too, which is required while Chrome is
// running. The returned directory must be removed by the caller.
func CopyCookieDB(ctx context.Context, profile Profile) (string, func(), error) {
	if err := ValidateProfile(profile); err != nil {
		return "", nil, err
	}
	temporary, err := os.MkdirTemp("", "abolqasem-chrome-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	if err := os.Chmod(temporary, 0700); err != nil {
		cleanup()
		return "", nil, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := ctx.Err(); err != nil {
			cleanup()
			return "", nil, err
		}
		source := profile.CookieDB + suffix
		destination := filepath.Join(temporary, "Cookies"+suffix)
		if err := copyFile(source, destination); err != nil && !os.IsNotExist(err) {
			cleanup()
			return "", nil, err
		}
	}
	return filepath.Join(temporary, "Cookies"), cleanup, nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func RedactedCookieSummary(cookies []Cookie) string {
	return fmt.Sprintf("%d ChatGPT cookies loaded", len(cookies))
}

// LoadChatGPTCookies opens only a private copy of Chrome's SQLite cookie DB.
// Values are retained solely in memory for the caller's authenticated request
// and are excluded from JSON by Cookie.Value's '-' tag.
func LoadChatGPTCookies(ctx context.Context, profile Profile, decrypt Decryptor) ([]Cookie, error) {
	copyPath, cleanup, err := CopyCookieDB(ctx, profile)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(copyPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer database.Close()
	metaVersion := chromeCookieDBVersion(ctx, database)
	rows, err := database.QueryContext(ctx, `SELECT host_key, name, value, encrypted_value, path, is_secure, expires_utc FROM cookies WHERE host_key LIKE '%chatgpt.com'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now()
	result := make([]Cookie, 0)
	for rows.Next() {
		var host, name, plain, path string
		var encrypted []byte
		var secure int
		var expires int64
		if err := rows.Scan(&host, &name, &plain, &encrypted, &path, &secure, &expires); err != nil {
			return nil, err
		}
		if expires > 0 && chromeTime(expires).Before(now) {
			continue
		}
		value := plain
		if value == "" && len(encrypted) > 0 {
			if decrypt == nil {
				decrypt = DefaultDecryptorForProfile(profile)
			}
			value, err = decrypt(encrypted)
			if err != nil {
				return nil, ErrEncryptedCookie
			}
			// Chromium schema v24+ prefixes encrypted cookie plaintext with a
			// 32-byte host hash. It is an integrity check, not part of the
			// cookie value; sending it as a header corrupts authentication.
			if metaVersion >= 24 && len(value) >= 32 {
				value = value[32:]
			}
		}
		if value != "" {
			result = append(result, Cookie{Name: name, Value: value, Domain: strings.TrimPrefix(strings.ToLower(host), "."), Path: path, Secure: secure != 0})
		}
	}
	return result, rows.Err()
}

func chromeCookieDBVersion(ctx context.Context, database *sql.DB) int {
	var version int
	if err := database.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'version'`).Scan(&version); err != nil {
		return 0
	}
	return version
}

func DefaultDecryptor() Decryptor {
	return DefaultDecryptorForProfile(Profile{})
}

// DefaultDecryptorForProfile selects Chrome's regular or Chromium's keyring
// entry. The password is read only inside the current process, used to decrypt
// the private cookie-db copy, and is never written or serialized.
func DefaultDecryptorForProfile(profile Profile) Decryptor {
	if runtime.GOOS == "linux" {
		label := "Chrome Safe Storage"
		root := strings.ToLower(profile.Root + "/" + profile.Path)
		if strings.Contains(root, "chromium") {
			label = "Chromium Safe Storage"
		}
		return decryptLinuxChrome(label)
	}
	return func([]byte) (string, error) { return "", ErrEncryptedCookie }
}

func decryptLinuxChrome(keyringLabel string) Decryptor {
	var once sync.Once
	var keyringPassword []byte
	var keyringErr error
	return func(value []byte) (string, error) {
		if len(value) < 3 {
			return "", ErrEncryptedCookie
		}
		switch string(value[:3]) {
		case "v10":
			return decryptLinuxWithPasswords(value, []byte("peanuts"), nil)
		case "v11":
			once.Do(func() {
				keyringPassword, keyringErr = readLinuxChromeKeyringPassword(keyringLabel)
			})
			if keyringErr != nil || len(keyringPassword) == 0 {
				return "", ErrEncryptedCookie
			}
			return decryptLinuxWithPasswords(value, keyringPassword, nil)
		default:
			return "", ErrEncryptedCookie
		}
	}
}

func decryptLinuxLegacy(value []byte) (string, error) {
	return decryptLinuxWithPasswords(value, []byte("peanuts"), nil)
}

func decryptLinuxWithPasswords(value []byte, passwords ...[]byte) (string, error) {
	if len(value) < 3 || (string(value[:3]) != "v10" && string(value[:3]) != "v11") {
		return "", ErrEncryptedCookie
	}
	for _, password := range passwords {
		key := pbkdf2.Key(password, []byte("saltysalt"), 1, 16, sha1.New)
		block, err := aes.NewCipher(key)
		if err != nil || (len(value)-3)%block.BlockSize() != 0 {
			continue
		}
		plain := make([]byte, len(value)-3)
		cipher.NewCBCDecrypter(block, []byte("                ")).CryptBlocks(plain, value[3:])
		if len(plain) == 0 {
			continue
		}
		padding := int(plain[len(plain)-1])
		if padding < 1 || padding > block.BlockSize() || padding > len(plain) {
			continue
		}
		return string(plain[:len(plain)-padding]), nil
	}
	return "", ErrEncryptedCookie
}

func readLinuxChromeKeyringPassword(label string) ([]byte, error) {
	service, err := ss.NewSecretService()
	if err != nil {
		return nil, err
	}
	defer service.Conn.Close()
	collection := service.GetLoginCollection()
	if err := service.Unlock(collection.Path()); err != nil {
		return nil, err
	}
	property, err := collection.GetProperty("org.freedesktop.Secret.Collection.Items")
	if err != nil {
		return nil, err
	}
	items, ok := property.Value().([]dbus.ObjectPath)
	if !ok {
		return nil, ErrEncryptedCookie
	}
	session, err := service.OpenSession()
	if err != nil {
		return nil, err
	}
	defer service.Close(session)
	for _, path := range items {
		item := service.Object("org.freedesktop.secrets", path)
		itemLabel, err := item.GetProperty("org.freedesktop.Secret.Item.Label")
		if err != nil || itemLabel.Value() != label {
			continue
		}
		secret, err := service.GetSecret(path, session.Path())
		if err != nil || len(secret.Value) == 0 {
			return nil, ErrEncryptedCookie
		}
		return append([]byte(nil), secret.Value...), nil
	}
	return nil, ErrEncryptedCookie
}

func chromeTime(value int64) time.Time {
	// Chrome stores microseconds since 1601-01-01 UTC.
	const windowsToUnixMicroseconds = 11_644_473_600_000_000
	return time.UnixMicro(value - windowsToUnixMicroseconds)
}
