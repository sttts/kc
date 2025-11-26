package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	readmedoc "github.com/sttts/kc"
	kccluster "github.com/sttts/kc/internal/cluster"
	models "github.com/sttts/kc/internal/models"
	navui "github.com/sttts/kc/internal/navigation"
	"github.com/sttts/kc/internal/podfs"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	describewidget "github.com/sttts/kc/internal/ui/panelcontent/describe"
	manifestwidget "github.com/sttts/kc/internal/ui/panelcontent/manifest"
	"github.com/sttts/kc/internal/ui/termctx"
	viewpkg "github.com/sttts/kc/internal/ui/viewer"
	"github.com/sttts/kc/pkg/appconfig"
	describe "github.com/sttts/kc/pkg/describe"
	"github.com/sttts/kc/pkg/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metamapper "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// EscTimeoutMsg is sent when the escape sequence times out
type EscTimeoutMsg struct{}

// FolderDirtyMsg requests an immediate refresh for the given panel.
type FolderDirtyMsg struct {
	PanelIdx int
}

func (a *App) hasApplicableCommands(panel *Panel) bool {
	if panel == nil || len(a.cfg.Commands) == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	item, _ := panel.SelectedNavItem(ctx)
	selected := panel.GetSelectedItems()
	activeNamespace := deriveNamespace(item, selected, panel.currentPath)
	for _, cmd := range a.cfg.Commands {
		if isCommandApplicable(cmd, item, len(selected), activeNamespace) {
			return true
		}
	}
	return false
}

// DiscoveryRefreshedMsg is emitted when API discovery invalidates; resource folders should refresh.
type DiscoveryRefreshedMsg struct{}

// kubectlEditFinishedMsg notifies that a kubectl edit invocation exited.
type kubectlEditFinishedMsg struct {
	err         error
	panelIndex  int
	resourceRef string
	tempConfig  string
}

type namespaceCreatedMsg struct {
	name string
	err  error
}

type namespaceRetryMsg struct {
	namespace string
}

type commandWatchTickMsg struct {
	PanelIdx int
	Token    int
}

type startupIntentMsg struct{}

type deleteTarget struct {
	panelIdx  int
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

type cachedView struct {
	view   string
	cursor *tea.Cursor
	valid  bool
}

type resourceDeletedMsg struct {
	target deleteTarget
	err    error
}

func (a *App) setCommandWatchInterval(panelIdx int, interval time.Duration) tea.Cmd {
	if panelIdx < 0 || panelIdx > 1 {
		return nil
	}
	a.commandWatchInterval[panelIdx] = interval
	// Invalidate outstanding ticks.
	a.commandWatchToken[panelIdx]++
	if interval <= 0 {
		return nil
	}
	token := a.commandWatchToken[panelIdx]
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return commandWatchTickMsg{PanelIdx: panelIdx, Token: token}
	})
}

func (a *App) scheduleCommandWatch(panelIdx int) tea.Cmd {
	if panelIdx < 0 || panelIdx > 1 {
		return nil
	}
	interval := a.commandWatchInterval[panelIdx]
	a.commandWatchToken[panelIdx]++
	if interval <= 0 {
		return nil
	}
	token := a.commandWatchToken[panelIdx]
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return commandWatchTickMsg{PanelIdx: panelIdx, Token: token}
	})
}

type viewerOptionsTarget interface {
	SetTheme(string)
	Theme() string
	SetWrapMode(bool)
	WrapMode() bool
}

// App represents the main application state
type App struct {
	leftPanel    *Panel
	rightPanel   *Panel
	terminal     *Terminal
	termCtx      *termctx.Manager
	modalManager *ModalManager
	width        int
	height       int
	activePanel  int // 0 = left, 1 = right
	showTerminal bool
	allResources []schema.GroupVersionKind
	// Esc sequence tracking
	escPressed bool
	// Data providers
	kubeMgr           *kubeconfig.Manager
	cl                *kccluster.Cluster
	clPool            *kccluster.Pool
	ctx               context.Context
	cancel            context.CancelFunc
	currentCtx        *kubeconfig.Context
	impersonateUser   string
	impersonateUID    string
	impersonateGroups []string
	viewConfig        *ViewConfig
	cfg               *appconfig.Config
	// Command watch scheduling
	commandWatchToken    [2]int
	commandWatchInterval [2]time.Duration
	// New navigation (folder-backed) using a Navigator
	leftNav  *navui.Navigator
	rightNav *navui.Navigator
	// Mouse double-click detection
	lastClickTime  time.Time
	lastClickPanel int
	lastClickRowID string
	// Suppress forwarding of mouse to terminal immediately after toggling fullscreen
	suppressMouseUntil time.Time
	// Resources options dialog state
	prevResShowNonEmpty bool
	prevResOrder        string
	resOptsChanged      bool
	resOptsConfirmed    bool
	// Busy spinner state (lightweight, non-intrusive)
	busyActive bool
	busyLabel  string
	busyFrame  int
	busyToken  int
	// Toast notification state (auto-dismiss)
	toastActive bool
	toastText   string
	toastUntil  time.Time
	// Logger that emits toasts on errors with rate limiting
	toastLogger            *ToastLogger
	pendingCmdsMu          sync.Mutex
	pendingCmds            []tea.Cmd
	leftConfig             *appconfig.Config
	rightConfig            *appconfig.Config
	namespaceInput         *NamespaceCreateModel
	deleteConfirm          *DeleteConfirmModel
	copyInput              *CopyToLocalModel
	pendingCopy            *copyRequest
	pendingDelete          *deleteTarget
	lastCopyDir            string
	namespaceCreatePanel   int
	leftPanelWidthPercent  int
	rightPanelWidthPercent int
	leftFolderDirtyCancel  func()
	rightFolderDirtyCancel func()
	viewerOptionsTarget    viewerOptionsTarget
	// Discovery refresh notifications
	discoveryCh     chan struct{}
	discoveryCancel func()
	// Recent Bubble Tea messages for idle-loop diagnostics
	msgLog []string
	// Cached views
	mainViewCache     cachedView
	functionBarCache  cachedView
	terminalAreaCache cachedView
	// Namespace auto-navigation state
	namespaceAutoTarget    string
	namespaceAutoAttempts  int
	namespaceOverride      string
	startupIntent          StartupIntent
	startupIntentScheduled bool
	startupIntentApplied   bool
	helpViewer             *MarkdownHelpViewer
	podfsFactory           podfs.Factory
	folderDirtyCh          chan FolderDirtyMsg
}

const (
	requestTimeout            = 10 * time.Second
	namespaceRetryInterval    = 200 * time.Millisecond
	namespaceRetryMaxAttempts = 50
	copyRequestTimeout        = 30 * time.Second
	msgLogLimit               = 512
)

var panelWidthPercentOptions = []int{25, 33, 50, 66, 75, 100}

// Invariant: a.cfg is always non-nil. NewApp initializes it with defaults and
// Init() loads and overwrites with persisted config, never leaving it nil.

// NewApp creates a new application instance
func NewApp() *App {
	app := &App{
		leftPanel:    NewPanel(""),
		rightPanel:   NewPanel(""),
		terminal:     NewTerminal(),
		modalManager: NewModalManager(),
		activePanel:  0,
		showTerminal: false,
		allResources: make([]schema.GroupVersionKind, 0),
		escPressed:   false,
		viewConfig:   NewViewConfig(),
		// Invariant: cfg is always non-nil; initialize with defaults
		cfg:                    appconfig.Default(),
		namespaceCreatePanel:   -1,
		leftPanelWidthPercent:  50,
		rightPanelWidthPercent: 50,
		folderDirtyCh:          make(chan FolderDirtyMsg, 64),
	}
	app.ctx, app.cancel = context.WithCancel(context.Background())
	app.terminal.SetLogger(ctrllog.Log.WithName("terminal"))
	app.toastLogger = NewToastLogger(app, 2*time.Second)

	// Register modals
	app.setupModals()
	app.setupPanelInputs()

	return app
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	// Load config (best-effort)
	cfg, err := appconfig.Load()
	if err != nil {
		cfg = appconfig.Default()
	}
	a.cfg = cfg
	leftPercent, rightPercent := normalizePanelWidthPercents(cfg.Panel.Width.LeftPercent, cfg.Panel.Width.RightPercent)
	a.leftPanelWidthPercent = leftPercent
	a.rightPanelWidthPercent = rightPercent
	a.leftConfig = cloneConfig(cfg)
	a.rightConfig = cloneConfig(cfg)
	return tea.Batch(
		a.leftPanel.Init(),
		a.rightPanel.Init(),
		a.terminal.Init(),
		func() tea.Msg {
			// Focus the terminal initially since it's the main input area
			a.terminal.Focus()
			return nil
		},
		a.waitFolderDirtyEvents(),
	)
}

func (a *App) waitFolderDirtyEvents() tea.Cmd {
	if a == nil || a.folderDirtyCh == nil {
		return nil
	}
	ctx := a.ctx
	ch := a.folderDirtyCh
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			return msg
		}
	}
}

func (a *App) signalFolderDirty(panelIdx int) {
	if a == nil || a.folderDirtyCh == nil {
		return
	}
	msg := FolderDirtyMsg{PanelIdx: panelIdx}
	select {
	case a.folderDirtyCh <- msg:
	default:
		ctrllog.FromContext(a.ctx).WithName("folderDirty").Info("dropping folder dirty event", "panel", panelIdx)
	}
}

func cloneConfig(cfg *appconfig.Config) *appconfig.Config {
	if cfg == nil {
		return appconfig.Default()
	}
	clone := *cfg
	clone.Resources.Favorites = append([]string(nil), cfg.Resources.Favorites...)
	return &clone
}

func (a *App) watchDiscovery() tea.Cmd {
	if a.discoveryCh == nil {
		return nil
	}
	ctx := a.ctx
	ch := a.discoveryCh
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case <-ch:
			return DiscoveryRefreshedMsg{}
		}
	}
}

func (a *App) handleDiscoveryRefresh() {
	if a.cl != nil {
		if infos, err := a.cl.GetResourceInfos(); err == nil {
			a.leftPanel.SetResourceCatalog(infos)
			a.rightPanel.SetResourceCatalog(infos)
		} else if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Discovery refresh failed: %v", err))
		}
	}
	mark := func(nav *navui.Navigator) {
		if nav == nil {
			return
		}
		nav.ForEach(func(f models.Folder) {
			switch f.(type) {
			case *models.ClusterResourcesFolder, *models.NamespacedResourcesFolder, *models.RootFolder, *models.ContextRootFolder:
				if refresher, ok := f.(interface{ Refresh() }); ok {
					refresher.Refresh()
				}
			}
		})
	}
	mark(a.leftNav)
	mark(a.rightNav)
}

func (a *App) ensurePanelConfig(panel *Panel) *appconfig.Config {
	if panel == a.leftPanel {
		if a.leftConfig == nil {
			a.leftConfig = cloneConfig(a.cfg)
		}
		return a.leftConfig
	}
	if a.rightConfig == nil {
		a.rightConfig = cloneConfig(a.cfg)
	}
	return a.rightConfig
}

// activePanelRef returns the currently focused panel.
func (a *App) activePanelRef() *Panel {
	if a.activePanel == 1 {
		return a.rightPanel
	}
	return a.leftPanel
}

// panelIndex returns 0 for the left panel and 1 for the right panel.
func (a *App) panelIndex(panel *Panel) int {
	if panel == nil {
		return -1
	}
	if panel == a.rightPanel {
		return 1
	}
	return 0
}

func applyResourceOptionsToFolder(folder models.Folder, show bool, order appconfig.ResourcesViewOrder, favorites []string) {
	if folder == nil {
		return
	}
	if configurable, ok := folder.(models.ResourceViewConfigurable); ok {
		configurable.ApplyResourceViewOptions(show, order, favorites)
	}
}

func (a *App) navigatorForPanel(panel *Panel) *navui.Navigator {
	if panel == a.rightPanel {
		return a.rightNav
	}
	return a.leftNav
}

func (a *App) panelByIndex(idx int) *Panel {
	if idx == 1 {
		return a.rightPanel
	}
	return a.leftPanel
}

func (a *App) setActivePanel(idx int, reason string) {
	if idx < 0 || idx > 1 {
		return
	}
	if a.activePanel == idx {
		return
	}
	a.activePanel = idx
	a.invalidateView(reason)
	a.invalidateFunctionBar("active panel change")
}

func (a *App) setPanelFolder(ctx context.Context, panelIdx int, folder models.Folder, hasBack bool) {
	panel := a.panelByIndex(panelIdx)
	if panel == nil {
		return
	}
	panel.SetFolder(ctx, folder, hasBack)
	panel.RefreshFolder(ctx)
	a.attachFolderDirtyListener(panelIdx, folder)
	a.invalidateView(fmt.Sprintf("panel %d folder assigned", panelIdx))
	a.invalidateFunctionBar(fmt.Sprintf("panel %d folder assigned", panelIdx))
}

func (a *App) showModal(key string) {
	if a.modalManager == nil {
		return
	}
	a.modalManager.Show(key)
	a.invalidateView("show modal " + key)
}

func (a *App) hideModal() {
	if a.modalManager == nil {
		return
	}
	a.modalManager.Hide()
	a.invalidateView("hide modal")
}

func (a *App) hideModalName(name string) {
	if a.modalManager == nil {
		return
	}
	a.modalManager.HideName(name)
	a.invalidateView("hide modal " + name)
}

func (a *App) attachFolderDirtyListener(panelIdx int, folder models.Folder) {
	var cancelPrev func()
	switch panelIdx {
	case 0:
		cancelPrev = a.leftFolderDirtyCancel
	case 1:
		cancelPrev = a.rightFolderDirtyCancel
	default:
		return
	}
	if cancelPrev != nil {
		cancelPrev()
	}
	assign := func(func()) {}
	switch panelIdx {
	case 0:
		assign = func(fn func()) { a.leftFolderDirtyCancel = fn }
	case 1:
		assign = func(fn func()) { a.rightFolderDirtyCancel = fn }
	}
	if folder == nil {
		assign(nil)
		return
	}
	obs, ok := folder.(models.DirtyObservable)
	if !ok {
		ctrllog.FromContext(a.ctx).WithName("folderDirty").Info("folder not dirty observable", "panel", panelIdx)
		assign(nil)
		return
	}
	cancel := obs.RegisterDirtyListener(func() {
		log := ctrllog.FromContext(a.ctx).WithName("folderDirty")
		log.Info("folder marked dirty", "panel", panelIdx)
		idx := panelIdx
		a.signalFolderDirty(idx)
	})
	assign(cancel)
}

