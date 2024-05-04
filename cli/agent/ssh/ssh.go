package ssh

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/quexten/goldwarden/cli/agent/config"
	"github.com/quexten/goldwarden/cli/agent/notify"
	"github.com/quexten/goldwarden/cli/agent/sockets"
	"github.com/quexten/goldwarden/cli/agent/systemauth"
	"github.com/quexten/goldwarden/cli/agent/systemauth/pinentry"
	"github.com/quexten/goldwarden/cli/agent/vault"
	"github.com/quexten/goldwarden/cli/logging"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var log = logging.GetLogger("Goldwarden", "SSH")

type vaultAgent struct {
	vault               *vault.Vault
	config              *config.Config
	unlockRequestAction func() bool
	context             sockets.CallingContext
}

func (vaultAgent) Add(key agent.AddedKey) error {
	log.Warn("Add Request - Not implemented")
	return nil
}

func (vaultAgent vaultAgent) List() ([]*agent.Key, error) {
	log.Info("List Request")
	if vaultAgent.vault.Keyring.IsLocked() {
		if !vaultAgent.unlockRequestAction() {
			log.Warn("List request failed - Vault is locked")
			return nil, errors.New("vault is locked")
		}

		systemauth.CreatePinSession(vaultAgent.context, systemauth.SSHTTL)
	}

	vaultSSHKeys := (*vaultAgent.vault).GetSSHKeys()
	var sshKeys []*agent.Key
	for _, vaultSSHKey := range vaultSSHKeys {
		signer, err := ssh.ParsePrivateKey([]byte(vaultSSHKey.Key))
		if err != nil {
			log.Warn("List request key skipped - Could not parse key: %s", err)
			continue
		}
		pub := signer.PublicKey()
		sshKeys = append(sshKeys, &agent.Key{
			Format:  pub.Type(),
			Blob:    pub.Marshal(),
			Comment: vaultSSHKey.Name})
	}

	return sshKeys, nil
}

func (vaultAgent) Lock(passphrase []byte) error {
	log.Warn("Lock Request - Not implemented")
	return nil
}

func (vaultAgent) Remove(key ssh.PublicKey) error {
	log.Warn("Remove Request - Not implemented")
	return nil
}

func (vaultAgent) RemoveAll() error {
	log.Warn("RemoveAll Request - Not implemented")
	return nil
}

func Eq(a, b ssh.PublicKey) bool {
	return 0 == bytes.Compare(a.Marshal(), b.Marshal())
}

func (vaultAgent vaultAgent) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return vaultAgent.SignWithFlags(key, data, agent.SignatureFlagReserved)
}

func (vaultAgent vaultAgent) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	log.Info("Sign Request for key: %s", ssh.FingerprintSHA256(key))
	if vaultAgent.vault.Keyring.IsLocked() {
		if !vaultAgent.unlockRequestAction() {
			return nil, errors.New("vault is locked")
		}

		systemauth.CreatePinSession(vaultAgent.context, systemauth.SSHTTL)
	}

	var signer ssh.Signer
	var sshKey *vault.SSHKey

	vaultSSHKeys := (*vaultAgent.vault).GetSSHKeys()
	for _, vaultSSHKey := range vaultSSHKeys {
		sg, err := ssh.ParsePrivateKey([]byte(vaultSSHKey.Key))
		if err != nil {
			return nil, err
		}
		if Eq(sg.PublicKey(), key) {
			signer = sg
			sshKey = &vaultSSHKey
			break
		}
	}

	if sshKey == nil {
		return nil, errors.New("key not found")
	}

	isGit := false
	magicHeader := []byte("SSHSIG\x00\x00\x00\x03git")
	if bytes.HasPrefix(data, magicHeader) {
		isGit = true
	}

	requestTemplate := ""
	message := ""
	if !vaultAgent.context.Error {
		if isGit {
			requestTemplate = "%s on %s>%s>%s is requesting git signage with key %s"
		} else {
			requestTemplate = "%s on %s>%s>%s is requesting ssh signage with key %s"
		}
		message = fmt.Sprintf(requestTemplate, vaultAgent.context.UserName, vaultAgent.context.GrandParentProcessName, vaultAgent.context.ParentProcessName, vaultAgent.context.ProcessName, sshKey.Name)
	} else {
		if isGit {
			requestTemplate = "%s is requesting git signage with key %s"
		} else {
			requestTemplate = "%s is requesting ssh signage with key %s"
		}
		message = fmt.Sprintf(requestTemplate, vaultAgent.context.UserName, sshKey.Name)
	}

	// todo refactor
	if !systemauth.GetSSHSession(vaultAgent.context) {
		if approved, err := pinentry.GetApproval("SSH Key Signing Request", message); err != nil || !approved {
			log.Info("Sign Request for key: %s denied", sshKey.Name)
			return nil, errors.New("Approval not given")
		}

		if !systemauth.VerifyPinSession(vaultAgent.context) {
			if permission, err := systemauth.GetPermission(systemauth.SSHKey, vaultAgent.context, vaultAgent.config); err != nil || !permission {
				log.Info("Sign Request for key: %s denied", key.Marshal())
				return nil, errors.New("Biometrics not checked")
			}
		}

		systemauth.CreateSSHSession(vaultAgent.context)
	} else {
		log.Info("Using cached session approval")
	}

	var rand = rand.Reader
	log.Info("Sign Request for key: %s %s accepted", ssh.FingerprintSHA256(key), sshKey.Name)
	if isGit {
		notify.Notify("Goldwarden", fmt.Sprintf("Git Signing Request Approved for %s", sshKey.Name), "", 10*time.Second, func() {})
	} else {
		notify.Notify("Goldwarden", fmt.Sprintf("SSH Signing Request Approved for %s", sshKey.Name), "", 10*time.Second, func() {})
	}

	algo := ""

	switch flags {
	case agent.SignatureFlagRsaSha256:
		algo = ssh.KeyAlgoRSASHA256
	case agent.SignatureFlagRsaSha512:
		algo = ssh.KeyAlgoRSASHA512
	default:
		return signer.Sign(rand, data)
	}

	log.Info("%s algorithm requested", algo)

	algoSigner, err := ssh.NewSignerWithAlgorithms(signer.(ssh.AlgorithmSigner), []string{algo})
	if err != nil {
		return nil, err
	}

	return algoSigner.SignWithAlgorithm(rand, data, algo)
}

func (vaultAgent) Extension(extensionType string, contents []byte) ([]byte, error) {
	return nil, agent.ErrExtensionUnsupported
}

func (vaultAgent) Signers() ([]ssh.Signer, error) {
	log.Warn("Signers Request - Not implemented")
	return []ssh.Signer{}, nil
}

func (vaultAgent) Unlock(passphrase []byte) error {
	log.Warn("Unlock Request - Not implemented")
	return nil
}

type SSHAgentServer struct {
	vault               *vault.Vault
	config              *config.Config
	runtimeConfig       *config.RuntimeConfig
	unlockRequestAction func() bool
}

func (v *SSHAgentServer) SetUnlockRequestAction(action func() bool) {
	v.unlockRequestAction = action
}

func NewVaultAgent(vault *vault.Vault, config *config.Config, runtimeConfig *config.RuntimeConfig) SSHAgentServer {
	return SSHAgentServer{
		vault:         vault,
		config:        config,
		runtimeConfig: runtimeConfig,
		unlockRequestAction: func() bool {
			log.Info("Unlock Request, but no action defined")
			return false
		},
	}
}
