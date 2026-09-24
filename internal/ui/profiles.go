package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/carabila/lazyftp/internal/client"
	"github.com/carabila/lazyftp/internal/config"
	"github.com/mattn/go-runewidth"
)

type profilePickerState struct {
	open          bool
	selected      int
	confirmDelete bool
	pending       bool
	err           string
}

type profileSaveState struct {
	open        bool
	name        textinput.Model
	overwriting bool
	pending     bool
	err         string
}

type profilesLoadedMsg struct {
	profiles []config.Profile
	err      error
}

type profileWriteKind int

const (
	profileWriteSave profileWriteKind = iota
	profileWriteDelete
)

type profileWriteRequestMsg struct {
	kind    profileWriteKind
	profile config.Profile
	name    string
}

type profileWriteDoneMsg struct {
	kind     profileWriteKind
	profiles []config.Profile
	name     string
	err      error
}

func loadProfiles() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		return profilesLoadedMsg{profiles: cfg.Profiles, err: err}
	}
}

func (c ConnectionBar) modalOpen() bool {
	return c.picker.open || c.profilePicker.open || c.profileSave.open
}

func (c ConnectionBar) openProfiles() ConnectionBar {
	c = c.blur()
	c.profileNotice = ""
	c.profilePicker = profilePickerState{open: true}
	for i, profile := range c.profiles {
		if strings.EqualFold(profile.Name, c.selectedProfile) {
			c.profilePicker.selected = i
			break
		}
	}
	return c
}

func (c ConnectionBar) openProfileSave() ConnectionBar {
	c = c.blur()
	name := textinput.New()
	name.Prompt = ""
	name.Placeholder = "Profile name"
	name.SetWidth(32)
	if c.selectedProfile != "" {
		name.SetValue(c.selectedProfile)
	}
	name.Focus()
	c.profileSave = profileSaveState{open: true, name: name}
	c.profileNotice = ""
	return c
}

func (c ConnectionBar) updateProfilePicker(msg tea.KeyPressMsg) (ConnectionBar, tea.Cmd) {
	switch {
	case c.profilePicker.confirmDelete:
		switch {
		case key.Matches(msg, keyProfileConfirm):
			if profile, ok := c.selectedProfileEntry(); ok {
				c.profilePicker.confirmDelete = false
				c.profilePicker.pending = true
				return c, func() tea.Msg {
					return profileWriteRequestMsg{kind: profileWriteDelete, name: profile.Name}
				}
			}
		case key.Matches(msg, keyProfileDecline), key.Matches(msg, keyProfileCancel):
			c.profilePicker.confirmDelete = false
		}
		return c, nil

	case c.profilePicker.pending:
		return c, nil

	case key.Matches(msg, keyProfileCancel):
		c.profilePicker.open = false
		return c.focus(), nil

	case key.Matches(msg, keySaveProfile):
		if c.profilesLoaded && c.profileLoadError == "" {
			c.profilePicker.open = false
			return c.openProfileSave(), nil
		}
	case key.Matches(msg, keyPickerUp):
		if len(c.profiles) > 0 {
			c.profilePicker.selected = (c.profilePicker.selected + len(c.profiles) - 1) % len(c.profiles)
		}
	case key.Matches(msg, keyPickerDown):
		if len(c.profiles) > 0 {
			c.profilePicker.selected = (c.profilePicker.selected + 1) % len(c.profiles)
		}
	case key.Matches(msg, keyPickerSelect):
		if profile, ok := c.selectedProfileEntry(); ok {
			return c.withProfile(profile), nil
		}
	case key.Matches(msg, keyDeleteProfile):
		if _, ok := c.selectedProfileEntry(); ok {
			c.profilePicker.confirmDelete = true
		}
	}
	return c, nil
}

func (c ConnectionBar) updateProfileSave(msg tea.KeyPressMsg) (ConnectionBar, tea.Cmd) {
	if c.profileSave.pending {
		return c, nil
	}

	if c.profileSave.overwriting {
		switch {
		case key.Matches(msg, keyProfileConfirm):
			return c.saveProfile()
		case key.Matches(msg, keyProfileDecline), key.Matches(msg, keyProfileCancel):
			c.profileSave.overwriting = false
			return c, nil
		default:
			return c, nil
		}
	}

	switch {
	case key.Matches(msg, keyProfileCancel):
		c.profileSave.name.Blur()
		c.profileSave.open = false
		return c.focus(), nil
	case key.Matches(msg, keySaveProfileConfirm):
		name := strings.TrimSpace(c.profileSave.name.Value())
		if name == "" {
			c.profileSave.err = "Enter a profile name"
			return c, nil
		}
		c.profileSave.name.SetValue(name)
		c.profileSave.err = ""
		if index := c.profileIndex(name); index >= 0 {
			c.profileSave.name.SetValue(c.profiles[index].Name)
			c.profileSave.overwriting = true
			return c, nil
		}
		return c.saveProfile()
	}

	var cmd tea.Cmd
	c.profileSave.name, cmd = c.profileSave.name.Update(msg)
	return c, cmd
}