func (a *App) panelIndexFor(panel *Panel) (int, bool) {
	if panel == nil {
		return -1, false
	}
	if panel == a.leftPanel {
		return 0, true
	}
	if panel == a.rightPanel {
		return 1, true
	}
	return -1, false
}

func (a *App) inlineTerminalHeight() int {
	if a == nil || a.terminal == nil {
		return 0
	}
	if a.showTerminal {
		return 0
	}
	return 2
}

func (a *App) panelAreaMetrics() (leftWidth int, rightWidth int, panelHeight int, headerOffset int) {
	reserved := 1 + a.inlineTerminalHeight()
	if a.toastActive {
		reserved++
	}
	panelHeight = a.height - reserved
	if panelHeight < 0 {
		panelHeight = 0
	}
	leftWidth, rightWidth = a.panelWidthsFor(a.width)
	headerOffset = 2
	return
}

func (a *App) resizePanelsForCurrentLayout() {
	if a == nil {
		return
	}
	leftWidth, rightWidth, panelHeight, _ := a.panelAreaMetrics()
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	log := ctrllog.FromContext(ctx).WithName("ui").WithName("panelResize")
	computeContentHeight := func(p *Panel, width int) int {
		if p == nil || width <= 0 {
			return 0
		}
		footerHeight := p.EstimatedFooterHeight(ctx, width)
		return max(1, panelHeight-footerHeight-2)
	}
	resize := func(p *Panel, width int) {
		if p == nil || width <= 0 {
			return
		}
		contentWidth := max(1, width-2)
		p.SetDimensions(ctx, contentWidth, computeContentHeight(p, width))
	}
	resize(a.leftPanel, leftWidth)
	resize(a.rightPanel, rightWidth)
	log.V(1).Info("panels resized",
		"leftWidth", leftWidth,
		"rightWidth", rightWidth,
		"panelHeight", panelHeight,
		"leftContentHeight", computeContentHeight(a.leftPanel, leftWidth),
		"rightContentHeight", computeContentHeight(a.rightPanel, rightWidth),
	)
}

func (a *App) panelWidthPercentFor(panelIdx int) int {
	if panelIdx == 1 {
		return clampPercent(a.rightPanelWidthPercent)
	}
	return clampPercent(a.leftPanelWidthPercent)
}

func (a *App) panelWidthsFor(total int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	leftPercent, rightPercent := normalizePanelWidthPercents(a.leftPanelWidthPercent, a.rightPanelWidthPercent)
	if leftPercent >= 100 {
		return total, 0
	}
	if rightPercent >= 100 {
		return 0, total
	}
	left := 0
	if leftPercent > 0 {
		left = (total*leftPercent + 50) / 100
		if left <= 0 {
			left = 1
		}
		if left >= total {
			left = total - 1
		}
	}
	right := total - left
	if rightPercent == 0 {
		right = 0
	} else if right <= 0 {
		right = 1
		if left > 0 {
			left = total - right
		}
	}
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	return left, total - left
}

func (a *App) setPanelWidthPercent(panelIdx int, percent int) {
	percent = clampPercent(percent)
	switch panelIdx {
	case 1:
		left, right := normalizePanelWidthPercents(100-percent, percent)
		a.leftPanelWidthPercent = left
		a.rightPanelWidthPercent = right
	default:
		left, right := normalizePanelWidthPercents(percent, 100-percent)
		a.leftPanelWidthPercent = left
		a.rightPanelWidthPercent = right
	}
	if a.leftConfig != nil {
		a.leftConfig.Panel.Width.LeftPercent = a.leftPanelWidthPercent
		a.leftConfig.Panel.Width.RightPercent = a.rightPanelWidthPercent
	}
	if a.rightConfig != nil {
		a.rightConfig.Panel.Width.LeftPercent = a.leftPanelWidthPercent
		a.rightConfig.Panel.Width.RightPercent = a.rightPanelWidthPercent
	}
	if a.leftPanelWidthPercent == 0 && a.rightPanelWidthPercent == 100 {
		a.setActivePanel(1, "panel width forced right")
	} else if a.leftPanelWidthPercent == 100 && a.rightPanelWidthPercent == 0 {
		a.setActivePanel(0, "panel width forced left")
	}
	a.resizePanelsForCurrentLayout()
	a.invalidateView("panel width percent")
}

var panelWidthCycle = panelWidthPercentOptions

func nextPanelWidthPercent(current int) int {
	if len(panelWidthCycle) == 0 {
		return current
	}
	for _, candidate := range panelWidthCycle {
		if candidate > current {
			return candidate
		}
	}
	return panelWidthCycle[len(panelWidthCycle)-1]
}

func (a *App) cyclePanelWidth(panelIdx int) {
	current := a.panelWidthPercentFor(panelIdx)
	next := nextPanelWidthPercent(current)
	if next == current {
		return
	}
	a.setPanelWidthPercent(panelIdx, next)
}

func normalizePanelWidthPercents(left, right int) (int, int) {
	left = clampPercent(left)
	right = clampPercent(right)
	switch {
	case left == 0 && right == 0:
		left, right = 50, 50
	case left == 0:
		left = clampPercent(100 - right)
		right = clampPercent(right)
	default:
		if left >= 100 {
			left, right = 100, 0
		} else {
			right = clampPercent(100 - left)
		}
	}
	if right >= 100 {
		left = 0
		right = 100
	}
	return left, right
}

func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func (a *App) syncPanelConfig(panel *Panel) {
	cfg := a.ensurePanelConfig(panel)
	showNonEmpty, order := panel.ResourceViewOptions()
	cfg.Resources.ShowNonEmptyOnly = showNonEmpty
	cfg.Resources.Order = appconfig.ResourcesViewOrder(order)
	columns := panel.ColumnsMode()
	cfg.Resources.Columns = columns
	cfg.Objects.Order = panel.ObjectOrder()
	cfg.Objects.Columns = columns
	cfg.Panel.Table.Mode = appconfig.TableMode(panel.TableMode())
}

func (a *App) applyResourceOptions(panel *Panel) {
	if panel == nil {
		return
	}
	show, orderStr := panel.ResourceViewOptions()
	order := appconfig.ResourcesViewOrder(orderStr)
	cfg := a.ensurePanelConfig(panel)
	var favorites []string
	if cfg != nil {
		favorites = cfg.Resources.Favorites
	}
	applyResourceOptionsToFolder(panel.Folder(), show, order, favorites)
	if nav := a.navigatorForPanel(panel); nav != nil {
		if cur := nav.Current(); cur != nil {
			applyResourceOptionsToFolder(cur, show, order, favorites)
		}
	}
}

func (a *App) aggregatedKubeConfig(current string) clientcmdapi.Config {
	contexts := make(map[string]*clientcmdapi.Context)
	if a.kubeMgr != nil {
		for _, ctx := range a.kubeMgr.GetContexts() {
			if ctx == nil {
				continue
			}
			contexts[ctx.Name] = &clientcmdapi.Context{
				Cluster:   ctx.Cluster,
				AuthInfo:  ctx.User,
				Namespace: ctx.Namespace,
			}
		}
	}
	return clientcmdapi.Config{
		CurrentContext: current,
		Contexts:       contexts,
	}
}

func (a *App) makeDeps(cl *kccluster.Cluster, cfg *appconfig.Config, key kccluster.Key) models.Deps {
	if cfg == nil {
		cfg = a.cfg
	}
	return models.Deps{
		Cl:               cl,
		Ctx:              a.ctx,
		CtxName:          key.ContextName,
		KubeConfig:       a.aggregatedKubeConfig(key.ContextName),
		AppConfig:        cfg,
		ClusterKey:       key,
		NamespaceFactory: a.namespaceClusterFactory(),
		PodFSFactory:     a.podfsFactory,
	}
}

func (a *App) currentClusterKey() kccluster.Key {
	if a.currentCtx == nil || a.currentCtx.Kubeconfig == nil {
		return kccluster.Key{}
	}
	return kccluster.Key{
		KubeconfigPath:       a.currentCtx.Kubeconfig.Path,
		ContextName:          a.currentCtx.Name,
		ImpersonateUser:      strings.TrimSpace(a.impersonateUser),
		ImpersonateUID:       strings.TrimSpace(a.impersonateUID),
		ImpersonateGroupsKey: a.impersonationGroupsKey(),
	}
}

func (a *App) namespaceClusterFactory() models.NamespaceClusterFactory {
	return func(ctx context.Context, key kccluster.Key, selectors map[schema.GroupVersionResource]crcache.ByObject) (*kccluster.Cluster, error) {
		if a.clPool == nil {
			return nil, fmt.Errorf("cluster pool unavailable")
		}
		if len(selectors) > 0 {
			return a.clPool.GetWithSelectors(ctx, key, selectors)
		}
		return a.clPool.Get(ctx, key)
	}
}

func (a *App) navigatorPath(nav *navui.Navigator) string {
	if nav == nil {
		return "/"
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	return nav.Path(ctx)
}

func (a *App) impersonationGroupsKey() string {
	if len(a.impersonateGroups) == 0 {
		return ""
	}
	groups := append([]string(nil), a.impersonateGroups...)
	for i := range groups {
		groups[i] = strings.TrimSpace(groups[i])
	}
	sort.Strings(groups)
	return strings.Join(groups, ",")
}

func (a *App) impersonationSpec() termctx.Impersonation {
	imp := termctx.Impersonation{
		User: strings.TrimSpace(a.impersonateUser),
		UID:  strings.TrimSpace(a.impersonateUID),
	}
	for _, g := range a.impersonateGroups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		imp.Groups = append(imp.Groups, g)
	}
	return imp
}

func (a *App) impersonationTemplate() *clientcmdapi.Config {
	if a.currentCtx == nil || a.currentCtx.Kubeconfig == nil || a.currentCtx.Kubeconfig.Config == nil {
		return nil
	}
	imp := a.impersonationSpec()
	if strings.TrimSpace(imp.User) == "" && strings.TrimSpace(imp.UID) == "" && len(imp.Groups) == 0 {
		return a.currentCtx.Kubeconfig.Config
	}
	cfg := a.currentCtx.Kubeconfig.Config.DeepCopy()
	ctx := cfg.Contexts[a.currentCtx.Name]
	if ctx == nil {
		return cfg
	}
	userName := strings.TrimSpace(ctx.AuthInfo)
	if userName == "" {
		return cfg
	}
	if cfg.AuthInfos == nil {
		cfg.AuthInfos = make(map[string]*clientcmdapi.AuthInfo)
	}
	auth := cfg.AuthInfos[userName]
	if auth == nil {
		auth = &clientcmdapi.AuthInfo{}
		cfg.AuthInfos[userName] = auth
	}
	auth.Impersonate = imp.User
	auth.ImpersonateUID = imp.UID
	auth.ImpersonateGroups = append([]string(nil), imp.Groups...)
	return cfg
}

func (a *App) navigatorNamespace(nav *navui.Navigator) string {
	if nav == nil {
		return ""
	}
	ns := ""
	type namespaceProvider interface {
		Namespace() string
	}
	nav.ForEach(func(folder models.Folder) {
		if provider, ok := folder.(namespaceProvider); ok {
			if name := strings.TrimSpace(provider.Namespace()); name != "" {
				ns = name
			}
		}
	})
	return ns
}

func (a *App) syncTerminalNamespaceForNavigator(nav *navui.Navigator) {
	if a.cfg == nil || !a.cfg.Terminal.Follow {
		return
	}
	ns := strings.TrimSpace(a.navigatorNamespace(nav))
	a.updateTermContextNamespace(ns)
}

func (a *App) makeEnterContextFunc(cfg *appconfig.Config) func(string, []string) (models.Folder, error) {
	return func(name string, basePath []string) (models.Folder, error) {
		if a.kubeMgr == nil {
			return nil, fmt.Errorf("no kubeconfig manager available")
		}
		target := a.kubeMgr.GetContextByName(name)
		if target == nil {
			return nil, fmt.Errorf("context %q not found", name)
		}
		if target.Kubeconfig == nil {
			return nil, fmt.Errorf("context %q has no kubeconfig", name)
		}
		key := kccluster.Key{
			KubeconfigPath:       target.Kubeconfig.Path,
			ContextName:          target.Name,
			ImpersonateUser:      strings.TrimSpace(a.impersonateUser),
			ImpersonateUID:       strings.TrimSpace(a.impersonateUID),
			ImpersonateGroupsKey: a.impersonationGroupsKey(),
		}
		cl, err := a.clPool.Get(a.ctx, key)
		if err != nil {
			return nil, err
		}
		deps := a.makeDeps(cl, cfg, key)
		return models.NewContextRootFolder(deps, basePath), nil
	}
}

// setupModals sets up the modal dialogs

