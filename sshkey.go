package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"

	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivateKeyPEM  []byte
	AuthorizedLine string
}

func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}

	privBlock, err := ssh.MarshalPrivateKey(priv, "pvcmount")
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		PrivateKeyPEM:  pem.EncodeToMemory(privBlock),
		AuthorizedLine: string(ssh.MarshalAuthorizedKey(sshPub)),
	}, nil
}

func WriteKeyFile(kp *KeyPair) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "pvcmount-key-*")
	if err != nil {
		return "", nil, err
	}
	if err := os.Chmod(f.Name(), 0600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	if _, err := f.Write(kp.PrivateKeyPEM); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}
