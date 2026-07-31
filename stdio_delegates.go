package wuzapi

import (
	"io"
	"wuzapi/internal/infrastructure/stdio"
)

// stdioServer re-exported from internal/infrastructure/stdio.
type stdioServer = stdio.Server

// NewStdioServer wraps stdio.NewServer.
func NewStdioServer(s *server) *stdioServer {
	return stdio.NewServer(s.router)
}

// SendNotification delegates to stdio.SendNotification.
func (s *server) SendNotification(method string, params map[string]interface{}) {
	stdio.SendNotification(method, params)
}
func newStdioServerWithIO(s *server, stdin io.Reader, stdout io.Writer) *stdioServer {
	return stdio.NewServerWithIO(s.router, stdin, stdout)
}