func (a *App) setupModals() {
	// Resources options modal (content set dynamically on open)
	viewOpts := NewViewOptionsModel(ViewOptionsConfig{
		PanelIndex:        0,
		PanelModes:        []PanelViewMode{PanelModeList},
		ActivePanelMode:   PanelModeList,
		PanelWidthPercent: a.leftPanelWidthPercent,
		TableMode:         "scroll",
	})
	viewModal := NewModal("View Options", viewOpts)
	a.modalManager.Register("view_options", viewModal)

	// Namespace creation modal (configured on open)
	nsModel := NewNamespaceCreateModel()
	nsModal := NewModal("Create Namespace", nsModel)
	nsModal.SetCloseOnSingleEsc(true)
	a.modalManager.Register("namespace_create", nsModal)
	a.namespaceInput = nsModel

	// Delete confirmation modal
	delModel := NewDeleteConfirmModel()
	delModal := NewModal("Confirm Delete", delModel)
	delModal.SetCloseOnSingleEsc(true)
	a.modalManager.Register("delete_confirm", delModal)
	a.deleteConfirm = delModel

	// Copy-to-local modal
	copyModel := NewCopyToLocalModel()
	copyModal := NewModal("Copy to Local File", copyModel)
	copyModal.SetCloseOnSingleEsc(true)
	a.modalManager.Register("copy_local", copyModal)
	a.copyInput = copyModel

	for idx := 0; idx < 2; idx++ {
		modeModel := NewPanelModeModel(idx, []PanelViewMode{PanelModeList}, PanelModeList)
		modeModal := NewModal("Panel Mode", modeModel)
		modeModal.SetCloseOnSingleEsc(true)
		a.modalManager.Register(panelModeModalKey(idx), modeModal)
	}

	helpContent := helpMarkdownContent(strings.TrimSpace(readmedoc.README))
	helpViewer := NewMarkdownHelpViewer(helpContent)
	helpModal := NewModal("Kubernetes Commander Help", helpViewer)
	helpModal.SetCloseOnSingleEsc(true)
	a.modalManager.Register("help", helpModal)
	a.helpViewer = helpViewer
}

