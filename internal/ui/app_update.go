package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sttts/kc/pkg/appconfig"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// enqueueCmd appends a command to be executed on the next Update cycle.
func (a *App) enqueueCmd(cmd tea.Cmd) {
	a.pendingCmdsMu.Lock()
	defer a.pendingCmdsMu.Unlock()
	if cmd != nil {
		a.pendingCmds = append(a.pendingCmds, cmd)
	}
}

func (a *App) updateTerminal(msg tea.Msg, reason string) tea.Cmd {
	if a.terminal == nil {
		return nil
	}
	model, cmd := a.terminal.Update(msg)
	a.terminal = model.(*Terminal)
	a.invalidateView(reason)
	a.invalidateTerminalArea(reason)
	return cmd
}

// Update handles messages and updates the application state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.cfg != nil {
		a.logMsg(msg)
	}

	var cmds []tea.Cmd
	a.pendingCmdsMu.Lock()
	if len(a.pendingCmds) > 0 {
		cmds = append(cmds, a.pendingCmds...)
		a.pendingCmds = nil
	}
	a.pendingCmdsMu.Unlock()
	viewWasValid := a.mainViewCache.valid

	// Always adapt size
	switch msg := msg.(type) {
	case startupIntentMsg:
		if cmd := a.applyStartupIntent(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)
	case commandWatchTickMsg:
		model, cmd := a.handleCommandWatchTick(msg)
		return model, cmd
	case namespaceRetryMsg:
		a.goToNamespaceWithRetry(msg.namespace, false)
		return a, tea.Batch(cmds...)
	case tea.WindowSizeMsg:
		msg.Width = max(40, msg.Width)
		msg.Height = max(5, msg.Height)

		sizeChanged := msg.Width != a.width || msg.Height != a.height
		if sizeChanged {
			a.width = msg.Width
			a.height = msg.Height
			a.invalidateView("window resize")
			a.invalidateFunctionBar("window resize")
			a.invalidateTerminalArea("window resize")

			// Ensure active modal scales with terminal size
			if a.modalManager != nil && a.modalManager.IsModalVisible() {
				if m := a.modalManager.GetActiveModal(); m != nil {
					m.SetDimensions(a.width, a.height)
					if vm, ok := m.content.(*ViewOptionsModel); ok {
						a.layoutViewOptionsModal(m, vm, vm.PanelIndex(), "")
					}
				}
			}
		}

		if a.terminal != nil {
			// Reserve space for status bar (1 line)
			// Terminal gets the remaining space
			terminalMsg := tea.WindowSizeMsg{
				Width:  msg.Width,
				Height: msg.Height - 1,
			}
			if cmd := a.updateTerminal(terminalMsg, "window resize"); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		if !sizeChanged && len(cmds) == 0 {
			return a, nil
		}
	}

	// Handle modals first
	if a.modalManager.IsModalVisible() {
		switch msg.(type) {
		case PanelModeSelectedMsg, DeleteConfirmMsg, NamespaceCreateResultMsg, CopyToLocalResultMsg:
			// Let modal result messages fall through to the main switch so they can close dialogs.
		default:
			// Intercept modal-scoped commits while the dialog is open.
			switch m := msg.(type) {
			case ViewOptionsCommittedMsg:
				if m.Viewer != nil {
					cmd := a.applyViewerOptions(m)
					return a, cmd
				}
				targetIdx := m.PanelIndex
				if targetIdx < 0 || targetIdx > 1 {
					targetIdx = a.activePanel
				}
				var subCmds []tea.Cmd
				closedBySubMsg := false
				if m.SetPanelMode && m.Accept {
					if _, cmd := a.Update(PanelModeSelectedMsg{PanelIndex: targetIdx, Mode: m.PanelMode}); cmd != nil {
						subCmds = append(subCmds, cmd)
					}
				}
				panel := a.panelByIndex(targetIdx)
				currentTableMode := "scroll"
				if panel != nil {
					currentTableMode = panel.TableMode()
				}
				widthSavePending := false
				saveHandled := false
				if m.SetPanelWidth && (m.Accept || m.SaveDefault) {
					targetPercent := clampPercent(m.PanelWidthPercent)
					a.setPanelWidthPercent(targetIdx, targetPercent)
					if m.SaveDefault {
						if a.cfg == nil {
							a.cfg = appconfig.Default()
						}
						a.cfg.Panel.Width.LeftPercent = a.leftPanelWidthPercent
						a.cfg.Panel.Width.RightPercent = a.rightPanelWidthPercent
						widthSavePending = true
					}
				}
				if m.Resources != nil && (m.Accept || m.SaveDefault) {
					tableMode := currentTableMode
					if m.HasTableMode {
						tableMode = m.TableMode
					}
					resMsg := ResourcesOptionsChangedMsg{
						ShowNonEmptyOnly: !m.Resources.IncludeEmpty,
						Order:            m.Resources.Order,
						TableMode:        tableMode,
						HasInclude:       m.Resources.HasInclude,
						HasOrder:         m.Resources.HasOrder,
						SaveDefault:      m.SaveDefault,
						Accept:           m.Accept,
						Close:            m.Close,
					}
					if _, cmd := a.Update(resMsg); cmd != nil {
						subCmds = append(subCmds, cmd)
					}
					if m.Close {
						closedBySubMsg = true
					}
					if m.SaveDefault {
						saveHandled = true
					}
				}
				if m.Objects != nil && (m.Accept || m.SaveDefault) {
					tableMode := currentTableMode
					if m.HasTableMode {
						tableMode = m.TableMode
					}
					objMsg := ObjectOptionsChangedMsg{
						TableMode:    tableMode,
						Columns:      m.Objects.Columns,
						ObjectsOrder: m.Objects.Order,
						SaveDefault:  m.SaveDefault,
						Accept:       m.Accept,
						Close:        m.Close,
					}
					if _, cmd := a.Update(objMsg); cmd != nil {
						subCmds = append(subCmds, cmd)
					}
					if m.Close {
						closedBySubMsg = true
					}
					if m.SaveDefault {
						saveHandled = true
					}
				}
				if m.Command != nil && (m.Accept || m.SaveDefault) {
					cmdMsg := CommandOptionsChangedMsg{
						WatchInterval: m.Command.WatchInterval,
						SaveDefault:   m.SaveDefault,
						Accept:        m.Accept,
						Close:         m.Close,
					}
					if _, cmd := a.Update(cmdMsg); cmd != nil {
						subCmds = append(subCmds, cmd)
					}
					if m.Close {
						closedBySubMsg = true
					}
				}
				if widthSavePending && !saveHandled {
					_ = appconfig.Save(a.cfg)
				}
				if m.Close && !closedBySubMsg {
					a.hideModal()
				}
				return a, tea.Batch(subCmds...)
			case ResourcesOptionsChangedMsg:
				if m.SaveDefault {
					// Persist current dialog values to config defaults
					if a.cfg == nil {
						a.cfg = appconfig.Default()
					}
					if m.HasInclude {
						a.cfg.Resources.ShowNonEmptyOnly = m.ShowNonEmptyOnly
					}
					if m.HasOrder {
						a.cfg.Resources.Order = appconfig.ResourcesViewOrder(m.Order)
					}
					a.cfg.Panel.Table.Mode = appconfig.TableMode(m.TableMode)
					_ = appconfig.Save(a.cfg)
					a.applyResourceOptions(a.leftPanel)
					a.applyResourceOptions(a.rightPanel)
				}
				if m.Accept {
					// Apply to active panel only; do not persist
					if a.activePanel == 0 {
						ctxTable, cancelTable := context.WithTimeout(a.ctx, panelContextTimeout)
						a.leftPanel.SetTableMode(ctxTable, m.TableMode)
						cancelTable()
						if m.HasInclude || m.HasOrder {
							a.leftPanel.SetResourceViewOptions(m.ShowNonEmptyOnly, m.Order)
						}
						a.syncPanelConfig(a.leftPanel)
						a.applyResourceOptions(a.leftPanel)
					} else {
						ctxTable, cancelTable := context.WithTimeout(a.ctx, panelContextTimeout)
						a.rightPanel.SetTableMode(ctxTable, m.TableMode)
						cancelTable()
						if m.HasInclude || m.HasOrder {
							a.rightPanel.SetResourceViewOptions(m.ShowNonEmptyOnly, m.Order)
						}
						a.syncPanelConfig(a.rightPanel)
						a.applyResourceOptions(a.rightPanel)
					}
					// Refresh only the active panel's folder
					if a.activePanel == 0 && a.leftNav != nil {
						ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
						if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok {
							rf.Refresh()
						}
						a.setPanelFolder(ctx, 0, a.leftNav.Current(), a.leftNav.HasBack())
						a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
						cancel()
						ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
						a.leftPanel.RefreshFolder(ctxRefresh)
						cancelRefresh()
					}
					if a.activePanel == 1 && a.rightNav != nil {
						ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
						if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok {
							rf.Refresh()
						}
						a.setPanelFolder(ctx, 1, a.rightNav.Current(), a.rightNav.HasBack())
						a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))
						cancel()
						ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
						a.rightPanel.RefreshFolder(ctxRefresh)
						cancelRefresh()
					}
				}
				if m.Close {
					a.hideModal()
				}
				return a, nil
			case ObjectOptionsChangedMsg:
				if m.SaveDefault {
					if a.cfg == nil {
						a.cfg = appconfig.Default()
					}
					// Save table mode
					switch strings.ToLower(m.TableMode) {
					case "fit":
						a.cfg.Panel.Table.Mode = appconfig.TableModeFit
					default:
						a.cfg.Panel.Table.Mode = appconfig.TableModeScroll
					}
					// Save columns mode to objects.columns
					if strings.EqualFold(m.Columns, "wide") {
						a.cfg.Objects.Columns = "wide"
					} else {
						a.cfg.Objects.Columns = "normal"
					}
					// Save objects order
					a.cfg.Objects.Order = m.ObjectsOrder
					_ = appconfig.Save(a.cfg)
				}
				if a.activePanel == 0 {
					ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
					a.leftPanel.SetTableMode(ctxPanel, m.TableMode)
					a.leftPanel.SetColumnsMode(ctxPanel, m.Columns)
					a.leftPanel.SetObjectOrder(ctxPanel, m.ObjectsOrder)
					a.syncPanelConfig(a.leftPanel)
					cancelPanel()
					if a.leftNav != nil {
						if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok {
							rf.Refresh()
						}
						ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
						a.leftPanel.RefreshFolder(ctxRefresh)
						cancelRefresh()
					}
				} else {
					ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
					a.rightPanel.SetTableMode(ctxPanel, m.TableMode)
					a.rightPanel.SetColumnsMode(ctxPanel, m.Columns)
					a.rightPanel.SetObjectOrder(ctxPanel, m.ObjectsOrder)
					a.syncPanelConfig(a.rightPanel)
					cancelPanel()
					if a.rightNav != nil {
						if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok {
							rf.Refresh()
						}
						ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
						a.rightPanel.RefreshFolder(ctxRefresh)
						cancelRefresh()
					}
				}
				if m.Close {
					a.hideModal()
				}
				return a, nil
			case CommandOptionsChangedMsg:
				panel := a.activePanelRef()
				panelIdx := a.activePanel
				if panel != nil {
					ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
					if cmd := panel.SetCommandWatchInterval(ctxPanel, m.WatchInterval); cmd != nil {
						cmds = append(cmds, cmd)
					}
					cancelPanel()
				}
				if panelIdx >= 0 && panelIdx <= 1 {
					if cmd := a.setCommandWatchInterval(panelIdx, m.WatchInterval); cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
				if m.Close {
					a.hideModal()
				}
				return a, tea.Batch(cmds...)
			}
			model, cmd := a.modalManager.Update(msg)
			a.modalManager = model.(*ModalManager)
			cmds = append(cmds, cmd)
			// While a modal is open, still forward non-key messages to the
			// terminal (process output, window size). Background is snapshotted,
			// so this stays light and keeps the 2-line terminal fresh.
			if _, isKey := msg.(tea.KeyMsg); !isKey && a.terminal != nil {
				if tcmd := a.updateTerminal(msg, "modal forward"); tcmd != nil {
					cmds = append(cmds, tcmd)
				}
			}
			return a, tea.Batch(cmds...)
		}
	}

	// Check if terminal process has exited (check on every message)
	if a.terminal.IsProcessExited() {
		return a, tea.Quit
	}

	switch msg := msg.(type) {
	case BusyShowMsg:
		if msg.token == a.busyToken {
			a.busyActive = true
			a.busyFrame = 0
			a.invalidateView("busy show")
			return a, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return BusyTickMsg{} })
		}
		return a, nil
	case BusyTickMsg:
		if a.busyActive {
			a.busyFrame = (a.busyFrame + 1) % 10
			a.invalidateView("busy tick")
			return a, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return BusyTickMsg{} })
		}
		return a, nil
	case BusyHideMsg:
		if msg.token == a.busyToken {
			a.busyActive = false
			a.invalidateView("busy hide")
		}
		return a, nil
	case busyDoneMsg:
		if msg.token == a.busyToken {
			a.busyActive = false
			a.busyToken++
			a.invalidateView("busy done")
		}
		// Re-dispatch the original message for normal handling
		return a, func() tea.Msg { return msg.msg }
	case showToastMsg:
		a.toastActive = true
		a.toastText = msg.text
		a.toastUntil = time.Now().Add(msg.ttl)
		a.invalidateView("toast show")
		a.invalidateFunctionBar("toast show")
		return a, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
	case toastTickMsg:
		if a.toastActive {
			if time.Now().After(a.toastUntil) {
				a.toastActive = false
				a.invalidateView("toast hide")
				a.invalidateFunctionBar("toast hide")
			} else {
				return a, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
			}
		}
		return a, nil
	case kubectlEditFinishedMsg:
		if msg.tempConfig != "" {
			_ = os.Remove(msg.tempConfig)
		}
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("kubectl edit %s failed: %v", msg.resourceRef, msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", msg.err), 5*time.Second))
			}
			return a, nil
		}
		a.refreshPanelAfterEdit(msg.panelIndex)
		return a, nil
	case NamespaceCreateResultMsg:
		if msg.Close {
			a.hideModal()
			if a.namespaceInput != nil {
				a.namespaceInput.Reset()
			}
			if !msg.Confirm {
				a.namespaceCreatePanel = -1
			}
		}
		if msg.Confirm {
			if a.cl == nil {
				if a.toastLogger != nil {
					a.enqueueCmd(a.toastLogger.Errorf("Cluster not ready for namespace creation"))
				}
				return a, nil
			}
			return a, a.createNamespaceWithName(msg.Name)
		}
		return a, nil
	case CopyToLocalResultMsg:
		if msg.Close {
			a.hideModal()
			if a.copyInput != nil {
				a.copyInput.BlurInputs()
			}
		}
		if msg.Confirm {
			if path := strings.TrimSpace(msg.Path); path != "" {
				a.lastCopyDir = filepath.Dir(path)
				return a, a.executeCopyRequest(path)
			}
			a.pendingCopy = nil
			return a, nil
		}
		a.pendingCopy = nil
		return a, nil
	case namespaceCreatedMsg:
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("Create namespace %s failed: %v", msg.name, msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("Create namespace failed: %v", msg.err), 5*time.Second))
			}
			a.namespaceCreatePanel = -1
			return a, nil
		}
		a.enqueueCmd(a.ShowToast(fmt.Sprintf("Namespace %s created", msg.name), 3*time.Second))
		if a.namespaceCreatePanel == 0 || a.namespaceCreatePanel == 1 {
			a.refreshPanelAfterEdit(a.namespaceCreatePanel)
			other := 1 - a.namespaceCreatePanel
			if other == 0 || other == 1 {
				var otherPanel *Panel
				if other == 0 {
					otherPanel = a.leftPanel
				} else {
					otherPanel = a.rightPanel
				}
				if otherPanel != nil && otherPanel.GetCurrentPath() == "/namespaces" {
					a.refreshPanelAfterEdit(other)
				}
			}
		} else {
			a.refreshPanelAfterEdit(0)
			a.refreshPanelAfterEdit(1)
		}
		a.namespaceCreatePanel = -1
		return a, nil
	case copyFinishedMsg:
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("Copy %s failed: %v", msg.subject, msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("Copy failed: %v", msg.err), 5*time.Second))
			}
			return a, nil
		}
		if msg.path != "" {
			a.lastCopyDir = filepath.Dir(msg.path)
		}
		log := ctrllog.FromContext(a.ctx).WithName("copy")
		log.Info("copy completed", "subject", msg.subject, "path", msg.path)
		return a, nil
	case DeleteConfirmMsg:
		target := a.pendingDelete
		if msg.Close {
			a.hideModal()
		}
		// Clear pending delete regardless of confirmation outcome once we've captured it.
		a.pendingDelete = nil
		if target != nil && msg.Confirm {
			return a, a.performDelete(*target)
		}
		return a, nil
	case PanelSelectionChangedMsg:
		if panel := msg.Panel; panel != nil {
			ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
			panel.NotifySelection(ctxSel, msg.Selection)
			cancelSel()
			if idx, ok := a.panelIndexFor(panel); ok {
				other := a.panelByIndex(1 - idx)
				if other != nil {
					ctxOther, cancelOther := context.WithTimeout(a.ctx, panelContextTimeout)
					other.NotifySelection(ctxOther, msg.Selection)
					cancelOther()
				}
			}
		}
		a.invalidateFunctionBar("selection changed")
		a.invalidateView("selection changed")
		return a, nil
	case PanelModeSelectedMsg:
		if panel := a.panelByIndex(msg.PanelIndex); panel != nil {
			log := ctrllog.FromContext(a.ctx).WithName("panelMode").WithValues("panelIdx", msg.PanelIndex, "mode", msg.Mode)
			log.Info("switching panel mode")
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			modeCmd := panel.SetMode(ctx, msg.Mode)
			cancel()
			if modeCmd != nil {
				cmds = append(cmds, modeCmd)
			}
			ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
			source := a.panelByIndex(1 - msg.PanelIndex)
			if source == nil {
				source = panel
			}
			if source != nil {
				sel := source.CurrentSelection(ctxSel)
				if sel.Item == nil {
					if item, ok := source.SelectedNavItem(ctxSel); ok {
						sel.Item = item
					}
				}
				if sel.ID != "" || sel.Item != nil {
					log.Info("replaying selection", "selectionID", sel.ID, "path", sel.Path)
					sel.Force = true
					panel.NotifySelection(ctxSel, sel)
				} else {
					log.Info("missing selection to replay")
				}
			}
			cancelSel()
			a.syncPanelConfig(panel)
			if nav := a.navigatorForPanel(panel); nav != nil {
				if rf, ok := nav.Current().(interface{ Refresh() }); ok {
					rf.Refresh()
				}
			}
		}
		a.hideModalName(panelModeModalKey(msg.PanelIndex))
		return a, tea.Batch(cmds...)
	case resourceDeletedMsg:
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("Delete %s failed: %v", kubectlResourceRef(msg.target.gvr, msg.target.name), msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("Delete failed: %v", msg.err), 5*time.Second))
			}
			return a, nil
		}
		a.refreshPanelAfterEdit(msg.target.panelIdx)
		return a, nil
	case EscTimeoutMsg:
		// Escape sequence timed out
		a.escPressed = false
		return a, nil
	case FolderDirtyMsg:
		panel := a.panelByIndex(msg.PanelIdx)
		if panel == nil {
			return a, nil
		}
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		panel.RefreshFolder(ctx)
		cancel()
		a.invalidateView(fmt.Sprintf("folder dirty panel %d", msg.PanelIdx))
		// Always wake the renderer when data changed so cached frames are dropped
		return a, tea.Batch(
			tea.Tick(0, func(time.Time) tea.Msg { return nil }),
			a.waitFolderDirtyEvents(),
		)
	case DiscoveryRefreshedMsg:
		a.handleDiscoveryRefresh()
		return a, a.watchDiscovery()

	case tea.KeyMsg:
		return a.handleKeyMsg(msg, cmds)
	case commandExitMsg:
		if msg.OnExit == appconfig.CommandExitRestore || msg.OnExit == appconfig.CommandExitClose {
			if panel := a.panelByIndex(msg.PanelIdx); panel != nil {
				ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
				modeCmd := panel.SetMode(ctx, PanelModeList)
				panel.setCommandFocus(false)
				cancel()
				if modeCmd != nil {
					cmds = append(cmds, modeCmd)
				}
				a.invalidateView("command exit restore")
				a.invalidateFunctionBar("command exit restore")
			}
		}
		return a, tea.Batch(cmds...)

	default:
		if mm, ok := msg.(tea.MouseMsg); ok {
			if a.showTerminal {
				m := mm.Mouse()
				if m.Y == a.height-1 {
					if rel, ok := mm.(tea.MouseReleaseMsg); ok && rel.Mouse().Button == tea.MouseLeft {
						a.showTerminal = false
						a.terminal.SetShowPanels(true)
						a.invalidateView("terminal bar click exit")
					}
					return a, nil
				}
				if cmd := a.updateTerminal(mm, "terminal mouse"); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return a, tea.Batch(cmds...)
			}
			m := mm.Mouse()
			if m.Y == a.height-1 {
				if rel, ok := mm.(tea.MouseReleaseMsg); ok && rel.Mouse().Button == tea.MouseLeft {
					if cmd := a.handleFunctionKeyClick(m.X); cmd != nil {
						return a, cmd
					}
				}
				return a, nil
			}
			if cmd, panel, panelMsg, panelIdx, handled := a.dispatchPanelMouse(mm); handled {
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				if panelMsg.Intent == PanelMouseClick && panelMsg.Button == tea.MouseLeft && panel != nil {
					ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
					selectionID := panel.currentSelectionID(ctxSel)
					cancelSel()
					if selectionID != "" {
						now := time.Now()
						timeout := a.cfg.Input.Mouse.DoubleClickTimeout.Duration
						if timeout <= 0 {
							timeout = 300 * time.Millisecond
						}
						if a.lastClickRowID == selectionID && a.lastClickPanel == panelIdx && now.Sub(a.lastClickTime) <= timeout {
							a.lastClickTime = time.Time{}
							a.lastClickRowID = ""
							ctxEnter, cancelEnter := context.WithTimeout(a.ctx, panelContextTimeout)
							enterCmd := panel.Enter(ctxEnter)
							cancelEnter()
							if enterCmd != nil {
								return a, enterCmd
							}
						} else {
							a.lastClickTime = now
							a.lastClickPanel = panelIdx
							a.lastClickRowID = selectionID
						}
					}
				}
				return a, tea.Batch(cmds...)
			}
			return a, tea.Batch(cmds...)
		}
		if !a.showTerminal {
			if panelCmds := a.updatePanelsWithMsg(msg); len(panelCmds) > 0 {
				cmds = append(cmds, panelCmds...)
				a.invalidateView("panel update msg")
			}
		}
		if cmd := a.updateTerminal(msg, ""); cmd != nil {
			cmds = append(cmds, cmd)
		}

	}

	if len(cmds) == 0 && viewWasValid && a.mainViewCache.valid {
		return a, nil
	}
	return a, tea.Batch(cmds...)
}

