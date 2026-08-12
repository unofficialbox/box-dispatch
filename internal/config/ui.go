package config

// UISettings contains local operator presentation choices. Persistence lives
// in internal/shellstate as BCL so UI preferences do not leak into a packaged
// solution manifest.
type UISettings struct {
	BoxComponentVisibility map[string]bool `json:"boxComponentVisibility,omitempty"`
	AccessibleForms        bool            `json:"accessibleForms,omitempty"`
}
