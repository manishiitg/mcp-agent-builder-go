package videoproduct

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var secretNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

func normalizeSecretName(name string) (string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if !secretNamePattern.MatchString(name) {
		return "", fmt.Errorf("secret name must use 2-64 uppercase letters, numbers, or underscores")
	}
	return name, nil
}

func (s *Store) PutSecret(userID, name, value string) error {
	name, err := normalizeSecretName(name)
	if err != nil {
		return err
	}
	if value == "" {
		return errors.New("secret value is required")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := s.aead.Seal(nil, nonce, []byte(value), []byte(userID+":"+name))
	now := dbTime(time.Now())
	_, err = s.db.Exec(`INSERT INTO secrets(user_id,name,nonce,ciphertext,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(user_id,name) DO UPDATE SET nonce=excluded.nonce,ciphertext=excluded.ciphertext,updated_at=excluded.updated_at`, userID, name, nonce, ciphertext, now, now)
	return err
}

func (s *Store) SecretNames(userID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM secrets WHERE user_id=? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (s *Store) DeleteSecret(userID, name string) error {
	name, err := normalizeSecretName(name)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM secrets WHERE user_id=? AND name=?`, userID, name)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SecretEnv(userID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT name,nonce,ciphertext FROM secrets WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var env []string
	for rows.Next() {
		var name string
		var nonce, ciphertext []byte
		if err := rows.Scan(&name, &nonce, &ciphertext); err != nil {
			return nil, err
		}
		value, err := s.aead.Open(nil, nonce, ciphertext, []byte(userID+":"+name))
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %s: %w", name, err)
		}
		env = append(env, name+"="+string(value))
	}
	sort.Strings(env)
	return env, rows.Err()
}
