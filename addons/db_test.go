package addons

import (
	"path/filepath"
	"testing"
)

func TestOpenDBCreatesTables(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Verify tables exist by running queries that would fail without them
	sessions := database.ListSessions()
	if sessions == nil {
		// nil is valid for empty result, but should not panic
	}

	facts := database.QueryFacts("anything")
	if facts != nil {
		test.Errorf("expected no facts initially, got %d", len(facts))
	}

	setting := database.GetSetting("nonexistent")
	if setting != "" {
		test.Errorf("expected empty setting, got %q", setting)
	}
}

func TestSaveMessageAndLoadMessagesRoundTrip(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	sessionID := database.CreateSession("test-session")

	database.SaveMessage(sessionID, "user", "Hello")
	database.SaveMessage(sessionID, "assistant", "Hi there")
	database.SaveMessage(sessionID, "user", "How are you?")

	messages := database.LoadMessages(sessionID)
	if len(messages) != 3 {
		test.Fatalf("expected 3 messages, got %d", len(messages))
	}

	if messages[0].Role != "user" || messages[0].Content != "Hello" {
		test.Errorf("message 0: expected user/Hello, got %s/%s", messages[0].Role, messages[0].Content)
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Hi there" {
		test.Errorf("message 1: expected assistant/Hi there, got %s/%s", messages[1].Role, messages[1].Content)
	}
	if messages[2].Role != "user" || messages[2].Content != "How are you?" {
		test.Errorf("message 2: expected user/How are you?, got %s/%s", messages[2].Role, messages[2].Content)
	}
}

func TestLoadMessagesEmptySession(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	messages := database.LoadMessages("nonexistent-session")
	if len(messages) != 0 {
		test.Errorf("expected 0 messages for nonexistent session, got %d", len(messages))
	}
}

func TestSaveFactAndQueryFacts(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	database.SaveFact("language", "Go", "user")
	database.SaveFact("framework", "Heinzel", "system")
	database.SaveFact("language", "Python", "user")

	// Query by key
	facts := database.QueryFacts("language")
	if len(facts) != 2 {
		test.Fatalf("expected 2 facts matching 'language', got %d", len(facts))
	}

	// Query by value
	facts = database.QueryFacts("Heinzel")
	if len(facts) != 1 {
		test.Fatalf("expected 1 fact matching 'Heinzel', got %d", len(facts))
	}
	if facts[0].Key != "framework" || facts[0].Value != "Heinzel" {
		test.Errorf("expected framework/Heinzel, got %s/%s", facts[0].Key, facts[0].Value)
	}

	// Query with no match
	facts = database.QueryFacts("nonexistent")
	if len(facts) != 0 {
		test.Errorf("expected 0 facts, got %d", len(facts))
	}
}

func TestDeleteFact(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	database.SaveFact("color", "blue", "user")
	database.SaveFact("color", "red", "user")

	database.DeleteFact("color", "blue")

	facts := database.QueryFacts("color")
	if len(facts) != 1 {
		test.Fatalf("expected 1 fact after delete, got %d", len(facts))
	}
	if facts[0].Value != "red" {
		test.Errorf("expected remaining fact value 'red', got %q", facts[0].Value)
	}
}

func TestGetSettingAndSetSetting(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Initial get returns empty
	value := database.GetSetting("theme")
	if value != "" {
		test.Errorf("expected empty for unset key, got %q", value)
	}

	// Set and get
	database.SetSetting("theme", "dark")
	value = database.GetSetting("theme")
	if value != "dark" {
		test.Errorf("expected 'dark', got %q", value)
	}

	// Overwrite
	database.SetSetting("theme", "light")
	value = database.GetSetting("theme")
	if value != "light" {
		test.Errorf("expected 'light' after overwrite, got %q", value)
	}
}

func TestListSessionsAndCreateSession(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Initially empty
	sessions := database.ListSessions()
	if len(sessions) != 0 {
		test.Errorf("expected 0 sessions initially, got %d", len(sessions))
	}

	// Create sessions
	id1 := database.CreateSession("first")
	id2 := database.CreateSession("second")

	if id1 == "" || id2 == "" {
		test.Fatal("expected non-empty session IDs")
	}
	if id1 == id2 {
		test.Error("expected unique session IDs")
	}

	sessions = database.ListSessions()
	if len(sessions) != 2 {
		test.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify names are present
	names := map[string]bool{}
	for _, session := range sessions {
		names[session.Name] = true
	}
	if !names["first"] || !names["second"] {
		test.Errorf("expected sessions named 'first' and 'second', got %v", names)
	}
}

func TestCloseWithoutError(test *testing.T) {
	dbPath := filepath.Join(test.TempDir(), "test.db")
	database, err := OpenDB(dbPath)
	if err != nil {
		test.Fatalf("OpenDB failed: %v", err)
	}

	err = database.Close()
	if err != nil {
		test.Errorf("Close returned error: %v", err)
	}

	// Close on nil conn should also not error
	nilDB := &DB{}
	err = nilDB.Close()
	if err != nil {
		test.Errorf("Close on nil conn returned error: %v", err)
	}
}
