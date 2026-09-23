package client

import (
	"time"

	"github.com/MawCeron/lazyftp/internal/model"
)

// dialTimeout bounds how long establishing a connection may take. Left unset,
// a dial waits out the operating system's TCP timeout instead.
const dialTimeout = 10 * time.Second

// SFTPAuthMethod selects how an SFTP connection authenticates.
type SFTPAuthMethod int

const (
	SFTPAuthPassword SFTPAuthMethod = iota
	SFTPAuthIdentityFile
)

// ConnectionOptions contains the fields needed to connect using a protocol.
// Password is used by FTP/FTPS and SFTP password mode; the identity fields are
// used only by SFTP identity-file mode.
type ConnectionOptions struct {
	Host          string
	User          string
	Password      string
	Port          int
	SFTPAuth      SFTPAuthMethod
	IdentityFile  string
	KeyPassphrase string
}

type Client interface {
	Connect(options ConnectionOptions) error
	Disconnect() error
	// InitialDir is the server-reported working directory established by login.
	InitialDir() (string, error)
	List(path string) ([]model.FileInfo, error)
	Upload(localPath, remotePath string, progress func(int64)) error
	Download(remotePath, localPath string, progress func(int64)) error
	Mkdir(path string) error
	Rename(oldPath, newPath string) error
	Delete(path string, isDir bool) error
}