func helpMarkdownContent(raw string) string {
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.Contains(line, "docs/screenshot.png") {
			continue
		}
		if strings.HasPrefix(trim, "![") && strings.Contains(trim, "](") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func (a *App) setupPanelInputs() {
	envSupplier := func() PanelEnvironment { return a.panelEnvironment() }
	registerModes := func(panel *Panel, name string) {
		if panel == nil {
			return
		}
		panel.RegisterMode(PanelModeDescribe, func(p *Panel) PanelWidget {
			deps := p.manifestWidgetDeps()
			deps.Describe = a.describeFunc()
			return describewidget.New(deps)
		})
		panel.RegisterMode(PanelModeManifest, func(p *Panel) PanelWidget {
			return manifestwidget.New(p.manifestWidgetDeps())
		})
	}
	if a.leftPanel != nil {
		a.leftPanel.SetEnvironmentSupplier(envSupplier)
		a.leftPanel.SetActionHandlers(a.panelActionHandlers())
		registerModes(a.leftPanel, "Left panel")
	}
	if a.rightPanel != nil {
		a.rightPanel.SetEnvironmentSupplier(envSupplier)
		a.rightPanel.SetActionHandlers(a.panelActionHandlers())
		registerModes(a.rightPanel, "Right panel")
	}
}

func (a *App) describeFunc() panelcontent.DescribeFunc {
	return func(ctx context.Context, target describe.Target) (describe.Result, error) {
		renderer, err := a.newDescribeRenderer()
		if err != nil {
			return describe.Result{}, err
		}
		return renderer.Describe(target)
	}
}

func (a *App) newDescribeRenderer() (*describe.Renderer, error) {
	if a == nil || a.cl == nil {
		return nil, fmt.Errorf("cluster client unavailable")
	}
	cfg := rest.CopyConfig(a.cl.GetConfig())
	mapper := a.cl.RESTMapper()
	disco := a.cl.DiscoveryClient()
	var loader clientcmd.ClientConfig
	if a.currentCtx != nil && a.currentCtx.Kubeconfig != nil && a.currentCtx.Kubeconfig.Config != nil {
		overrides := &clientcmd.ConfigOverrides{CurrentContext: a.currentCtx.Name}
		loader = clientcmd.NewDefaultClientConfig(*a.currentCtx.Kubeconfig.Config, overrides)
	}
	return describe.NewRenderer(cfg, mapper, disco, loader)
}

func (a *App) panelActionHandlers() PanelActionHandlers {
	handlers := PanelActionHandlers{
		PanelActionHelp: func(*Panel) tea.Cmd {
			return a.showHelp()
		},
		PanelActionOptions: func(p *Panel) tea.Cmd {
			return a.showViewOptionsModalForPanel(p)
		},
		PanelActionView: func(p *Panel) tea.Cmd {
			return a.openViewerForPanel(p)
		},
		PanelActionEdit: func(p *Panel) tea.Cmd {
			return a.editSelectionForPanel(p)
		},
		PanelActionCopy: func(p *Panel) tea.Cmd {
			return a.copyItemForPanel(p)
		},
		PanelActionCreateNamespace: func(p *Panel) tea.Cmd {
			return a.createNamespaceForPanel(p)
		},
		PanelActionDelete: func(p *Panel) tea.Cmd {
			return a.deleteResourceForPanel(p)
		},
		PanelActionMenu: func(p *Panel) tea.Cmd {
			return a.showContextMenuForPanel(p)
		},
	}
	// Help (F1) is intentionally omitted until implemented.
	return handlers
}

func (a *App) panelEnvironment() PanelEnvironment {
	env := PanelEnvironment{}
	if a.currentCtx != nil {
		env.AllowCreateNamespaces = true
		if a.currentCtx.Kubeconfig != nil {
			env.AllowEditObjects = true
		}
	}
	if a.cl != nil {
		env.AllowDeleteObjects = true
	}
	return env
}

func (a *App) capabilitiesForPanel(panel *Panel) PanelCapabilities {
	if panel == nil {
		return PanelCapabilities{}
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	caps := panel.Capabilities(ctx)
	if caps.HasContextMenu {
		caps.HasContextMenu = a.hasApplicableCommands(panel)
	}
	return caps
}

func (a *App) updatePanelsWithMsg(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	if a.leftPanel != nil {
		model, cmd := a.leftPanel.Update(msg)
		if model != nil {
			a.leftPanel = model.(*Panel)
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if a.rightPanel != nil {
		model, cmd := a.rightPanel.Update(msg)
		if model != nil {
			a.rightPanel = model.(*Panel)
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func (a *App) cyclePanelMode(idx int) tea.Cmd {
	panel := a.panelByIndex(idx)
	if panel == nil {
		return nil
	}
	next := NextPanelMode(panel.Mode())
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	return panel.SetMode(ctx, next)
}

func (a *App) cyclePanelModeIfVisible(idx int) tea.Cmd {
	leftWidth, rightWidth, _, _ := a.panelAreaMetrics()
	// Only cycle when both panels are visible to avoid acting on hidden panes.
	if leftWidth <= 0 || rightWidth <= 0 {
		return nil
	}
	switch idx {
	case 0:
		if leftWidth <= 0 {
			return nil
		}
	case 1:
		if rightWidth <= 0 {
			return nil
		}
	default:
		return nil
	}
	return a.cyclePanelMode(idx)
}

func panelModeModalKey(idx int) string {
	if idx == 0 {
		return "panel_mode_left"
	}
	return "panel_mode_right"
}

func (a *App) showPanelModeModal(panelIdx int) tea.Cmd {
	panel := a.panelByIndex(panelIdx)
	if panel == nil {
		return nil
	}
	modes := panel.AvailableModes()
	if len(modes) == 0 {
		modes = []PanelViewMode{PanelModeList}
	}
	model := NewPanelModeModel(panelIdx, modes, panel.Mode())
	key := panelModeModalKey(panelIdx)
	modal := a.modalManager.modals[key]
	if modal == nil {
		modal = NewModal("Panel Mode", model)
		modal.SetCloseOnSingleEsc(true)
		a.modalManager.Register(key, modal)
	} else {
		modal.SetContent(model)
	}
	leftPanelWidth, rightPanelWidth, panelHeight, headerOffset := a.panelAreaMetrics()
	panelWidth := leftPanelWidth
	panelOffset := 0
	if panelIdx == 1 {
		panelWidth = rightPanelWidth
		panelOffset = leftPanelWidth
	}
	if panelWidth <= 0 {
		panelWidth = max(20, max(a.width/2, a.width))
		panelOffset = 0
	}
	width := panelWidth / 2
	if width < 24 {
		width = 24
	}
	width = min(width, panelWidth-2)
	width = min(width, a.width-4)
	maxContentHeight := max(len(modes)+4, 6)
	targetHeight := panelHeight / 2
	if targetHeight < 6 {
		targetHeight = 6
	}
	height := min(maxContentHeight, targetHeight)
	if height < 6 {
		height = 6
	}
	model.SetDimensions(max(1, width-4), max(1, height-3))
	modal.SetDimensions(a.width, a.height)
	bg, _ := a.renderMainView()
	modal.SetWindowed(width, height, bg)
	offsetX := panelOffset + max(0, (panelWidth-width)/2)
	offsetY := headerOffset
	modal.SetWindowOffset(offsetX, offsetY)
	modal.SetOnClose(func() tea.Cmd { return nil })
	a.showModal(key)
	return nil
}

// Message handlers for function keys
// showViewOptionsModal opens the appropriate View Options dialog (Resources or Objects)
// depending on the active view context.
func (a *App) showViewOptionsModal() tea.Cmd {
	return a.showViewOptionsModalForPanel(a.activePanelRef())
}

func (a *App) showViewOptionsModalForPanel(panel *Panel) tea.Cmd {
	if panel == nil {
		panel = a.activePanelRef()
	}
	if panel == nil {
		return nil
	}

	panelIdx, ok := a.panelIndexFor(panel)
	if !ok {
		panelIdx = a.activePanel
	}

	// Determine folder context for contextual options.
	var curFolder models.Folder
	if nav := a.navigatorForPanel(panel); nav != nil {
		curFolder = nav.Current()
	}
	if curFolder == nil {
		curFolder = panel.Folder()
	}

	showInclude := false
	showOrder := false
	resourceSection := false
	commandSection := panel.Mode() == PanelModeCommand
	if curFolder != nil {
		switch curFolder.(type) {
		case *models.RootFolder,
			*models.ClusterResourcesFolder,
			*models.NamespacedResourcesFolder,
			*models.ContextRootFolder,
			*models.ResourcesFolder:
			showInclude = true
			showOrder = true
			resourceSection = true
		}
	}
	if commandSection {
		resourceSection = false
	}

	objectSection := false
	if curFolder != nil {
		if _, ok := curFolder.(interface {
			ObjectListMeta() (schema.GroupVersionResource, string, bool)
		}); ok {
			objectSection = true
		}
	}
	if panel.Mode() != PanelModeList {
		objectSection = false
	}
	if commandSection {
		objectSection = false
	}

	// Panel-derived defaults.
	showNonEmpty, order := panel.ResourceViewOptions()
	tableMode := panel.TableMode()
	columns := panel.ColumnsMode()
	objOrder := panel.ObjectOrder()

	var resConfig *ViewOptionsResourcesConfig
	if resourceSection {
		resConfig = &ViewOptionsResourcesConfig{
			ShowInclude:   showInclude,
			IncludeEmpty:  !showNonEmpty,
			ShowOrder:     showOrder,
			Order:         order,
			ShowTableMode: true,
		}
	}

	var objConfig *ViewOptionsObjectsConfig
	if objectSection {
		objConfig = &ViewOptionsObjectsConfig{
			ShowTableMode: true,
			Columns:       columns,
			Order:         objOrder,
		}
	}

	var cmdConfig *ViewOptionsCommandConfig
	if commandSection {
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		defer cancel()
		cmdConfig = &ViewOptionsCommandConfig{
			WatchInterval: panel.CommandWatchInterval(ctx),
		}
	}

	content := NewViewOptionsModel(ViewOptionsConfig{
		PanelIndex:        panelIdx,
		PanelModes:        panel.AvailableModes(),
		ActivePanelMode:   panel.Mode(),
		PanelWidthPercent: a.panelWidthPercentFor(panelIdx),
		TableMode:         tableMode,
		Resources:         resConfig,
		Objects:           objConfig,
		Command:           cmdConfig,
	})

	modal := a.modalManager.modals["view_options"]
	if modal == nil {
		modal = NewModal("View Options", content)
		a.modalManager.Register("view_options", modal)
	} else {
		modal.SetContent(content)
		modal.title = "View Options"
	}

	a.layoutViewOptionsModal(modal, content, panelIdx, "")
	modal.SetOnClose(func() tea.Cmd { return nil })
	a.showModal("view_options")
	return nil
}

func (a *App) showViewerOptionsModal(target viewerOptionsTarget) tea.Cmd {
	if target == nil {
		return nil
	}
	themes := viewpkg.AvailableThemes()
	currentTheme := target.Theme()
	if currentTheme == "" {
		currentTheme = a.viewerTheme()
	}
	wrapMode := appconfig.ViewerModeScroll
	if target.WrapMode() {
		wrapMode = appconfig.ViewerModeWrap
	}
	content := NewViewOptionsModel(ViewOptionsConfig{
		PanelIndex:       -1,
		SkipPanelSection: true,
		Viewer: &ViewOptionsViewerConfig{
			ThemeNames: themes,
			Theme:      currentTheme,
			WrapMode:   wrapMode,
		},
	})
	modal := a.modalManager.modals["viewer_options"]
	if modal == nil {
		modal = NewModal("Viewer Options", content)
		modal.SetCloseOnSingleEsc(true)
		a.modalManager.Register("viewer_options", modal)
	} else {
		modal.SetContent(content)
		modal.title = "Viewer Options"
	}
	modal.SetOnClose(func() tea.Cmd {
		a.viewerOptionsTarget = nil
		return nil
	})
	base := ""
	if active := a.modalManager.GetActiveModal(); active != nil {
		if view := active.View(); viewString(view) != "" {
			base = viewString(view)
		}
	}
	a.layoutViewOptionsModal(modal, content, -1, base)
	a.viewerOptionsTarget = target
	a.showModal("viewer_options")
	return nil
}

func (a *App) applyViewerOptions(msg ViewOptionsCommittedMsg) tea.Cmd {
	target := a.viewerOptionsTarget
	if msg.Viewer == nil {
		if msg.Close {
			a.hideModal()
		}
		return nil
	}
	if target == nil {
		if msg.Close {
			a.hideModal()
		}
		return nil
	}
	apply := msg.Accept || msg.SaveDefault
	if apply {
		theme := msg.Viewer.Theme
		if theme == "" {
			theme = target.Theme()
		}
		target.SetTheme(theme)
		wrap := strings.EqualFold(msg.Viewer.WrapMode, appconfig.ViewerModeWrap)
		target.SetWrapMode(wrap)
		if a.cfg == nil {
			a.cfg = appconfig.Default()
		}
		a.cfg.Viewer.Theme = theme
		if wrap {
			a.cfg.Viewer.Mode = appconfig.ViewerModeWrap
		} else {
			a.cfg.Viewer.Mode = appconfig.ViewerModeScroll
		}
		if msg.SaveDefault {
			_ = appconfig.Save(a.cfg)
		}
	}
	if msg.Close {
		a.hideModal()
	}
	a.viewerOptionsTarget = nil
	return nil
}

func (a *App) layoutViewOptionsModal(modal *Modal, content *ViewOptionsModel, panelIdx int, background string) {
	leftPanelWidth, rightPanelWidth, _, _ := a.panelAreaMetrics()
	panelWidth := leftPanelWidth
	if panelIdx == 1 {
		panelWidth = rightPanelWidth
	}
	if panelWidth <= 0 {
		panelWidth = max(24, max(a.width/2, a.width))
	}
	fallbackWidth := panelWidth / 2
	if fallbackWidth < 36 {
		fallbackWidth = 36
	}
	fallbackHeight := content.ContentHeight() + 2
	if fallbackHeight < 6 {
		fallbackHeight = 6
	}
	a.configureModalWindow(modal, content, panelIdx, background, fallbackWidth, fallbackHeight)
}

func (a *App) modalPanelMetrics(panelIdx int) (panelWidth, panelHeight, headerOffset, panelOffset int, centered bool) {
	leftWidth, rightWidth, panelHeight, headerOffset := a.panelAreaMetrics()
	panelOffset = 0
	panelWidth = leftWidth
	centered = panelIdx < 0
	if centered {
		panelWidth = a.width
		panelOffset = 0
	} else if panelIdx == 1 {
		panelWidth = rightWidth
		panelOffset = leftWidth
	}
	if panelWidth <= 0 {
		panelWidth = max(24, max(a.width/2, a.width))
		panelOffset = 0
	}
	return panelWidth, panelHeight, headerOffset, panelOffset, centered
}

func (a *App) configureModalWindow(modal *Modal, content interface{}, panelIdx int, background string, fallbackWinW, fallbackWinH int) {
	panelWidth, panelHeight, headerOffset, panelOffset, centered := a.modalPanelMetrics(panelIdx)
	maxFrameWidth := min(panelWidth-2, a.width-4)
	if maxFrameWidth < 6 {
		maxFrameWidth = 6
	}
	maxFrameHeight := min(panelHeight-2, a.height-4)
	if maxFrameHeight < 6 {
		maxFrameHeight = 6
	}
	maxContentWidth := max(1, maxFrameWidth-2)
	maxContentHeight := max(1, maxFrameHeight-2)

	innerW := max(1, fallbackWinW-2)
	innerH := max(1, fallbackWinH-2)
	if fallbackWinW <= 0 || innerW <= 0 {
		innerW = maxContentWidth
	}
	if fallbackWinH <= 0 || innerH <= 0 {
		innerH = maxContentHeight
	}
	if sizer, ok := content.(ModalSizer); ok {
		if prefW, prefH := sizer.PreferredSize(maxContentWidth, maxContentHeight); prefW > 0 || prefH > 0 {
			if prefW > 0 {
				innerW = min(prefW, maxContentWidth)
			}
			if prefH > 0 {
				innerH = min(prefH, maxContentHeight)
			}
		}
	}
	if innerW > maxContentWidth {
		innerW = maxContentWidth
	}
	if innerH > maxContentHeight {
		innerH = maxContentHeight
	}
	winW := innerW + 2
	winH := innerH + 2
	if winW > maxFrameWidth {
		winW = maxFrameWidth
		innerW = max(1, winW-2)
	}
	if winH > maxFrameHeight {
		winH = maxFrameHeight
		innerH = max(1, winH-2)
	}
	if setter, ok := content.(interface{ SetDimensions(int, int) }); ok {
		setter.SetDimensions(innerW, innerH)
	}
	bg := background
	if bg == "" {
		bg, _ = a.renderMainView()
	}
	modal.SetDimensions(a.width, a.height)
	modal.SetWindowed(winW, winH, bg)

	offsetX := panelOffset + max(0, (panelWidth-winW)/2)
	panelUsable := max(0, panelHeight-headerOffset-1)
	offsetY := headerOffset
	if panelUsable > winH {
		offsetY += (panelUsable - winH) / 2
	}
	if offsetY < headerOffset {
		offsetY = headerOffset
	}
	maxOffsetY := max(0, (a.height-1)-winH)
	if offsetY > maxOffsetY {
		offsetY = maxOffsetY
	}
	if centered {
		offsetX = max(0, (a.width-winW)/2)
		offsetY = headerOffset + max(0, (panelHeight-winH)/2)
		if offsetY > maxOffsetY {
			offsetY = maxOffsetY
		}
	}
	modal.SetWindowOffset(offsetX, offsetY)
}

func (a *App) viewResource() tea.Cmd {
	// TODO: Implement resource viewer
	return nil
}

func (a *App) editResource() tea.Cmd {
	// TODO: Implement resource editor
	return nil
}

func (a *App) createNamespace() tea.Cmd {
	return a.createNamespaceForPanel(a.activePanelRef())
}

func (a *App) createNamespaceForPanel(panel *Panel) tea.Cmd {
	if a.currentCtx == nil {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("No active context for namespace creation"))
		}
		return nil
	}
	if panel == nil {
		panel = a.activePanelRef()
	}
	if panel == nil {
		return nil
	}
	if panel.GetCurrentPath() != "/namespaces" {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Create namespace is only available at /namespaces"))
		}
		return nil
	}
	a.namespaceCreatePanel = a.panelIndex(panel)
	modal := a.modalManager.modals["namespace_create"]
	if modal == nil {
		a.namespaceCreatePanel = -1
		return nil
	}
	if a.namespaceInput == nil {
		if model, ok := modal.content.(*NamespaceCreateModel); ok {
			a.namespaceInput = model
		} else {
			return nil
		}
	}
	a.namespaceInput.Reset()
	modal.SetContent(a.namespaceInput)
	a.configureModalWindow(modal, a.namespaceInput, a.namespaceCreatePanel, "", 30, 6)
	modal.SetOnClose(func() tea.Cmd {
		a.namespaceInput.Reset()
		return nil
	})
	a.showModal("namespace_create")
	return nil
}

func (a *App) deleteResource() tea.Cmd {
	return a.deleteResourceForPanel(a.activePanelRef())
}

func (a *App) deleteResourceForPanel(panel *Panel) tea.Cmd {
	panelIdx := a.activePanel
	if panel == nil {
		panel = a.activePanelRef()
	} else {
		if idx := a.panelIndex(panel); idx >= 0 {
			panelIdx = idx
		}
	}
	if panel == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	item, ok := panel.SelectedNavItem(ctx)
	cancel()
	if !ok || item == nil {
		return nil
	}
	obj, ok := item.(models.ObjectItem)
	if !ok {
		return nil
	}
	gvr := obj.GVR()
	name := obj.Name()
	namespace := obj.Namespace()
	target := deleteTarget{
		panelIdx:  panelIdx,
		gvr:       gvr,
		namespace: namespace,
		name:      name,
	}
	a.pendingDelete = &target

	modal := a.modalManager.modals["delete_confirm"]
	if modal == nil {
		return nil
	}
	if a.deleteConfirm == nil {
		if model, ok := modal.content.(*DeleteConfirmModel); ok {
			a.deleteConfirm = model
		} else {
			return nil
		}
	}
	resourceLabel := kubectlResourceRef(gvr, name)
	a.deleteConfirm.Configure(resourceLabel, namespace)
	a.deleteConfirm.SetDimensions(max(20, a.width-4), max(5, a.height-6))
	modal.SetContent(a.deleteConfirm)
	modal.SetDimensions(a.width, a.height)
	bg, _ := a.renderMainView()
	winW := min(max(50, a.width/2), a.width-4)
	winH := min(8, a.height-4)
	if winW < 40 {
		winW = 40
	}
	if winH < 6 {
		winH = 6
	}
	modal.SetWindowed(winW, winH, bg)
	modal.SetOnClose(func() tea.Cmd {
		a.pendingDelete = nil
		return nil
	})
	a.showModal("delete_confirm")
	return nil
}

func (a *App) showContextMenu() tea.Cmd {
	return a.showContextMenuForPanel(a.activePanelRef())
}

func (a *App) createNamespaceWithName(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return a.withBusy("Create namespace", 300*time.Millisecond, func() tea.Msg {
		if a.cl == nil {
			return namespaceCreatedMsg{name: name, err: fmt.Errorf("cluster not ready")}
		}
		ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
		defer cancel()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := a.cl.GetClient().Create(ctx, ns); err != nil {
			return namespaceCreatedMsg{name: name, err: err}
		}
		return namespaceCreatedMsg{name: name}
	})
}

func (a *App) performDelete(target deleteTarget) tea.Cmd {
	return a.withBusy("Delete", 300*time.Millisecond, func() tea.Msg {
		if a.cl == nil {
			return resourceDeletedMsg{target: target, err: fmt.Errorf("cluster not ready")}
		}
		ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
		defer cancel()
		kind, err := a.cl.RESTMapper().KindFor(target.gvr)
		if err != nil {
			return resourceDeletedMsg{target: target, err: err}
		}
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(kind)
		obj.SetName(target.name)
		if target.namespace != "" {
			obj.SetNamespace(target.namespace)
		}
		if err := a.cl.GetClient().Delete(ctx, obj); err != nil {
			return resourceDeletedMsg{target: target, err: err}
		}
		return resourceDeletedMsg{target: target}
	})
}

// Function key action methods
func (a *App) showHelp() tea.Cmd {
	if a.helpViewer == nil || a.modalManager == nil {
		return nil
	}
	modal := a.modalManager.modals["help"]
	if modal == nil {
		return nil
	}
	a.helpViewer.ScrollTop()
	modal.SetDimensions(a.width, a.height)
	bg, _ := a.renderMainView()
	winW := a.width - 8
	if winW < 40 {
		winW = max(20, a.width-2)
	}
	if winW > a.width {
		winW = a.width
	}
	winH := a.height - 4
	if winH < 18 {
		winH = max(10, a.height-2)
	}
	if winH > a.height {
		winH = a.height
	}
	if winW <= 0 {
		winW = max(1, a.width)
	}
	if winH <= 0 {
		winH = max(1, a.height-1)
	}
	modal.SetWindowed(winW, winH, bg)
	offsetX := max(0, (a.width-winW)/2)
	offsetY := max(0, (a.height-winH)/2)
	modal.SetWindowOffset(offsetX, offsetY)
	modal.SetOnClose(func() tea.Cmd {
		a.helpViewer.ScrollTop()
		return nil
	})
	a.showModal("help")
	return nil
}

func (a *App) viewItem() tea.Cmd {
	return a.openViewerForPanel(a.activePanelRef())
}

func (a *App) editItem() tea.Cmd {
	return a.editSelectionForPanel(a.activePanelRef())
}

// openViewerForSelection opens the focused item's viewer when available.
func (a *App) openViewerForSelection() tea.Cmd {
	return a.openViewerForPanel(a.activePanelRef())
}

func (a *App) openViewerForPanel(panel *Panel) tea.Cmd {
	if panel == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	item, ok := panel.SelectedNavItem(ctx)
	if !ok || item == nil {
		return nil
	}
	if _, isBack := item.(models.Back); isBack {
		return nil
	}
	if logs, ok := item.(models.LogsProvider); ok {
		return a.openLogsViewer(item, logs.LogsSpec())
	}
	viewable, ok := item.(models.Viewable)
	if !ok {
		type vc interface {
			ViewContent() (string, string, string, string, string, error)
		}
		if alt, okAlt := item.(vc); okAlt {
			viewable = alt
		} else {
			return nil
		}
	}
	title, body, lang, mime, filename, err := viewable.ViewContent()
	if err != nil {
		if errors.Is(err, models.ErrNoViewContent) {
			return nil
		}
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("View failed: %v", err))
		}
		return nil
	}
	if filename == "" {
		filename = title
	}
	theme := a.viewerTheme()
	var onEdit func() tea.Cmd
	if _, ok := item.(models.ObjectItem); ok {
		onEdit = func() tea.Cmd { return a.editSelectionForPanel(panel) }
	}
	view := NewTextViewer(title, body, lang, mime, filename, theme, onEdit, nil, func() tea.Cmd {
		a.hideModal()
		return nil
	})
	view.SetWrapMode(a.cfg != nil && strings.EqualFold(a.cfg.Viewer.Mode, appconfig.ViewerModeWrap))
	view.SetOnOptions(func() tea.Cmd { return a.showViewerOptionsModal(view) })
	modalTitle := a.modalTitleFromItem(item, title)
	modal := NewModal(modalTitle, view)
	modal.SetMode(ModalModeFullscreen)
	modal.SetDimensions(a.width, a.height)
	modal.SetCloseOnSingleEsc(false)
	a.modalManager.Register("yaml_viewer", modal)
	a.showModal("yaml_viewer")
	return nil
}

func (a *App) openLogsViewer(item models.Item, spec models.LogsSpec) tea.Cmd {
	if a.cl == nil {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Logs unavailable: no active cluster"))
		}
		return nil
	}
	cfg := rest.CopyConfig(a.cl.GetConfig())
	clientset, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kc-log-viewer"))
	if err != nil {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Logs client: %v", err))
		}
		return nil
	}
	opts := &corev1.PodLogOptions{
		Container: spec.Container,
		Follow:    true,
	}
	if spec.Follow {
		opts.Follow = true
	}
	tail := spec.TailLines
	if tail <= 0 {
		tail = models.DefaultLogsTailLines
	}
	opts.TailLines = &tail
	ctx, cancel := context.WithCancel(a.ctx)
	req := clientset.CoreV1().Pods(spec.Namespace).GetLogs(spec.Pod, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		cancel()
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Logs stream failed: %v", err))
		}
		return nil
	}
	title := fmt.Sprintf("logs:%s/%s/%s", spec.Namespace, spec.Pod, spec.Container)
	logsViewer := NewLogsViewer(title, stream, cancel, a.viewerTheme())
	logsViewer.SetWrapMode(a.cfg != nil && strings.EqualFold(a.cfg.Viewer.Mode, appconfig.ViewerModeWrap))
	logsViewer.SetOnOptions(func() tea.Cmd { return a.showViewerOptionsModal(logsViewer) })
	logsViewer.SetOnClose(func() tea.Cmd {
		a.hideModal()
		return nil
	})
	modalTitle := a.modalTitleFromItem(item, title)
	modal := NewModal(modalTitle, logsViewer)
	modal.SetMode(ModalModeFullscreen)
	modal.SetDimensions(a.width, a.height)
	modal.SetCloseOnSingleEsc(false)
	modal.SetOnClose(func() tea.Cmd {
		logsViewer.Close()
		return nil
	})
	a.modalManager.Register("logs_viewer", modal)
	a.showModal("logs_viewer")
	return logsViewer.Init()
}

