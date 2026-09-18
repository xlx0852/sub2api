//go:build headless

package nativeui

import "fmt"

func RunWindow(panelURL, title string) error {
	return fmt.Errorf("native ui disabled in headless build")
}

func OpenInProcessWindow(panelURL string) error {
	return fmt.Errorf("native ui disabled in headless build")
}
