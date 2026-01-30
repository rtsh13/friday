package context

import (
	"sync"

	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
)

type Manager struct {
	messages    []types.Message
	maxMessages int
	mu          sync.RWMutex
}

func NewManager(maxMessages int) *Manager {
	return &Manager{
		messages:    make([]types.Message, 0),
		maxMessages: maxMessages,
	}
}

func (m *Manager) AddMessage(msg types.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.messages = append(m.messages, msg)
	
	if len(m.messages) > m.maxMessages {
		m.messages = m.messages[len(m.messages)-m.maxMessages:]
	}
}

func (m *Manager) GetMessages() []types.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]types.Message, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.messages = make([]types.Message, 0)
}