func (a *App) modalTitleFromItem(item models.Item, fallback string) string {
	if item != nil {
		if pa, ok := item.(interface{ Path() []string }); ok {
			if segs := pa.Path(); len(segs) > 0 {
				return "/" + strings.Join(segs, "/")
			}
		}
	}
	if fallback != "" {
		return "/" + fallback
	}
	return "/"
}

func (a *App) viewerTheme() string {
	theme := "dracula"
	if a.cfg != nil && a.cfg.Viewer.Theme != "" {
		theme = a.cfg.Viewer.Theme
	}
	return theme
}

// editSelection triggers kubectl edit for the selected object.
func (a *App) editSelection() tea.Cmd {
	return a.editSelectionForPanel(a.activePanelRef())
}

func (a *App) editSelectionForPanel(panel *Panel) tea.Cmd {
	panelIdx := a.activePanel
	if panel == nil {
		panel = a.activePanelRef()
	} else {
		if idx := a.panelIndex(panel); idx >= 0 {
			panelIdx = idx
		}
	}
	if panel == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	item, ok := panel.SelectedNavItem(ctx)
	cancel()
	if !ok || item == nil {
		return nil
	}
	obj, ok := item.(models.ObjectItem)
	if !ok {
		return nil
	}

	path := panel.GetCurrentPath()
	return a.runKubectlEdit(panelIdx, path, obj)
}

func (a *App) runKubectlEdit(panelIdx int, panelPath string, obj models.ObjectItem) tea.Cmd {
	if a.currentCtx == nil || a.currentCtx.Kubeconfig == nil {
		if a.toastLogger != nil {
			return a.toastLogger.Errorf("kubectl edit: no active kubeconfig")
		}
		return a.ShowToast("kubectl edit unavailable: no active kubeconfig", 5*time.Second)
	}

	log := ctrllog.FromContext(a.ctx).WithName("kubectl_edit")

	kubeconfigPath := a.currentCtx.Kubeconfig.Path
	tempConfigPath := ""
	if kubeconfigPath == "" {
		if a.currentCtx.Kubeconfig.Config == nil {
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: kubeconfig has no backing file")
			}
			return a.ShowToast("kubectl edit failed: kubeconfig missing backing file", 5*time.Second)
		}
		tmpFile, err := os.CreateTemp("", "kc-kubeconfig-*.yaml")
		if err != nil {
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: temp kubeconfig: %v", err)
			}
			return a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", err), 5*time.Second)
		}
		if err := tmpFile.Close(); err != nil {
			_ = os.Remove(tmpFile.Name())
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: temp kubeconfig close: %v", err)
			}
			return a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", err), 5*time.Second)
		}
		if err := clientcmd.WriteToFile(*a.currentCtx.Kubeconfig.Config, tmpFile.Name()); err != nil {
			_ = os.Remove(tmpFile.Name())
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: write kubeconfig: %v", err)
			}
			return a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", err), 5*time.Second)
		}
		kubeconfigPath = tmpFile.Name()
		tempConfigPath = tmpFile.Name()
	}

	contextName := a.currentCtx.Name
	if contextName == "" {
		if a.toastLogger != nil {
			return a.toastLogger.Errorf("kubectl edit: context name empty")
		}
		return a.ShowToast("kubectl edit failed: active context missing name", 5*time.Second)
	}

	namespace := strings.TrimSpace(obj.Namespace())
	if namespace == "" {
		namespace = strings.TrimSpace(a.currentCtx.Namespace)
	}

	resourceRef := kubectlResourceRef(obj.GVR(), obj.Name())

	commandStr := strings.Join(append([]string{"kubectl", "edit", resourceRef, "--context", contextName, "--kubeconfig", kubeconfigPath}), " ")
	args := []string{"edit", resourceRef, "--context", contextName, "--kubeconfig", kubeconfigPath}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
		commandStr = commandStr + " --namespace " + namespace
	}

	log.Info("launching kubectl edit", "command", commandStr)
	a.logCommandToTerminal(commandStr)

	// Ensure the UI returns to the primary panel view before launching the editor.
	if a.modalManager != nil {
		for a.modalManager.IsModalVisible() {
			a.hideModal()
		}
	}
	if a.showTerminal {
		a.showTerminal = false
		if a.terminal != nil {
			a.terminal.SetShowPanels(true)
		}
		a.invalidateView("kubectl edit exit terminal")
		a.invalidateFunctionBar("terminal exit")
		a.invalidateTerminalArea("terminal exit")
	}

	cmd := exec.Command("kubectl", args...)
	env := os.Environ()
	if kubeconfigPath != "" {
		env = append(env, "KUBECONFIG="+kubeconfigPath)
	}
	cmd.Env = env

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return kubectlEditFinishedMsg{
			err:         err,
			panelIndex:  panelIdx,
			resourceRef: resourceRef,
			tempConfig:  tempConfigPath,
		}
	})
}

func kubectlResourceRef(gvr schema.GroupVersionResource, name string) string {
	resource := strings.Join([]string{gvr.Resource, gvr.Version, gvr.Group}, ".")
	return fmt.Sprintf("%s/%s", resource, name)
}

func (a *App) applyStartupIntent() tea.Cmd {
	if a.startupIntentApplied || a.startupIntent.Verb == KubectlVerbNone {
		return nil
	}
	a.startupIntentApplied = true
	switch a.startupIntent.Verb {
	case KubectlVerbGet:
		return a.applyStartupIntentGet()
	case KubectlVerbLogs:
		return a.applyStartupIntentLogs()
	default:
		return nil
	}
}

