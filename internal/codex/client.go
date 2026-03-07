package codex

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
	"github.com/gorilla/websocket"
)

type Client struct {
	cfg    config.Config
	paths  config.Paths
	logger *slog.Logger
	tools  *ToolRegistry

	cmd    *exec.Cmd
	conn   *websocket.Conn
	wsURL  string
	closed chan struct{}

	writeMu sync.Mutex
	stateMu sync.Mutex
	nextID  int64
	pending map[string]chan rpcResponse
	turns   map[string]*turnWaiter
}

type ThreadSession struct {
	ID string
}

type rpcResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type turnWaiter struct {
	threadID   string
	turnID     string
	deltas     strings.Builder
	texts      []string
	completed  chan turnResult
	receivedAt time.Time
}

type turnResult struct {
	Text  string
	Error error
}

type TurnOptions struct {
	Effort string
}

var ErrTurnInterrupted = errors.New("turn interrupted")

func NewClient(cfg config.Config, paths config.Paths, logger *slog.Logger, tools *ToolRegistry) *Client {
	if tools == nil {
		tools = NewToolRegistry()
	}
	return &Client{
		cfg:     cfg,
		paths:   paths,
		logger:  logger,
		tools:   tools,
		closed:  make(chan struct{}),
		pending: map[string]chan rpcResponse{},
		turns:   map[string]*turnWaiter{},
	}
}
