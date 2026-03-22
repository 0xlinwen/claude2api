package core

import "sync"

// ConversationState holds the state for a reusable conversation
type ConversationState struct {
	ConversationID    string
	ParentMessageUUID string
	SessionKey        string
	Model             string
}

// ConversationStore is a thread-safe in-memory store for conversation states
type ConversationStore struct {
	mu    sync.RWMutex
	store map[string]*ConversationState
}

// NewConversationStore creates a new ConversationStore
func NewConversationStore() *ConversationStore {
	return &ConversationStore{
		store: make(map[string]*ConversationState),
	}
}

// Get retrieves a conversation state by chatID
func (cs *ConversationStore) Get(chatID string) (*ConversationState, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	state, ok := cs.store[chatID]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid data races
	copied := *state
	return &copied, true
}

// Set stores or updates a conversation state by chatID
func (cs *ConversationStore) Set(chatID string, state *ConversationState) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.store[chatID] = state
}

// Delete removes a conversation state by chatID
func (cs *ConversationStore) Delete(chatID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.store, chatID)
}

// GlobalConversationStore is the singleton conversation store
var GlobalConversationStore = NewConversationStore()