func (a *App) applyStartupIntentGet() tea.Cmd {
	intent := a.startupIntent.Get
	if intent == nil || a.leftNav == nil {
		return nil
	}
	order, groups, err := groupGetTargets(intent)
	if err != nil {
		a.notifyIntentError("kubectl get: %v", err)
		return nil
	}
	ns := strings.TrimSpace(a.startupIntent.Namespace)
	if ns == "" {
		ns = a.currentNamespace()
	}
	resolved, warn := a.resolveGetResources(ns, order)
	if len(resolved) == 0 {
		if warn != "" {
			a.notifyIntentError("kubectl get: %s", warn)
		}
		return nil
	}
	if warn != "" {
		a.notifyIntentError("kubectl get: %s", warn)
	}
	var cmds []tea.Cmd
	if len(order) == 1 {
		res := resolved[order[0]]
		group := groups[order[0]]
		if err := a.goToResourceFolder(res); err != nil {
			a.notifyIntentError("kubectl get %s: %v", res.name, err)
			return nil
		}
		a.setActivePanel(0, "startup get focus left")
		names := group.Names
		switch len(names) {
		case 0:
			// already at resource list
		case 1:
			if cmd := a.selectSingleObject(res, ns, names[0], intent.OutputFormat); cmd != nil {
				cmds = append(cmds, cmd)
			}
		default:
			ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
			a.leftPanel.SelectRowIDs(ctxSel, names)
			cancelSel()
			if strings.EqualFold(intent.OutputFormat, "yaml") {
				if cmd := a.ensureManifestPreview(res, ns, names[0]); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
	} else {
		idList := make([]string, 0, len(order))
		for _, resName := range order {
			if res, ok := resolved[resName]; ok {
				idList = append(idList, res.rowID)
			}
		}
		if len(idList) > 0 {
			ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
			a.leftPanel.SelectRowIDs(ctxSel, idList)
			cancelSel()
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) applyStartupIntentLogs() tea.Cmd {
	intent := a.startupIntent.Logs
	if intent == nil || a.leftNav == nil {
		return nil
	}
	ns := strings.TrimSpace(a.startupIntent.Namespace)
	if ns == "" {
		ns = a.currentNamespace()
	}
	if ns == "" {
		ns = corev1.NamespaceDefault
	}
	order := []string{"pods"}
	resolved, warn := a.resolveGetResources(ns, order)
	if warn != "" {
		a.notifyIntentError("kubectl logs: %s", warn)
	}
	res, ok := resolved["pods"]
	if !ok {
		return nil
	}
	if err := a.goToResourceFolder(res); err != nil {
		a.notifyIntentError("kubectl logs %s: %v", intent.Pod, err)
		return nil
	}
	target, err := a.resolveLogsContainer(ns, intent)
	if err != nil {
		a.notifyIntentError("kubectl logs %s: %v", intent.Pod, err)
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
	defer cancel()
	steps := []navui.GoToStep{
		{SelectionID: intent.Pod, Enter: true},
		{SelectionID: target.SectionID, Enter: true},
		{SelectionID: target.Container, Enter: true},
		{SelectionID: "logs_latest", Enter: false},
	}
	if _, err := navui.GoTo(ctx, a.leftNav, steps); err != nil {
		a.notifyIntentError("kubectl logs %s: %v", intent.Pod, err)
		return nil
	}
	a.syncPanelWithNavigator(0)
	ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.SelectByRowID(ctxSel, "logs_latest")
	cancelSel()
	a.setActivePanel(0, "startup logs focus left")
	return a.openLogsViewerForIntent(intent)
}

func (a *App) currentNamespace() string {
	if a.currentCtx != nil {
		return strings.TrimSpace(a.currentCtx.Namespace)
	}
	return ""
}

func (a *App) resolveResource(resource string) (schema.GroupVersionResource, bool, error) {
	var zero schema.GroupVersionResource
	if a.cl == nil {
		return zero, false, fmt.Errorf("cluster not ready")
	}
	mapper := a.cl.RESTMapper()
	gvr, err := mapper.ResourceFor(schema.GroupVersionResource{Resource: resource})
	if err != nil {
		return zero, false, err
	}
	namespaced := true
	if kind, err := mapper.KindFor(gvr); err == nil {
		if mapping, err := mapper.RESTMapping(kind.GroupKind(), gvr.Version); err == nil && mapping.Scope != nil {
			namespaced = mapping.Scope.Name() == metamapper.RESTScopeNameNamespace
		}
	}
	return gvr, namespaced, nil
}

func resourceRowID(namespace string, gvr schema.GroupVersionResource, namespaced bool) string {
	if namespaced {
		return fmt.Sprintf("%s/%s/%s/%s", namespace, gvr.Group, gvr.Version, gvr.Resource)
	}
	return fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

type resolvedResource struct {
	name       string
	gvr        schema.GroupVersionResource
	namespaced bool
	rowID      string
	namespace  string
}

func (a *App) resolveGetResources(namespace string, order []string) (map[string]resolvedResource, string) {
	resolved := make(map[string]resolvedResource, len(order))
	var warn string
	for _, res := range order {
		gvr, namespaced, err := a.resolveResource(res)
		if err != nil {
			if warn == "" {
				warn = fmt.Sprintf("%s: %v", res, err)
			}
			continue
		}
		ns := namespace
		if namespaced && ns == "" {
			ns = corev1.NamespaceDefault
		}
		resolved[res] = resolvedResource{
			name:       res,
			gvr:        gvr,
			namespaced: namespaced,
			rowID:      resourceRowID(ns, gvr, namespaced),
			namespace:  ns,
		}
	}
	return resolved, warn
}

func (a *App) goToResourceFolder(res resolvedResource) error {
	if a.leftNav == nil {
		return fmt.Errorf("navigation not ready")
	}
	ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
	defer cancel()
	// Start from root to ensure cluster-scoped resources are reachable.
	for a.leftNav.HasBack() {
		a.leftNav.Back()
	}
	steps := []navui.GoToStep{}
	if res.namespaced {
		nsName := res.namespace
		if nsName == "" {
			nsName = corev1.NamespaceDefault
		}
		steps = append(steps,
			navui.GoToStep{SelectionID: "namespaces", Enter: true},
			navui.GoToStep{SelectionID: nsName, Enter: true},
		)
	}
	steps = append(steps, navui.GoToStep{SelectionID: res.rowID, Enter: true})
	if _, err := navui.GoTo(ctx, a.leftNav, steps); err != nil {
		return err
	}
	a.syncPanelWithNavigator(0)
	return nil
}

func (a *App) selectSingleObject(res resolvedResource, namespace, name, outputFormat string) tea.Cmd {
	if name == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
	defer cancel()
	if _, err := navui.GoTo(ctx, a.leftNav, []navui.GoToStep{{SelectionID: name, Enter: false}}); err != nil {
		a.notifyIntentError("select %s/%s: %v", res.name, name, err)
		return nil
	}
	a.syncPanelWithNavigator(0)
	ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.SelectByRowID(ctxSel, name)
	cancelSel()
	if strings.EqualFold(outputFormat, "yaml") {
		return a.ensureManifestPreview(res, namespace, name)
	}
	return nil
}

func (a *App) ensureManifestPreview(res resolvedResource, namespace, name string) tea.Cmd {
	if a.rightNav == nil || name == "" {
		return nil
	}
	ns := namespace
	if res.namespaced && ns == "" {
		ns = corev1.NamespaceDefault
	}
	ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
	defer cancel()
	steps := []navui.GoToStep{{SelectionID: resourceRowID(ns, res.gvr, res.namespaced), Enter: true}, {SelectionID: name, Enter: false}}
	if _, err := navui.GoTo(ctx, a.rightNav, steps); err != nil {
		a.notifyIntentError("manifest %s/%s: %v", res.name, name, err)
		return nil
	}
	a.syncPanelWithNavigator(1)
	ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
	a.rightPanel.SelectByRowID(ctxSel, name)
	cancelSel()
	ctxMode, cancelMode := context.WithTimeout(a.ctx, panelContextTimeout)
	cmd := a.rightPanel.SetMode(ctxMode, PanelModeManifest)
	cancelMode()
	return cmd
}

func (a *App) notifyIntentError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if a.toastLogger != nil {
		a.enqueueCmd(a.toastLogger.Errorf("%s", msg))
	}
}

type logsContainerTarget struct {
	Container string
	SectionID string
}

const (
	logSectionContainers = "containers"
	logSectionInit       = "init"
	logSectionEphemeral  = "ephemeral"
)

func (a *App) resolveLogsContainer(namespace string, intent *LogsIntent) (logsContainerTarget, error) {
	var zero logsContainerTarget
	if a.cl == nil {
		return zero, fmt.Errorf("cluster not ready")
	}
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	obj, err := a.cl.GetByGVR(a.ctx, gvr, namespace, intent.Pod)
	if err != nil {
		return zero, err
	}
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		return zero, err
	}
	return choosePodContainer(&pod, intent.Container)
}

func choosePodContainer(pod *corev1.Pod, requested string) (logsContainerTarget, error) {
	var zero logsContainerTarget
	req := strings.TrimSpace(requested)
	if req != "" {
		if containsContainer(pod.Spec.Containers, req) {
			return logsContainerTarget{Container: req, SectionID: logSectionContainers}, nil
		}
		if containsContainer(pod.Spec.InitContainers, req) {
			return logsContainerTarget{Container: req, SectionID: logSectionInit}, nil
		}
		if containsEphemeral(pod.Spec.EphemeralContainers, req) {
			return logsContainerTarget{Container: req, SectionID: logSectionEphemeral}, nil
		}
		return zero, fmt.Errorf("container %q not found in pod %s", req, pod.Name)
	}
	if len(pod.Spec.Containers) == 1 {
		return logsContainerTarget{Container: pod.Spec.Containers[0].Name, SectionID: logSectionContainers}, nil
	}
	if len(pod.Spec.Containers) == 0 && len(pod.Spec.InitContainers) == 1 {
		return logsContainerTarget{Container: pod.Spec.InitContainers[0].Name, SectionID: logSectionInit}, nil
	}
	if len(pod.Spec.Containers) == 0 && len(pod.Spec.EphemeralContainers) == 1 {
		return logsContainerTarget{Container: pod.Spec.EphemeralContainers[0].Name, SectionID: logSectionEphemeral}, nil
	}
	if len(pod.Spec.Containers) == 0 && len(pod.Spec.InitContainers) == 0 && len(pod.Spec.EphemeralContainers) == 0 {
		return zero, fmt.Errorf("pod %s has no containers", pod.Name)
	}
	names := make([]string, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	return zero, fmt.Errorf("pod %s has multiple containers (%s); specify -c", pod.Name, strings.Join(names, ", "))
}

func containsContainer(list []corev1.Container, name string) bool {
	for _, c := range list {
		if c.Name == name {
			return true
		}
	}
	return false
}

func containsEphemeral(list []corev1.EphemeralContainer, name string) bool {
	for _, c := range list {
		if c.Name == name {
			return true
		}
	}
	return false
}

func (a *App) openLogsViewerForIntent(intent *LogsIntent) tea.Cmd {
	if intent == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	item, ok := a.leftPanel.SelectedNavItem(ctx)
	if !ok || item == nil {
		return nil
	}
	logsProvider, ok := item.(models.LogsProvider)
	if !ok {
		return nil
	}
	spec := logsProvider.LogsSpec()
	spec.Follow = intent.Follow
	return a.openLogsViewer(item, spec)
}

func (a *App) logCommandToTerminal(command string) {
	if command == "" {
		return
	}
	fmt.Fprintf(os.Stdout, "\n[kc] %s\n", command)
}

func (a *App) ensureTermContext() {
	if !a.cfg.Terminal.Follow || a.currentCtx == nil || a.currentCtx.Kubeconfig == nil {
		if a.termCtx != nil {
			_ = a.termCtx.Close()
			a.termCtx = nil
		}
		a.terminal.SetEnv(nil)
		return
	}
	if a.termCtx != nil {
		_ = a.termCtx.Close()
	}
	mode := convertTerminalMode(a.cfg.Terminal.Mode)
	mgr, err := termctx.NewManager(a.currentCtx.Kubeconfig.Path, a.impersonationTemplate(), mode, a.impersonationSpec())
	if err != nil {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Terminal overlay: %v", err))
		}
		return
	}
	a.termCtx = mgr
	a.terminal.SetEnv(a.buildTerminalEnv())
	a.updateTermContextNamespace(strings.TrimSpace(a.currentCtx.Namespace))
}

func (a *App) updateTermContextNamespace(ns string) {
	if !a.cfg.Terminal.Follow || a.termCtx == nil || a.currentCtx == nil {
		return
	}
	if err := a.termCtx.Update(a.currentCtx.Name, ns); err != nil {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Terminal context update failed: %v", err))
		}
		return
	}
	log := ctrllog.FromContext(a.ctx).WithName("termctx")
	if overlay := a.termCtx.OverlayPath(); overlay != "" {
		log.Info("updated terminal overlay kubeconfig", "path", overlay, "context", a.currentCtx.Name, "namespace", ns)
		return
	}
	log.Info("updated terminal kubeconfig copy", "path", a.termCtx.EnvValue(), "context", a.currentCtx.Name, "namespace", ns)
}

func (a *App) buildTerminalEnv() []string {
	env := os.Environ()
	if a.termCtx == nil {
		return env
	}
	value := fmt.Sprintf("KUBECONFIG=%s", a.termCtx.EnvValue())
	return append(filterEnv(env, "KUBECONFIG"), value)
}

func filterEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func convertTerminalMode(mode appconfig.TerminalMode) termctx.Mode {
	switch mode {
	case appconfig.TerminalModeCopy:
		return termctx.ModeCopy
	default:
		return termctx.ModeOverlay
	}
}

func (a *App) refreshPanelAfterEdit(panelIdx int) {
	var panel *Panel
	var nav *navui.Navigator
	if panelIdx == 0 {
		panel = a.leftPanel
		nav = a.leftNav
	} else {
		panel = a.rightPanel
		nav = a.rightNav
	}

	if nav != nil {
		if rf, ok := nav.Current().(interface{ Refresh() }); ok {
			rf.Refresh()
		}
	}

	if panel != nil {
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		panel.RefreshFolder(ctx)
		cancel()
	}
}

func (a *App) copyItem() tea.Cmd {
	return a.copyItemForPanel(a.activePanelRef())
}

func (a *App) renameMoveItem() tea.Cmd {
	// TODO: Implement rename/move functionality (F6)
	return nil
}

func (a *App) copyItemForPanel(panel *Panel) tea.Cmd {
	if panel == nil {
		panel = a.activePanelRef()
	}
	if panel == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	item, ok := panel.SelectedNavItem(ctx)
	cancel()
	if !ok || item == nil {
		return nil
	}
	if _, isBack := item.(models.Back); isBack {
		return nil
	}

	if logs, ok := item.(models.LogsProvider); ok {
		if a.cl == nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("Copy unavailable: cluster not ready"))
			}
			return nil
		}
		spec := logs.LogsSpec()
		if spec.TailLines <= 0 {
			spec.TailLines = models.DefaultLogsTailLines
		}
		subject := fmt.Sprintf("logs %s/%s/%s", spec.Namespace, spec.Pod, spec.Container)
		filename := sanitizeFilename(fmt.Sprintf("%s-%s.log", spec.Pod, spec.Container))
		if filename == "" {
			filename = "logs.txt"
		}
		req := &copyRequest{
			subject:  subject,
			filename: filename,
			fetch: func(ctx context.Context) (io.ReadCloser, error) {
				specCopy := spec
				specCopy.Follow = false
				return a.fetchLogsSnapshot(ctx, specCopy)
			},
		}
		return a.showCopyDialog(req)
	}

	viewable, ok := item.(models.Viewable)
	if !ok {
		type viewContentProvider interface {
			ViewContent() (string, string, string, string, string, error)
		}
		if alt, ok := item.(viewContentProvider); ok {
			viewable = alt
		} else {
			return nil
		}
	}
	title, body, _, _, filename, err := viewable.ViewContent()
	if err != nil {
		if errors.Is(err, models.ErrNoViewContent) {
			return nil
		}
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Copy failed: %v", err))
		}
		return nil
	}
	if filename == "" {
		filename = filepath.Base(a.modalTitleFromItem(item, title))
	}
	filename = sanitizeFilename(filename)
	if filename == "" {
		filename = "resource.txt"
	}
	data := []byte(body)
	subject := a.modalTitleFromItem(item, title)
	req := &copyRequest{
		subject:  subject,
		filename: filename,
		fetch: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}
	return a.showCopyDialog(req)
}

func (a *App) showCopyDialog(req *copyRequest) tea.Cmd {
	if req == nil || req.fetch == nil || a.copyInput == nil || a.modalManager == nil {
		return nil
	}
	modal := a.modalManager.modals["copy_local"]
	if modal == nil {
		return nil
	}
	dir := a.defaultCopyDir()
	target := filepath.Join(dir, req.filename)
	a.copyInput.Configure(req.subject, target)
	a.configureModalWindow(modal, a.copyInput, -1, "", 0, 0)
	modal.SetOnClose(func() tea.Cmd {
		a.pendingCopy = nil
		if a.copyInput != nil {
			a.copyInput.BlurInputs()
		}
		return nil
	})
	a.pendingCopy = req
	a.showModal("copy_local")
	return a.copyInput.FocusPath()
}

func (a *App) defaultCopyDir() string {
	if strings.TrimSpace(a.lastCopyDir) != "" {
		return a.lastCopyDir
	}
	if wd, err := os.Getwd(); err == nil {
		if abs, err := filepath.Abs(wd); err == nil {
			return abs
		}
		return wd
	}
	if abs, err := filepath.Abs("."); err == nil {
		return abs
	}
	return "."
}

func (a *App) executeCopyRequest(path string) tea.Cmd {
	req := a.pendingCopy
	a.pendingCopy = nil
	if req == nil {
		return nil
	}
	target := strings.TrimSpace(path)
	if target == "" {
		return nil
	}
	return a.withBusy("Copy", 300*time.Millisecond, func() tea.Msg {
		err := a.copyToPath(req, target)
		return copyFinishedMsg{path: target, subject: req.subject, err: err}
	})
}

func (a *App) copyToPath(req *copyRequest, dest string) error {
	dir := filepath.Dir(dest)
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("destination %q is not a directory", dir)
	}
	ctx, cancel := context.WithTimeout(a.ctx, copyRequestTimeout)
	defer cancel()
	reader, err := req.fetch(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := io.Copy(file, reader); err != nil {
		_ = os.Remove(dest)
		return err
	}
	closed = true
	return file.Close()
}

type copyRequest struct {
	subject  string
	filename string
	fetch    func(ctx context.Context) (io.ReadCloser, error)
}

type copyFinishedMsg struct {
	path    string
	subject string
	err     error
}

func sanitizeFilename(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return ""
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func (a *App) fetchLogsSnapshot(ctx context.Context, spec models.LogsSpec) (io.ReadCloser, error) {
	if a.cl == nil {
		return nil, fmt.Errorf("cluster not ready")
	}
	cfg := rest.CopyConfig(a.cl.GetConfig())
	clientset, err := kubernetes.NewForConfig(rest.AddUserAgent(cfg, "kc-copy-logs"))
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{
		Container: spec.Container,
		Follow:    false,
	}
	if spec.TailLines > 0 {
		opts.TailLines = &spec.TailLines
	}
	if spec.Namespace == "" || spec.Pod == "" {
		return nil, fmt.Errorf("logs spec incomplete")
	}
	req := clientset.CoreV1().Pods(spec.Namespace).GetLogs(spec.Pod, opts)
	return req.Stream(ctx)
}

// RunConfig allows callers to customize the UI startup.
type RunConfig struct {
	Namespace     string
	StartupIntent StartupIntent
	// DebugLogPath, when non-empty, points to the UI log file (e.g., ~/.kc/debug.log).
	DebugLogPath string
	// SwitchToFileLogger is called right before the UI program starts. Allows callers
	// to stop logging to stderr once bubbletea takes over the terminal.
	SwitchToFileLogger func()
	ImpersonateUser    string
	ImpersonateUID     string
	ImpersonateGroups  []string
}

// Run starts the application
func Run(ctx context.Context, cfg RunConfig) error {
	ctx = ctrllog.IntoContext(ctx, ctrllog.Log.WithName("startup"))
	log := ctrllog.FromContext(ctx)
	app := NewApp()
	app.namespaceOverride = strings.TrimSpace(cfg.Namespace)
	app.startupIntent = cfg.StartupIntent
	app.impersonateUser = strings.TrimSpace(cfg.ImpersonateUser)
	app.impersonateUID = strings.TrimSpace(cfg.ImpersonateUID)
	app.impersonateGroups = append([]string(nil), cfg.ImpersonateGroups...)

	// Initialize data model (best-effort; UI can still run without it)
	log.Info("initializing data")
	if err := app.initData(ctx); err != nil {
		log.Error(err, "initialization warning")
		fmt.Printf("Data init warning: %v\n", err)
	}
	if cfg.DebugLogPath != "" {
		log.Info("initialization complete, launching UI; check debug.log for further log output while the UI is running", "debugLogPath", cfg.DebugLogPath)
	} else {
		log.Info("initialization complete, launching UI")
	}

	// Create program with proper options
	p := tea.NewProgram(
		app,
		tea.WithoutSignalHandler(), // Handle signals ourselves
	)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		// Send quit message to the program
		p.Quit()
	}()

	if cfg.SwitchToFileLogger != nil {
		cfg.SwitchToFileLogger()
		// Refresh contexts so logs use the file-only, unnamed logger (no lingering "startup" name).
		baseLog := ctrllog.Log
		ctx = ctrllog.IntoContext(ctx, baseLog)
		app.ctx = ctrllog.IntoContext(app.ctx, baseLog)
		log = ctrllog.FromContext(ctx)
	}

	// Ensure terminal is reset on exit
	defer func() {
		// Close the embedded terminal/PTY to restore its raw mode
		if app.terminal != nil {
			_ = app.terminal.Close()
		}
		// Stop background resources
		if app.discoveryCancel != nil {
			app.discoveryCancel()
			app.discoveryCancel = nil
		}
		if app.clPool != nil {
			app.clPool.Stop()
		}
		app.cancel()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		return err
	}

	return nil
}

