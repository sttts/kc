package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sttts/kc/pkg/appconfig"
)

// mockModel is a simple mock model for testing
type mockModel struct {
	content string
}

type capturingModel struct {
	last tea.Msg
}

func (m *mockModel) Init() tea.Cmd {
	return nil
}

func (m *mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *mockModel) View() tea.View {
	return tea.NewView(m.content)
}

func (m *mockModel) SetDimensions(width, height int) {
	// Mock implementation
}

func (m *capturingModel) Init() tea.Cmd { return nil }

func (m *capturingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.last = msg
	return m, nil
}

func (m *capturingModel) View() tea.View { return tea.NewView("") }

func (m *capturingModel) SetDimensions(width, height int) {}

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
	manager := NewModalManager(appconfig.Default())

	if manager == nil {
		t.Fatal("NewModalManager returned nil")
	}

	if manager.modals == nil {
		t.Error("Expected modals map to be initialized")
	}

	if len(manager.stack) != 0 {
		t.Error("Expected no active modal initially")
	}
}

func TestModalManagerRegister(t *testing.T) {
	manager := NewModalManager(appconfig.Default())
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	manager.Register("test", modal)

	if manager.modals["test"] != modal {
		t.Error("Expected modal to be registered")
	}
}

func TestModalManagerShow(t *testing.T) {
	manager := NewModalManager(appconfig.Default())
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	manager.Register("test", modal)
	manager.Show("test")

	if !manager.IsModalVisible() {
		t.Error("Expected modal to be visible")
	}

	if !(len(manager.stack) > 0 && manager.stack[len(manager.stack)-1] == "test") {
		t.Errorf("Expected 'test' to be the active modal")
	}

	if !modal.IsVisible() {
		t.Error("Expected modal to be visible after Show()")
	}
}

func TestModalManagerHide(t *testing.T) {
	manager := NewModalManager(appconfig.Default())
	content := &mockModel{content: "test content"}
	modal := NewModal("Test Modal", content)

	manager.Register("test", modal)
	manager.Show("test")
	manager.Hide()

	if manager.IsModalVisible() {
		t.Error("Expected modal to not be visible after Hide()")
	}

	if len(manager.stack) != 0 {
		t.Error("Expected no active modal after Hide()")
	}

	if modal.IsVisible() {
		t.Error("Expected modal to not be visible after Hide()")
	}
}

func TestModalManagerGetActiveModal(t *testing.T) {
	manager := NewModalManager(appconfig.Default())
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

func TestModalEscDigitMapsToFunctionKey(t *testing.T) {
	content := &capturingModel{}
	modal := NewModal("Fullscreen", content)
	modal.SetCloseOnSingleEsc(false)
	modal.Show()

	if _, cmd := modal.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})); cmd == nil {
		// ignore; command schedules timeout
	}
	modal.Update(tea.KeyPressMsg(tea.Key{Code: '2', Text: "2"}))

	keyMsg, ok := content.last.(tea.KeyMsg)
	if !ok {
		t.Fatalf("expected key message, got %T", content.last)
	}
	if got := keyMsg.Key().Code; got != tea.KeyF2 {
		t.Fatalf("expected F2 key code, got %v", got)
	}
}

func TestModalDoubleEscClose(t *testing.T) {
	content := &capturingModel{}
	modal := NewModal("Fullscreen", content)
	modal.SetCloseOnSingleEsc(false)
	modal.Show()

	modal.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if !modal.IsVisible() {
		t.Fatalf("modal should remain visible after first ESC when single-close disabled")
	}

	modal.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if modal.IsVisible() {
		t.Fatalf("modal should close on second ESC")
	}
}

func TestModalEscDelegatedToContent(t *testing.T) {
	viewer := NewTextViewer("title", "body", "yaml", "application/yaml", "title", "dracula", nil, nil, nil)
	modal := NewModal("Viewer", viewer)
	modal.SetMode(ModalModeFullscreen)
	modal.SetCloseOnSingleEsc(false)
	modal.Show()

	modal.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF7}))
	if !viewer.inner.SearchMode() {
		t.Fatalf("viewer should enter search mode after F7")
	}

	modal.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if !modal.IsVisible() {
		t.Fatalf("modal should remain visible when content handles ESC")
	}
	if viewer.inner.SearchMode() {
		t.Fatalf("viewer search mode should be cleared after ESC")
	}
}