func (c ConnectionBar) saveProfile() (ConnectionBar, tea.Cmd) {
	name := strings.TrimSpace(c.profileSave.name.Value())
	if name == "" {
		c.profileSave.err = "Enter a profile name"
		c.profileSave.overwriting = false
		return c, nil
	}
	profile := c.profileFromForm(name)
	c.profileSave.overwriting = false
	c.profileSave.pending = true
	return c, func() tea.Msg {
		return profileWriteRequestMsg{kind: profileWriteSave, profile: profile, name: profile.Name}
	}
}

func (c ConnectionBar) selectedProfileEntry() (config.Profile, bool) {
	if c.profilePicker.selected < 0 || c.profilePicker.selected >= len(c.profiles) {
		return config.Profile{}, false
	}
	return c.profiles[c.profilePicker.selected], true
}

func (c ConnectionBar) profileIndex(name string) int {
	for i, profile := range c.profiles {
		if strings.EqualFold(profile.Name, name) {
			return i
		}
	}
	return -1
}

func (c ConnectionBar) profileFromForm(name string) config.Profile {
	port, _ := strconv.Atoi(c.inputs[fieldPort].Value())
	profile := config.Profile{
		Name:     name,
		Protocol: c.protocol.String(),
		Host:     c.inputs[fieldHost].Value(),
		Port:     port,
		User:     c.inputs[fieldUser].Value(),
		Auth:     config.AuthPassword,
	}
	if c.protocol == client.SFTP {
		if c.auth == client.SFTPAuthIdentityFile {
			profile.Auth = config.AuthIdentityFile
			profile.IdentityFile = c.inputs[fieldIdentity].Value()
		} else {
			profile.Password = c.inputs[fieldPass].Value()
		}
	} else {
		profile.Password = c.inputs[fieldPass].Value()
	}
	return profile
}

func (c ConnectionBar) withProfile(profile config.Profile) ConnectionBar {
	switch profile.Protocol {
	case "FTPS":
		c.protocol = client.FTPS
	case "SFTP":
		c.protocol = client.SFTP
	default:
		c.protocol = client.FTP
	}
	c.inputs[fieldHost].SetValue(profile.Host)
	c.inputs[fieldUser].SetValue(profile.User)
	c.inputs[fieldPort].SetValue("")
	if profile.Port > 0 {
		c.inputs[fieldPort].SetValue(strconv.Itoa(profile.Port))
	}
	if c.protocol == client.SFTP && profile.Auth == config.AuthIdentityFile {
		c.auth = client.SFTPAuthIdentityFile
		c.inputs[fieldIdentity].SetValue(profile.IdentityFile)
		c.inputs[fieldPass].SetValue("")
		c.inputs[fieldKeyPassphrase].SetValue("")
	} else {
		c.auth = client.SFTPAuthPassword
		c.inputs[fieldPass].SetValue(profile.Password)
		c.inputs[fieldIdentity].SetValue("")
		c.inputs[fieldKeyPassphrase].SetValue("")
	}
	c.profilePicker.open = false
	c.profilePicker.confirmDelete = false
	c.selectedProfile = profile.Name
	c.profileNotice = ""

	focus := fieldProtocol
	if c.protocol == client.SFTP && c.auth == client.SFTPAuthIdentityFile {
		focus = fieldKeyPassphrase
	}
	return c.setFocus(focus).showDefaultPort()
}

func (c ConnectionBar) withProfilesLoaded(msg profilesLoadedMsg) ConnectionBar {
	c.profilesLoaded = true
	c.profileLoadError = ""
	c.profileNotice = ""
	if msg.err != nil {
		c.profileLoadError = msg.err.Error()
		c.profileNotice = "Could not load profiles; see the Log"
		c.profiles = nil
		return c
	}
	c.profiles = append([]config.Profile(nil), msg.profiles...)
	return c
}