// initData discovers kubeconfigs, selects current context, starts cluster/cache and wires navigation.
func (a *App) initData(ctx context.Context) error {
	log := ctrllog.FromContext(ctx).WithName("init")
	// Kubeconfig manager and discovery
	a.kubeMgr = kubeconfig.NewManager()
	log.Info("discovering kubeconfigs")
	if err := a.kubeMgr.DiscoverKubeconfigs(); err != nil {
		// Log and show toast
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Kubeconfig discovery failed: %v", err))
		}
		log.Error(err, "failed to discover kubeconfigs")
		return fmt.Errorf("discover kubeconfigs: %w", err)
	}
	log.Info("kubeconfigs discovered", "count", len(a.kubeMgr.GetKubeconfigs()), "contexts", len(a.kubeMgr.GetContexts()))
	// Select current context (prefer env KUBECONFIG first path)
	a.currentCtx = a.selectCurrentContext()
	if a.currentCtx == nil {
		log.Error(nil, "no current context found")
		return fmt.Errorf("no current context found")
	}
	ctxNamespace := strings.TrimSpace(a.currentCtx.Namespace)
	override := strings.TrimSpace(a.namespaceOverride)
	if override != "" {
		log.Info("namespace override requested", "requested", override, "original", ctxNamespace)
		ctxNamespace = override
		a.currentCtx.Namespace = override
	}
	nsLog := ctxNamespace
	if nsLog == "" {
		nsLog = "(cluster)"
	}
	log.Info("selected context", "name", a.currentCtx.Name, "cluster", a.currentCtx.Cluster, "namespace", nsLog)
	// Prepare app context and cluster pool; cluster will be started via pool.Get
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.clPool = kccluster.NewPool(
		a.cfg.Kubernetes.Clusters.TTL.Duration,
		a.cfg.Kubernetes.Discovery.Refresh.Duration,
	)
	log.Info("starting cluster pool")
	a.clPool.Start()
	k := kccluster.Key{
		KubeconfigPath:       a.currentCtx.Kubeconfig.Path,
		ContextName:          a.currentCtx.Name,
		ImpersonateUser:      strings.TrimSpace(a.impersonateUser),
		ImpersonateUID:       strings.TrimSpace(a.impersonateUID),
		ImpersonateGroupsKey: a.impersonationGroupsKey(),
	}
	log.Info("acquiring cluster", "key", k)
	cl, err := a.clPool.Get(a.ctx, k)
	if err != nil {
		log.Error(err, "cluster acquisition failed")
		return fmt.Errorf("cluster pool get: %w", err)
	}
	a.cl = cl
	if factory, err := podfs.NewFactory(a.cl.GetConfig()); err != nil {
		log.Error(err, "failed to initialize pod filesystem factory")
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Pod filesystem unavailable: %v", err))
		}
	} else {
		a.podfsFactory = factory
	}
	log.Info("cluster ready, fetching resource info")
	// Discovery-backed catalog (for panel displays)
	if infos, err := a.cl.GetResourceInfos(); err == nil {
		log.Info("resource infos fetched", "count", len(infos))
		a.leftPanel.SetResourceCatalog(infos)
		a.rightPanel.SetResourceCatalog(infos)
	} else {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Discovery resources failed: %v", err))
		}
		log.Error(err, "failed to fetch resource infos")
	}
	a.ensureTermContext()
	// Subscribe to discovery refresh events so resource folders can update dynamically.
	if a.discoveryCh == nil {
		a.discoveryCh = make(chan struct{}, 1)
	}
	if a.discoveryCancel != nil {
		a.discoveryCancel()
	}
	a.discoveryCancel = a.cl.AddDiscoveryListener(func() {
		select {
		case a.discoveryCh <- struct{}{}:
		default:
		}
	})
	a.enqueueCmd(a.watchDiscovery())
	// Legacy generic data sources removed; folders provide data directly
	a.leftPanel.SetViewConfig(a.viewConfig)
	a.rightPanel.SetViewConfig(a.viewConfig)
	// Provide contexts count to panels for root display
	a.leftPanel.SetContextCountProvider(func() int { return len(a.kubeMgr.GetContexts()) })
	a.rightPanel.SetContextCountProvider(func() int { return len(a.kubeMgr.GetContexts()) })
	// Initialize per-panel view options from config defaults
	if a.cfg != nil {
		a.leftPanel.SetResourceViewOptions(a.cfg.Resources.ShowNonEmptyOnly, string(a.cfg.Resources.Order))
		a.rightPanel.SetResourceViewOptions(a.cfg.Resources.ShowNonEmptyOnly, string(a.cfg.Resources.Order))
		a.applyResourceOptions(a.leftPanel)
		a.applyResourceOptions(a.rightPanel)
		// Initialize table mode from config defaults
		ctxLeft, cancelLeft := context.WithTimeout(a.ctx, panelContextTimeout)
		a.leftPanel.SetTableMode(ctxLeft, string(a.cfg.Panel.Table.Mode))
		a.leftPanel.SetColumnsMode(ctxLeft, a.cfg.Objects.Columns)
		a.leftPanel.SetObjectOrder(ctxLeft, a.cfg.Objects.Order)
		cancelLeft()
		ctxRight, cancelRight := context.WithTimeout(a.ctx, panelContextTimeout)
		a.rightPanel.SetTableMode(ctxRight, string(a.cfg.Panel.Table.Mode))
		a.rightPanel.SetColumnsMode(ctxRight, a.cfg.Objects.Columns)
		a.rightPanel.SetObjectOrder(ctxRight, a.cfg.Objects.Order)
		cancelRight()
		// Initialize columns mode and objects order from config defaults
		a.syncPanelConfig(a.leftPanel)
		a.syncPanelConfig(a.rightPanel)
	}
	// Preview: Use folder-backed rendering starting at root (not contexts listing)
	{
		// Programmatic navigation to current namespace for both panels
		ns := a.initialNamespace()
		if strings.TrimSpace(a.namespaceOverride) == "" {
			ctxNs := ""
			if a.currentCtx != nil {
				ctxNs = strings.TrimSpace(a.currentCtx.Namespace)
			}
			if ctxNs == "" && ns == corev1.NamespaceDefault {
				log.Info("no namespace set in context; defaulting", "namespace", ns)
			}
		}
		log.Info("initial navigation", "namespace", ns)
		a.goToNamespace(ns)
	}
	log.Info("panel initialization complete")
	return nil
}

// Legacy builder helpers removed (replaced by self-sufficient folders).

// goToNamespace programmatically navigates to /namespaces/<ns> and updates panels.
// If ns is empty, the panels remain at cluster scope. If the namespace does not exist, it retries briefly before staying at cluster scope.
func (a *App) goToNamespace(ns string) {
	a.goToNamespaceWithRetry(ns, true)
}

func (a *App) goToNamespaceWithRetry(ns string, reset bool) {
	log := ctrllog.FromContext(a.ctx).WithName("gotoNamespace")
	if reset {
		a.namespaceAutoTarget = ns
		a.namespaceAutoAttempts = 0
		log.Info("namespace navigation requested", "namespace", ns)
	} else if ns != a.namespaceAutoTarget {
		log.Info("namespace navigation skipped; target changed", "namespace", ns, "autoTarget", a.namespaceAutoTarget)
		return
	}
	leftCfg := a.ensurePanelConfig(a.leftPanel)
	rightCfg := a.ensurePanelConfig(a.rightPanel)
	a.syncPanelConfig(a.leftPanel)
	a.syncPanelConfig(a.rightPanel)
	key := a.currentClusterKey()
	if a.clPool != nil {
		// Keep at most the cluster-scoped cache plus the current namespace cache.
		a.clPool.PruneNamespaces(ns)
	}
	depsLeft := a.makeDeps(a.cl, leftCfg, key)
	depsRight := a.makeDeps(a.cl, rightCfg, key)
	enterLeft := a.makeEnterContextFunc(leftCfg)
	enterRight := a.makeEnterContextFunc(rightCfg)
	rootLeft := models.NewRootFolder(depsLeft, enterLeft)
	rootRight := models.NewRootFolder(depsRight, enterRight)
	a.leftNav = navui.NewNavigator(rootLeft)
	a.rightNav = navui.NewNavigator(rootRight)

	logged := false
	retryScheduled := false

	if ns == "" {
		log.Info("no namespace configured, staying at cluster scope")
		a.namespaceAutoTarget = ""
		a.namespaceAutoAttempts = 0
		a.updateTermContextNamespace("")
		logged = true
	} else {
		log.Info("checking namespace readiness", "namespace", ns, "attempt", a.namespaceAutoAttempts)
		nsReady, terminal, readyErr := a.namespaceReady(ns)
		if readyErr != nil {
			log.Error(readyErr, "checking namespace readiness", "namespace", ns)
		}
		log.Info("namespace readiness evaluated", "namespace", ns, "ready", nsReady, "terminal", terminal)
		switch {
		case nsReady:
			if !a.forceNamespaceNavigation(ns, depsLeft, depsRight) {
				log.Error(fmt.Errorf("navigation failed"), "unable to navigate to namespace", "namespace", ns)
				if ns == a.namespaceAutoTarget {
					a.namespaceAutoTarget = ""
				}
				logged = true
				break
			}
			log.Info("navigated to namespace", "namespace", ns)
			a.updateTermContextNamespace(ns)
			logged = true
			if ns == a.namespaceAutoTarget {
				a.namespaceAutoTarget = ""
				a.namespaceAutoAttempts = 0
			}
		case terminal:
			log.Info("namespace not found, staying at cluster scope", "namespace", ns)
			if ns == a.namespaceAutoTarget {
				a.namespaceAutoTarget = ""
			}
			logged = true
		case a.ctx.Err() == nil && ns == a.namespaceAutoTarget && a.namespaceAutoAttempts < namespaceRetryMaxAttempts:
			a.namespaceAutoAttempts++
			log.Info("namespace not ready yet, retrying", "namespace", ns, "attempt", a.namespaceAutoAttempts)
			a.enqueueCmd(a.namespaceRetryCmd(ns))
			retryScheduled = true
			logged = true
		default:
			log.Info("namespace not ready, staying at cluster scope", "namespace", ns)
			if ns == a.namespaceAutoTarget {
				a.namespaceAutoTarget = ""
			}
			logged = true
		}
	}

	if !logged && !retryScheduled && ns != "" {
		log.Info("namespace not found, staying at cluster scope", "namespace", ns)
		if ns == a.namespaceAutoTarget {
			a.namespaceAutoTarget = ""
		}
	}

	curL := a.leftNav.Current()
	hasBackL := a.leftNav.HasBack()
	curR := a.rightNav.Current()
	hasBackR := a.rightNav.HasBack()
	ctxLeft, cancelLeft := context.WithTimeout(a.ctx, panelContextTimeout)
	a.setPanelFolder(ctxLeft, 0, curL, hasBackL)
	cancelLeft()
	ctxRight, cancelRight := context.WithTimeout(a.ctx, panelContextTimeout)
	a.setPanelFolder(ctxRight, 1, curR, hasBackR)
	cancelRight()
	// Use navigator paths for breadcrumbs
	a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
	a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))
	ctxUseLeft, cancelUseLeft := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.UseFolder(ctxUseLeft, true)
	cancelUseLeft()
	ctxUseRight, cancelUseRight := context.WithTimeout(a.ctx, panelContextTimeout)
	a.rightPanel.UseFolder(ctxUseRight, true)
	cancelUseRight()
	a.applyResourceOptions(a.leftPanel)
	a.applyResourceOptions(a.rightPanel)
	a.leftPanel.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		a.setActivePanel(0, "left panel folder nav focus")
		a.handleFolderNav(back, selID, next)
	})
	a.rightPanel.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		a.setActivePanel(1, "right panel folder nav focus")
		a.handleFolderNav(back, selID, next)
	})
	ctxResetL, cancelResetL := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.ResetSelectionTop(ctxResetL)
	cancelResetL()
	ctxResetR, cancelResetR := context.WithTimeout(a.ctx, panelContextTimeout)
	a.rightPanel.ResetSelectionTop(ctxResetR)
	cancelResetR()
	a.scheduleStartupIntent()
}

func (a *App) namespaceRetryCmd(ns string) tea.Cmd {
	return func() tea.Msg {
		timer := time.NewTimer(namespaceRetryInterval)
		defer timer.Stop()
		select {
		case <-timer.C:
			return namespaceRetryMsg{namespace: ns}
		case <-a.ctx.Done():
			return nil
		}
	}
}

func (a *App) forceNamespaceNavigation(ns string, depsLeft, depsRight models.Deps) bool {
	gvrNamespaces := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsListPath := []string{"namespaces"}
	nsPath := []string{"namespaces", ns}
	log := ctrllog.FromContext(a.ctx).WithName("gotoNamespace").WithValues("namespace", ns)

	leftList := models.NewClusterObjectsFolder(depsLeft, gvrNamespaces, nsListPath, models.NamespaceResourceVerbs())
	rightList := models.NewClusterObjectsFolder(depsRight, gvrNamespaces, nsListPath, models.NamespaceResourceVerbs())
	leftResources := models.NewNamespacedResourcesFolder(depsLeft, ns, nsPath)
	rightResources := models.NewNamespacedResourcesFolder(depsRight, ns, nsPath)

	log.Info("pushing namespace folders to navigators")

	a.leftNav.SetSelectionID("namespaces")
	a.leftNav.Push(leftList)
	a.leftNav.SetSelectionID(ns)
	a.leftNav.Push(leftResources)
	log.Info("left navigator updated", "stackDepth", a.navigatorDepth(a.leftNav))

	a.rightNav.SetSelectionID("namespaces")
	a.rightNav.Push(rightList)
	a.rightNav.SetSelectionID(ns)
	a.rightNav.Push(rightResources)
	log.Info("right navigator updated", "stackDepth", a.navigatorDepth(a.rightNav))

	return true
}

