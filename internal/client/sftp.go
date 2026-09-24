package client

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/carabila/lazyftp/internal/model"
	"github.com/carabila/lazyftp/internal/shared"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPClient struct {
	sshConn       *ssh.Client
	client        *sftp.Client
	initialDir    string
	initialDirErr error
}

func NewSFTPClient() *SFTPClient {
	return &SFTPClient{}
}

func sftpAuth(options ConnectionOptions) (ssh.AuthMethod, error) {
	switch options.SFTPAuth {
	case SFTPAuthPassword:
		return ssh.Password(options.Password), nil
	case SFTPAuthIdentityFile:
		identityPath, err := resolveIdentityPath(options.IdentityFile)
		if err != nil {
			return nil, err
		}

		key, err := os.ReadFile(identityPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read identity file %q: %w", identityPath, err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		var passphraseMissing *ssh.PassphraseMissingError
		if errors.As(err, &passphraseMissing) {
			if options.KeyPassphrase == "" {
				return nil, fmt.Errorf("identity file %q requires a passphrase", identityPath)
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(options.KeyPassphrase))
		}
		if err != nil {
			return nil, fmt.Errorf("unable to parse identity file %q: %w", identityPath, err)
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("unsupported SFTP authentication method %d", options.SFTPAuth)
	}
}

func resolveIdentityPath(identityPath string) (string, error) {
	if identityPath == "" {
		return "", fmt.Errorf("identity file path is empty")
	}

	if identityPath == "~" || strings.HasPrefix(identityPath, "~/") || strings.HasPrefix(identityPath, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to resolve home directory for identity file: %w", err)
		}
		if identityPath == "~" {
			identityPath = home
		} else {
			identityPath = filepath.Join(home, identityPath[2:])
		}
	}

	return filepath.Clean(identityPath), nil
}

func (c *SFTPClient) Connect(options ConnectionOptions) error {
	auth, err := sftpAuth(options)
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: options.User,
		Auth: []ssh.AuthMethod{auth},
		// TODO: verificar host key en versiones futuras
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}

	addr := net.JoinHostPort(options.Host, strconv.Itoa(options.Port))

	// ssh.Dial bounds the TCP dial only. A host that accepts without speaking
	// SSH leaves the handshake waiting with nothing to end it.
	tcpConn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("unable to connect to %s: %w", addr, err)
	}
	tcpConn.SetDeadline(time.Now().Add(dialTimeout))

	conn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, config)
	if err != nil {
		tcpConn.Close()
		return fmt.Errorf("unable to connect to %s: %w", addr, err)
	}

	sshConn := ssh.NewClient(conn, chans, reqs)

	// The deadline set above still applies here: a server that accepts the
	// SSH handshake but never answers the SFTP subsystem request would
	// otherwise hang Connect forever.
	client, err := sftp.NewClient(sshConn)
	if err != nil {
		sshConn.Close()
		return fmt.Errorf("error starting SFTP session: %w", err)
	}

	initialDir, initialDirErr := client.Getwd()
	if initialDirErr == nil && initialDir == "" {
		initialDirErr = fmt.Errorf("server returned an empty working directory")
	}
	if initialDirErr != nil {
		initialDir = "/"
		initialDirErr = fmt.Errorf("unable to determine remote starting directory on %s: %w", addr, initialDirErr)
	}

	// Left in place the deadline would expire mid-transfer.
	tcpConn.SetDeadline(time.Time{})

	c.sshConn = sshConn
	c.client = client
	c.initialDir = initialDir
	c.initialDirErr = initialDirErr
	return nil
}

func (c *SFTPClient) InitialDir() (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("no active connection")
	}
	return c.initialDir, c.initialDirErr
}

func (c *SFTPClient) Disconnect() error {
	if c.client != nil {
		c.client.Close()
	}
	if c.sshConn != nil {
		c.sshConn.Close()
	}
	return nil
}

func (c *SFTPClient) List(path string) ([]model.FileInfo, error) {
	if c.client == nil {
		return nil, fmt.Errorf("no active connection")
	}

	entries, err := c.client.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("error listing %s: %w", path, err)
	}

	var files []model.FileInfo
	for _, e := range entries {
		if e.Name() == "." || e.Name() == ".." {
			continue
		}

		fileType := model.FileTypeFile
		if e.IsDir() {
			fileType = model.FileTypeDir
		} else if e.Mode()&os.ModeSymlink != 0 {
			fileType = model.FileTypeSymlink
		}

		files = append(files, model.FileInfo{
			Name:     e.Name(),
			Size:     e.Size(),
			ModTime:  e.ModTime(),
			Type:     fileType,
			IsHidden: len(e.Name()) > 0 && e.Name()[0] == '.',
		})
	}

	return files, nil
}

func (c *SFTPClient) Upload(localPath, remotePath string, progress func(int64)) error {
	if c.client == nil {
		return fmt.Errorf("no active connection")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("error opening local file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("error reading local file: %w", err)
	}

	remotePath = path.Join(remotePath, filepath.Base(localPath))
	dst, err := c.client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("error creating remote file: %w", err)
	}
	defer dst.Close()

	reader := &shared.ProgressReader{
		Reader:   f,
		Total:    info.Size(),
		Callback: progress,
	}

	if _, err := io.Copy(dst, reader); err != nil {
		return fmt.Errorf("error uploading file: %w", err)
	}

	return nil
}

func (c *SFTPClient) Download(remotePath, localPath string, progress func(int64)) error {
	if c.client == nil {
		return fmt.Errorf("no active connection")
	}

	src, err := c.client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("error opening remote file: %w", err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("error reading remote file: %w", err)
	}

	destPath := filepath.Join(localPath, filepath.Base(remotePath))
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("error creating local file: %w", err)
	}
	defer f.Close()

	writer := &shared.ProgressWriter{
		Writer:   f,
		Total:    info.Size(),
		Callback: progress,
	}

	if _, err := io.Copy(writer, src); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	return nil
}

func (c *SFTPClient) Mkdir(path string) error {
	if c.client == nil {
		return fmt.Errorf("no active connection")
	}
	return c.client.MkdirAll(path)
}

func (c *SFTPClient) Rename(oldPath, newPath string) error {
	if c.client == nil {
		return fmt.Errorf("no active connection")
	}
	return c.client.Rename(oldPath, newPath)
}

func (c *SFTPClient) Delete(path string, isDir bool) error {
	if c.client == nil {
		return fmt.Errorf("no active connection")
	}
	if isDir {
		return c.client.RemoveAll(path)
	}
	return c.client.Remove(path)
}