// shouldRouteToPanel determines if a key should be routed to the panel based on terminal state

func (a *App) dispatchPanelMouse(msg tea.MouseMsg) (tea.Cmd, *Panel, PanelMouseMsg, int, bool) {
	leftPanelWidth, rightPanelWidth, panelHeight, headerOffset := a.panelAreaMetrics()
	m := msg.Mouse()
	if m.Y >= panelHeight {
		return nil, nil, PanelMouseMsg{}, 0, false
	}
	panelIdx := 0
	if leftPanelWidth <= 0 && rightPanelWidth > 0 {
		panelIdx = 1
	} else if rightPanelWidth > 0 && leftPanelWidth > 0 && m.X >= leftPanelWidth {
		panelIdx = 1
	}
	panel := a.panelByIndex(panelIdx)
	if panel == nil {
		return nil, nil, PanelMouseMsg{}, panelIdx, false
	}
	relRow := m.Y - headerOffset
	if relRow < 0 {
		relRow = 0
	}
	var panelMsg PanelMouseMsg
	switch mm := msg.(type) {
	case tea.MouseWheelMsg:
		delta := 0
		switch mm.Button {
		case tea.MouseWheelUp:
			delta = -1
		case tea.MouseWheelDown:
			delta = 1
		default:
			return nil, nil, PanelMouseMsg{}, panelIdx, false
		}
		panelMsg = PanelMouseMsg{
			Intent: PanelMouseWheel,
			DeltaY: delta,
		}
	case tea.MouseClickMsg:
		panelMsg = PanelMouseMsg{
			Intent: PanelMouseClick,
			Row:    relRow,
			Button: mm.Button,
		}
	default:
		return nil, nil, PanelMouseMsg{}, panelIdx, false
	}
	if panelIdx != a.activePanel {
		a.setActivePanel(panelIdx, "panel mouse focus")
	}
	model, cmd := panel.Update(panelMsg)
	if model != nil {
		if panelIdx == 0 {
			a.leftPanel = model.(*Panel)
		} else {
			a.rightPanel = model.(*Panel)
		}
	}
	return cmd, panel, panelMsg, panelIdx, true
}

func (a *App) handleCommandWatchTick(msg commandWatchTickMsg) (tea.Model, tea.Cmd) {
	if msg.PanelIdx < 0 || msg.PanelIdx > 1 {
		return a, nil
	}
	// Ignore stale ticks.
	if msg.Token != a.commandWatchToken[msg.PanelIdx] {
		return a, nil
	}
	interval := a.commandWatchInterval[msg.PanelIdx]
	if interval <= 0 {
		return a, nil
	}
	panel := a.panelByIndex(msg.PanelIdx)
	if panel == nil {
		return a, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	restart := panel.RestartCommand(ctx)
	next := a.scheduleCommandWatch(msg.PanelIdx)
	return a, tea.Batch(restart, next)
}