func (a *App) scheduleStartupIntent() {
	if a.startupIntentScheduled || a.startupIntent.Verb == KubectlVerbNone {
		return
	}
	a.startupIntentScheduled = true
	a.enqueueCmd(func() tea.Msg { return startupIntentMsg{} })
}

func (a *App) initialNamespace() string {
	if ns := strings.TrimSpace(a.namespaceOverride); ns != "" {
		return ns
	}
	if a.currentCtx != nil {
		if ns := strings.TrimSpace(a.currentCtx.Namespace); ns != "" {
			return ns
		}
	}
	return corev1.NamespaceDefault
}

// handleFolderNav processes back/forward navigation from panels and updates both panels.
// currentNav returns the navigator for the active panel (left=0, right=1).
func (a *App) currentNav() *navui.Navigator {
	if a.activePanel == 0 {
		return a.leftNav
	}
	return a.rightNav
}

func (a *App) navigatorDepth(nav *navui.Navigator) int {
	if nav == nil {
		return 0
	}
	count := 0
	nav.ForEach(func(models.Folder) { count++ })
	return count
}

func (a *App) syncPanelWithNavigator(panelIdx int) {
	var nav *navui.Navigator
	var panel *Panel
	switch panelIdx {
	case 0:
		nav = a.leftNav
		panel = a.leftPanel
	case 1:
		nav = a.rightNav
		panel = a.rightPanel
	}
	if nav == nil || panel == nil {
		return
	}
	cur := nav.Current()
	hasBack := nav.HasBack()
	ctxSet, cancelSet := context.WithTimeout(a.ctx, panelContextTimeout)
	a.setPanelFolder(ctxSet, panelIdx, cur, hasBack)
	cancelSet()
	panel.SetCurrentPath(a.navigatorPath(nav))
	ctxUse, cancelUse := context.WithTimeout(a.ctx, panelContextTimeout)
	panel.UseFolder(ctxUse, true)
	cancelUse()
}

func (a *App) handleFolderNav(back bool, selID string, next models.Folder) {
	var nav *navui.Navigator
	var panelSet func(context.Context, models.Folder, bool)
	var panelSelectByID func(context.Context, string)
	var panelReset func(context.Context)
	var panelRef *Panel
	if a.activePanel == 0 {
		cfg := a.ensurePanelConfig(a.leftPanel)
		a.syncPanelConfig(a.leftPanel)
		if a.leftNav == nil {
			deps := a.makeDeps(a.cl, cfg, a.currentClusterKey())
			enter := a.makeEnterContextFunc(cfg)
			a.leftNav = navui.NewNavigator(models.NewRootFolder(deps, enter))
		}
		nav = a.leftNav
		panelSet = func(ctx context.Context, folder models.Folder, hasBack bool) {
			a.setPanelFolder(ctx, 0, folder, hasBack)
		}
		panelSelectByID = func(ctx context.Context, id string) { a.leftPanel.SelectByRowID(ctx, id) }
		panelReset = func(ctx context.Context) { a.leftPanel.ResetSelectionTop(ctx) }
		panelRef = a.leftPanel
	} else {
		cfg := a.ensurePanelConfig(a.rightPanel)
		a.syncPanelConfig(a.rightPanel)
		if a.rightNav == nil {
			deps := a.makeDeps(a.cl, cfg, a.currentClusterKey())
			enter := a.makeEnterContextFunc(cfg)
			a.rightNav = navui.NewNavigator(models.NewRootFolder(deps, enter))
		}
		nav = a.rightNav
		panelSet = func(ctx context.Context, folder models.Folder, hasBack bool) {
			a.setPanelFolder(ctx, 1, folder, hasBack)
		}
		panelSelectByID = func(ctx context.Context, id string) { a.rightPanel.SelectByRowID(ctx, id) }
		panelReset = func(ctx context.Context) { a.rightPanel.ResetSelectionTop(ctx) }
		panelRef = a.rightPanel
	}
	if back {
		nav.Back()
	} else if next != nil {
		// Pre-warm the next folder in background to trigger informer/lister start.
		// This shows a spinner if it takes longer than the delay and avoids UI freeze.
		a.enqueueCmd(a.withBusy("Loading", 800*time.Millisecond, func() tea.Msg {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			defer cancel()
			_ = next.Len(ctx)
			return nil
		}))
		nav.SetSelectionID(selID)
		nav.Push(next)
	}
	cur := nav.Current()
	hasBack := nav.HasBack()
	ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
	panelSet(ctxPanel, cur, hasBack)
	cancelPanel()
	// Update breadcrumbs from navigator state
	if a.activePanel == 0 {
		a.leftPanel.SetCurrentPath(a.navigatorPath(nav))
	} else {
		a.rightPanel.SetCurrentPath(a.navigatorPath(nav))
	}
	if back {
		id := nav.CurrentSelectionID()
		if id != "" {
			ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
			panelSelectByID(ctxSel, id)
			cancelSel()
		} else {
			ctxReset, cancelReset := context.WithTimeout(a.ctx, panelContextTimeout)
			panelReset(ctxReset)
			cancelReset()
		}
	} else {
		ctxReset, cancelReset := context.WithTimeout(a.ctx, panelContextTimeout)
		panelReset(ctxReset)
		cancelReset()
	}
	a.applyResourceOptions(panelRef)
	a.syncTerminalNamespaceForNavigator(nav)
}

// namespaceExists returns true if the namespace exists in the current cluster.
func (a *App) namespaceExists(ns string) bool {
	if ns == "" {
		return false
	}
	if a.cl == nil {
		return false
	}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
	gvr, err := a.cl.GVKToGVR(gvk)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
	defer cancel()
	if _, err := a.cl.GetByGVR(ctx, gvr, "", ns); err != nil {
		return false
	}
	return true
}

func (a *App) namespaceReady(ns string) (ready bool, terminal bool, err error) {
	if ns == "" {
		return false, true, nil
	}
	if a.cl == nil {
		return false, true, fmt.Errorf("no cluster available")
	}
	dyn := a.cl.Dynamic()
	if dyn == nil {
		return false, false, fmt.Errorf("dynamic client unavailable")
	}
	ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
	defer cancel()
	res := dyn.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"})
	if _, err := res.Get(ctx, ns, metav1.GetOptions{}); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return false, true, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return false, false, nil
		case ctx.Err() != nil:
			return false, false, nil
		default:
			return false, false, err
		}
	}
	return true, true, nil
}

// selectCurrentContext prefers $KUBECONFIG current-context, else any current-context, else first discovered.
func (a *App) selectCurrentContext() *kubeconfig.Context {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		for _, p := range strings.Split(env, string(os.PathListSeparator)) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			for _, kc := range a.kubeMgr.GetKubeconfigs() {
				if sameFilepath(kc.Path, p) {
					if c := a.kubeMgr.GetCurrentContext(kc); c != nil {
						return c
					}
				}
			}
		}
	}
	for _, kc := range a.kubeMgr.GetKubeconfigs() {
		if c := a.kubeMgr.GetCurrentContext(kc); c != nil {
			return c
		}
	}
	cs := a.kubeMgr.GetContexts()
	if len(cs) > 0 {
		return cs[0]
	}
	return nil
}

func sameFilepath(a, b string) bool {
	ap, err1 := filepath.Abs(a)
	bp, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return ap == bp
}

func (a *App) GetObject(gvk schema.GroupVersionKind, namespace, name string) (map[string]interface{}, error) {
	gvr, err := a.cl.GVKToGVR(gvk)
	if err != nil {
		return nil, err
	}
	obj, err := a.cl.GetByGVR(a.ctx, gvr, namespace, name)
	if err != nil {
		return nil, err
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	return obj.Object, nil
}

// RESTMapper exposes the app's RESTMapper to viewers for resource→GVK resolution.
func (a *App) RESTMapper() metamapper.RESTMapper { return a.cl.RESTMapper() }

// logMsg appends Bubble Tea messages to a bounded buffer for later inspection.
func (a *App) logMsg(msg tea.Msg) {
	if a == nil {
		return
	}
	if msgLogLimit == 0 {
		return
	}
	entry := describeTeaMsg(msg)
	a.msgLog = append(a.msgLog, entry)
	if overflow := len(a.msgLog) - msgLogLimit; overflow > 0 {
		copy(a.msgLog, a.msgLog[overflow:])
		a.msgLog = a.msgLog[:len(a.msgLog)-overflow]
	}
}

// resetMsgLog clears the diagnostic buffer.
func (a *App) resetMsgLog() {
	if a == nil {
		return
	}
	a.msgLog = nil
}

// msgLogSnapshot copies the current diagnostic log for debugging or tests.
func (a *App) msgLogSnapshot() []string {
	if len(a.msgLog) == 0 {
		return nil
	}
	out := make([]string, len(a.msgLog))
	copy(out, a.msgLog)
	return out
}

func describeTeaMsg(msg tea.Msg) string {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return fmt.Sprintf("tea.WindowSizeMsg(%dx%d)", m.Width, m.Height)
	case tea.KeyMsg:
		return fmt.Sprintf("tea.KeyMsg(%s)", m.String())
	case tea.MouseMsg:
		return fmt.Sprintf("tea.MouseMsg(%s)", m.String())
	case tea.BatchMsg:
		return fmt.Sprintf("tea.BatchMsg(%d cmds)", len(m))
	case fmt.Stringer:
		return fmt.Sprintf("%T(%s)", msg, m.String())
	case error:
		return fmt.Sprintf("error(%v)", m)
	default:
		return fmt.Sprintf("%T", msg)
	}
}

func (c *cachedView) invalidate() { c.valid = false }

func (c *cachedView) set(view string, cursor *tea.Cursor) {
	c.view = view
	if cursor != nil {
		copy := *cursor
		c.cursor = &copy
	} else {
		c.cursor = nil
	}
	c.valid = true
}

func (c *cachedView) get() (string, *tea.Cursor, bool) {
	if !c.valid {
		return "", nil, false
	}
	var cursorCopy *tea.Cursor
	if c.cursor != nil {
		copy := *c.cursor
		cursorCopy = &copy
	}
	return c.view, cursorCopy, true
}

func (a *App) invalidateView(reason string) {
	if a == nil {
		return
	}
	if reason != "" {
		ctrllog.FromContext(a.ctx).WithName("ui").Info("view invalidated", "reason", reason)
	}
	a.mainViewCache.invalidate()
}

func (a *App) invalidateFunctionBar(reason string) {
	if a == nil {
		return
	}
	if reason != "" {
		ctrllog.FromContext(a.ctx).WithName("ui").Info("function bar invalidated", "reason", reason)
	}
	a.functionBarCache.invalidate()
}

func (a *App) invalidateTerminalArea(reason string) {
	if a == nil {
		return
	}
	if reason != "" {
		ctrllog.FromContext(a.ctx).WithName("ui").Info("terminal area invalidated", "reason", reason)
	}
	a.terminalAreaCache.invalidate()
}

func (a *App) swapPanels() {
	// Swap panel pointers
	a.leftPanel, a.rightPanel = a.rightPanel, a.leftPanel

	// Swap navigators
	a.leftNav, a.rightNav = a.rightNav, a.leftNav

	// Swap configs
	a.leftConfig, a.rightConfig = a.rightConfig, a.leftConfig

	// Detach old listeners
	if a.leftFolderDirtyCancel != nil {
		a.leftFolderDirtyCancel()
		a.leftFolderDirtyCancel = nil
	}
	if a.rightFolderDirtyCancel != nil {
		a.rightFolderDirtyCancel()
		a.rightFolderDirtyCancel = nil
	}

	// Re-attach to new positions
	if a.leftPanel != nil {
		a.attachFolderDirtyListener(0, a.leftPanel.Folder())
	}
	if a.rightPanel != nil {
		a.attachFolderDirtyListener(1, a.rightPanel.Folder())
	}

	// Invalidate everything
	a.invalidateView("swap panels")
	a.invalidateFunctionBar("swap panels")

	// Toggle active panel so focus follows the content
	a.activePanel = (a.activePanel + 1) % 2
}

func (a *App) showContextMenuForPanel(p *Panel) tea.Cmd {
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()

	item, ok := p.SelectedNavItem(ctx)
	if !ok || item == nil {
		// For now, only show if item selected.
		// Later we might support global commands without selection if they don't depend on it.
		// But the requirement says "Global (d): Completely independent of selection".
		// So we should proceed even if item is nil, but filter accordingly.
	}

	// Filter commands
	selectedItems := p.GetSelectedItems()
	activeNamespace := deriveNamespace(item, selectedItems, p.currentPath)
	var available []appconfig.CommandConfig
	for _, cmd := range a.cfg.Commands {
		if isCommandApplicable(cmd, item, len(selectedItems), activeNamespace) {
			available = append(available, cmd)
		}
	}

	if len(available) == 0 {
		return nil
	}

	// Create selector
	selector := NewCommandSelectorModel(available, func(cmd appconfig.CommandConfig) tea.Cmd {
		a.hideModal()
		// Get items (handle multi-selection if supported)
		var items []models.Item
		if cmd.SupportsMultiSelection {
			// TODO: Get selected items from panel if multiple selected
			if item != nil {
				items = []models.Item{item}
			}
		} else {
			if item != nil {
				items = []models.Item{item}
			}
		}

		// Resolve GVR
		var gvr schema.GroupVersionResource
		if item != nil {
			if obj, ok := item.(models.ObjectItem); ok {
				gvr = obj.GVR()
			}
		}

		return p.StartCommand(a.ctx, cmd, items, gvr)
	}, func() tea.Cmd {
		a.hideModal()
		return nil
	})

	modal := NewModal("Commands", selector)
	modal.SetCloseOnSingleEsc(true)
	panelIdx := a.panelIndex(p)
	// Rebuild each time so we pick up fresh command lists and selection state.
	a.modalManager.Register("command_menu", modal)
	// Size and position like other panel-scoped modals.
	a.configureModalWindow(modal, selector, panelIdx, "", 48, len(selector.entries)+3)
	modal.SetOnClose(func() tea.Cmd { return nil })
	a.showModal("command_menu")
	return nil
}
