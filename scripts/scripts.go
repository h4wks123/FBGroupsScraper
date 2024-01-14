package scripts

import (
	_ "embed"
	"fmt"
	"time"
)

var (
	//go:embed js/extractAlltext.js
	ExtractAllTextScript string

	//go:embed js/trackStability.js
	TrackStabilityScript string
)

func All() []string {
	return []string{
		ExtractAllTextScript,
		TrackStabilityScript,
	}
}

func TrackStability(xpath, key string, debounce time.Duration) string {
	return fmt.Sprintf(`trackStability("%s", "%s", %d)`, xpath, key, debounce.Milliseconds())
}

func CheckStability(key string) string {
	return fmt.Sprintf(`window.isStable["%s"]`, key)
}

func ExtractAllText(xpath, refine string) string {
	return fmt.Sprintf(`extractAllText("%s", "%s");`, xpath, refine)
}
