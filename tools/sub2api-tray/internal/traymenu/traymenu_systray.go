//go:build !headless

package traymenu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/control"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/nativeui"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/state"
	"github.com/energye/systray"
)

type App struct {
	Store  *state.Store
	Server *control.Server
	OnQuit func()

	mu          sync.Mutex
	statusItem  *systray.MenuItem
	modelItem   *systray.MenuItem
	autoItem    *systray.MenuItem
	modelSlots  []*systray.MenuItem
	modelNames  []string
	autoEnabled bool
}

func (a *App) Run() {
	systray.Run(a.onReady, a.onExit)
}

func (a *App) openPanel() {
	// Prefer native OS window; fall back to system browser only if webview fails.
	if err := nativeui.OpenInProcessWindow(a.Server.PanelURL()); err != nil {
		_ = openURL(a.Server.PanelURL())
	}
}

func (a *App) onReady() {
	if len(iconPNG) > 0 {
		systray.SetTemplateIcon(iconPNG, iconPNG)
		systray.SetIcon(iconPNG)
	}
	systray.SetTitle("S2A")
	systray.SetTooltip("Sub2API Codex Tray")

	a.statusItem = systray.AddMenuItem("状态: …", "连接/配置状态")
	a.statusItem.Disable()
	a.modelItem = systray.AddMenuItem("模型: …", "当前模型")
	a.modelItem.Disable()
	systray.AddSeparator()

	mOpen := systray.AddMenuItem("打开面板", "本机窗口打开配置页")
	mOpen.Click(func() { a.openPanel() })

	mRepair := systray.AddMenuItem("修复配置", "按已保存设置重写 Codex 托管块")
	mRepair.Click(func() {
		_ = a.postJSON("/repair", map[string]any{})
		a.refresh()
	})

	mRefresh := systray.AddMenuItem("刷新", "刷新状态与模型列表")
	mRefresh.Click(func() {
		_, _ = a.getJSON("/models")
		a.refresh()
	})

	a.autoItem = systray.AddMenuItem("开机启动", "登录时自动启动托盘")
	a.autoItem.Click(func() {
		next := !a.autoEnabled
		_ = a.postJSON("/autostart", map[string]any{"enabled": next})
		a.refresh()
	})
	systray.AddSeparator()

	const slots = 8
	a.modelSlots = make([]*systray.MenuItem, 0, slots)
	for i := 0; i < slots; i++ {
		item := systray.AddMenuItem("·", "切换模型")
		item.Hide()
		a.modelSlots = append(a.modelSlots, item)
		idx := i
		item.Click(func() { a.setModelByIndex(idx) })
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出托盘")
	mQuit.Click(func() { systray.Quit() })

	a.refresh()
	// Auto-open native panel once on launch.
	go func() {
		time.Sleep(400 * time.Millisecond)
		a.openPanel()
	}()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			a.refresh()
		}
	}()
}

func (a *App) onExit() {
	if a.OnQuit != nil {
		a.OnQuit()
	}
}

func (a *App) refresh() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.statusItem == nil || a.modelItem == nil {
		return
	}
	st := a.Store.Settings()
	if st.BaseURL != "" && st.APIKey != "" {
		a.statusItem.SetTitle("状态: 已配置")
	} else {
		a.statusItem.SetTitle("状态: 未配置（打开面板）")
	}
	a.modelItem.SetTitle("模型: " + st.Model)

	if status, err := a.getJSON("/status"); err == nil {
		if auto, ok := status["autostart"].(map[string]any); ok {
			if enabled, ok := auto["enabled"].(bool); ok {
				a.autoEnabled = enabled
			}
		}
	}
	if a.autoItem != nil {
		if a.autoEnabled {
			a.autoItem.SetTitle("开机启动 ✓")
		} else {
			a.autoItem.SetTitle("开机启动")
		}
	}

	models := st.Models
	if len(models) == 0 && st.Model != "" {
		models = []string{st.Model}
	}
	a.modelNames = append([]string(nil), models...)
	for i, item := range a.modelSlots {
		if i >= len(models) {
			item.Hide()
			continue
		}
		name := models[i]
		title := "→ " + name
		if name == st.Model {
			title = "✓ " + name
		}
		item.SetTitle(title)
		item.Show()
	}
}

func (a *App) setModelByIndex(idx int) {
	a.mu.Lock()
	var model string
	if idx >= 0 && idx < len(a.modelNames) {
		model = a.modelNames[idx]
	}
	a.mu.Unlock()
	if model == "" {
		return
	}
	_ = a.postJSON("/model-set", map[string]any{"model": model})
	a.refresh()
}

func (a *App) postJSON(path string, payload any) error {
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, a.Server.BaseURL()+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("http %d", res.StatusCode)
	}
	return nil
}

func (a *App) getJSON(path string) (map[string]any, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.Get(a.Server.BaseURL() + path)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out, nil
}
