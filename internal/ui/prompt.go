package ui

import "github.com/charmbracelet/huh"

// huhPrompter is the interactive Prompter, backed by charmbracelet/huh. It
// is the only place that imports huh, keeping the prompt library out of the
// command handlers.
type huhPrompter struct{}

// MultiSelect shows a terminal multi-select and returns the chosen values.
func (huhPrompter) MultiSelect(title string, options []Option) ([]string, error) {
	huhOptions := make([]huh.Option[string], len(options))
	for i, o := range options {
		huhOptions[i] = huh.NewOption(o.Label, o.Value).Selected(o.Selected)
	}
	var selected []string
	field := huh.NewMultiSelect[string]().
		Title(title).
		Options(huhOptions...).
		Value(&selected)
	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		return nil, err
	}
	return selected, nil
}