func (c ConnectionBar) withProfileWriteDone(msg profileWriteDoneMsg) ConnectionBar {
	if msg.err != nil {
		if msg.kind == profileWriteSave {
			c.profileSave.pending = false
			c.profileSave.err = msg.err.Error()
		} else {
			c.profilePicker.pending = false
			c.profilePicker.err = msg.err.Error()
		}
		return c
	}

	c.profiles = append([]config.Profile(nil), msg.profiles...)
	c.profilesLoaded = true
	c.profileLoadError = ""
	if msg.kind == profileWriteSave {
		c.profileSave.name.Blur()
		c.profileSave.open = false
		c.profileSave.pending = false
		c.selectedProfile = msg.name
		c.profileNotice = fmt.Sprintf("Saved profile %q", msg.name)
		c = c.focus()
	} else {
		c.profilePicker.pending = false
		c.profilePicker.err = ""
		c.profilePicker.confirmDelete = false
		if strings.EqualFold(c.selectedProfile, msg.name) {
			c.selectedProfile = ""
		}
		if c.profilePicker.selected >= len(c.profiles) {
			c.profilePicker.selected = max(0, len(c.profiles)-1)
		}
	}
	return c
}

func (c ConnectionBar) profilePickerView(maxWidth int) string {
	width := min(64, maxWidth-2)
	if width < 20 {
		width = 20
	}
	innerWidth := max(1, borderInteriorWidth(width))
	lines := []string{}
	switch {
	case c.profileLoadError != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render(runewidth.Truncate(c.profileLoadError, innerWidth, "…")))
	case !c.profilesLoaded:
		lines = append(lines, "Loading saved profiles…")
	case c.profilePicker.pending:
		lines = append(lines, "Updating profiles…")
	case c.profilePicker.err != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render(runewidth.Truncate(c.profilePicker.err, innerWidth, "…")))
	case len(c.profiles) == 0:
		lines = append(lines, "No saved profiles.")
	default:
		start := max(0, c.profilePicker.selected-7)
		end := min(len(c.profiles), start+8)
		for i := start; i < end; i++ {
			profile := c.profiles[i]
			prefix := "  "
			if i == c.profilePicker.selected {
				prefix = "> "
			}
			line := runewidth.Truncate(prefix+profile.Name, innerWidth, "…")
			style := lipgloss.NewStyle().Width(innerWidth)
			if i == c.profilePicker.selected {
				style = style.Foreground(colorAccent).Bold(true).Reverse(true)
			}
			lines = append(lines, style.Render(line))
		}
	}

	if profile, ok := c.selectedProfileEntry(); ok && c.profileLoadError == "" {
		port := profile.Port
		if port == 0 {
			switch profile.Protocol {
			case "SFTP":
				port = 22
			default:
				port = 21
			}
		}
		detail := fmt.Sprintf("%s · %s@%s:%d", profile.Protocol, profile.User, profile.Host, port)
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colorMuted).Render(runewidth.Truncate(detail, innerWidth, "…")))
	}
	if c.profilePicker.confirmDelete {
		if profile, ok := c.selectedProfileEntry(); ok {
			lines = append(lines, "", lipgloss.NewStyle().Foreground(colorError).Render(runewidth.Truncate(fmt.Sprintf("Delete %q? y confirm · n cancel", profile.Name), innerWidth, "…")))
		}
	} else if c.profilesLoaded && c.profileLoadError == "" && !c.profilePicker.pending {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colorMuted).Render("↑/↓ move · Enter load · d delete · Ctrl+S save · Esc close"))
	}

	body := strings.Join(lines, "\n")
	if body == "" {
		body = "No saved profiles."
	}
	return borderWithTitle(body, "Saved profiles", width, lipgloss.Height(body)+2, colorAccent)
}

func (c ConnectionBar) profileSaveView(maxWidth int) string {
	width := min(56, maxWidth-2)
	if width < 20 {
		width = 20
	}
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	var lines []string
	switch {
	case c.profileLoadError != "":
		lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render(runewidth.Truncate("Cannot save: "+c.profileLoadError, borderInteriorWidth(width), "…")))
	case c.profileSave.pending:
		lines = append(lines, "Saving profile…")
	case c.profileSave.overwriting:
		lines = append(lines,
			fmt.Sprintf("Replace saved profile %q?", c.profileSave.name.Value()),
			muted.Render("y overwrite · n return to name"),
		)
	default:
		lines = append(lines,
			"Name: "+c.profileSave.name.View(),
			muted.Render("Any server password is plaintext in "+profileConfigPathHint()+"."),
			muted.Render("Key passphrases are not saved."),
		)
		if c.profileSave.err != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorError).Render(runewidth.Truncate(c.profileSave.err, borderInteriorWidth(width), "…")))
		}
		lines = append(lines, muted.Render("Enter save · Esc cancel"))
	}
	body := strings.Join(lines, "\n\n")
	return borderWithTitle(body, "Save profile", width, lipgloss.Height(body)+2, colorAccent)
}

func profileConfigPathHint() string {
	return filepath.Join("~", ".lazyftp", "config.json")
}
