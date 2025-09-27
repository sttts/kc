package ui

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea/v2"
)

// mockModel is a simple mock model for testing
type mockModel struct {
	content string
}

func (m *mockModel) Init() tea.Cmd {
	return nil
}

func (m *mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *mockModel) View() string {
	return m.content
}

func (m *mockModel) SetDimensions(width, height int) {
	// Mock implementation
}

func TestNewModal(t *testing.T) {
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	if modal == nil {
		t.Fatal("NewModal returned nil")
	}

	if modal.title != "Test Modal" {
		t.Errorf("Expected title 'Test Modal', got '%s'", modal.title)
	}

	if modal.content != content {
		t.Error("Expected content to be set correctly")
	}

	if modal.visible {
		t.Error("Expected modal to not be visible initially")
	}
}

func TestModalShowHide(t *testing.T) {
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	// Initially not visible
	if modal.IsVisible() {
		t.Error("Expected modal to not be visible initially")
	}

	// Show modal
	modal.Show()
	if !modal.IsVisible() {
		t.Error("Expected modal to be visible after Show()")
	}

	// Hide modal
	modal.Hide()
	if modal.IsVisible() {
		t.Error("Expected modal to not be visible after Hide()")
	}
}

func TestModalSetDimensions(t *testing.T) {
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	modal.SetDimensions(100, 50)

	if modal.width != 100 {
		t.Errorf("Expected width to be 100, got %d", modal.width)
	}

	if modal.height != 50 {
		t.Errorf("Expected height to be 50, got %d", modal.height)
	}
}

func TestModalSetOnClose(t *testing.T) {
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	called := false
	modal.SetOnClose(func() tea.Cmd {
		called = true
		return nil
	})

	// Simulate close
	if modal.onClose != nil {
		modal.onClose()
		if !called {
			t.Error("Expected onClose callback to be called")
		}
	}
}

func TestNewModalManager(t *testing.T) {
	manager := NewModalManager()

	if manager == nil {
		t.Fatal("NewModalManager returned nil")
	}

	if manager.modals == nil {
		t.Error("Expected modals map to be initialized")
	}

	if manager.active != "" {
		t.Error("Expected active to be empty initially")
	}
}

func TestModalManagerRegister(t *testing.T) {
	manager := NewModalManager()
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	manager.Register("test", modal)

	if manager.modals["test"] != modal {
		t.Error("Expected modal to be registered")
	}
}

func TestModalManagerShow(t *testing.T) {
	manager := NewModalManager()
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	manager.Register("test", modal)
	manager.Show("test")

	if !manager.IsModalVisible() {
		t.Error("Expected modal to be visible")
	}

	if manager.active != "test" {
		t.Errorf("Expected active to be 'test', got '%s'", manager.active)
	}

	if !modal.IsVisible() {
		t.Error("Expected modal to be visible after Show()")
	}
}

func TestModalManagerHide(t *testing.T) {
	manager := NewModalManager()
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	manager.Register("test", modal)
	manager.Show("test")
	manager.Hide()

	if manager.IsModalVisible() {
		t.Error("Expected modal to not be visible after Hide()")
	}

	if manager.active != "" {
		t.Error("Expected active to be empty after Hide()")
	}

	if modal.IsVisible() {
		t.Error("Expected modal to not be visible after Hide()")
	}
}

func TestModalManagerGetActiveModal(t *testing.T) {
	manager := NewModalManager()
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	// No active modal initially
	if manager.GetActiveModal() != nil {
		t.Error("Expected no active modal initially")
	}

	manager.Register("test", modal)
	manager.Show("test")

	activeModal := manager.GetActiveModal()
	if activeModal != modal {
		t.Error("Expected active modal to be returned")
	}
}
